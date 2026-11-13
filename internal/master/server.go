// Package master: website, API, agent hub, jobs and release store.
package master

import (
	"context"
	"crypto/tls"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"coremon/internal/proto"
	"coremon/internal/sign"
)

//go:embed web
var webFS embed.FS

type Config struct {
	Listen     string
	HTTPListen string
	DataDir    string
	ReleaseDir string
	SANs       []string
	Version    string
	BuildDate  string

	SigningKeyPath string

	GitHubRepo string

	IndexURL string

	OSInfoURL string

	OSVURL, NVDURL, KEVURL, EPSSURL string
}

type Server struct {
	cfg           Config
	st            *store
	repo          *repo
	sess          *sessions
	audit         *auditLog
	keyMu         sync.Mutex
	keys          keyState
	certMu        sync.Mutex
	tlsMu         sync.Mutex
	https         []*http.Server
	ticketKeys    [][32]byte
	ticketKeyAt   time.Time
	tlsCfg        *tls.Config
	certTried     time.Time
	signingKeyB64 string
	wakes         map[string]chan struct{}
	pings         map[string]chan proto.Result
	polls         map[string]chan struct{}
	agentRate     rateLimiter
	intake        intake
	enrolls       enrollGuard
	hashSem       chan struct{}
	idx           *indexSync
	nonces        nonceCache
	osi           osInfo
	vul           vulnStore

	handler http.Handler

	purgeMu   sync.Mutex
	nextPurge time.Time

	uiMu  sync.Mutex
	uiSrv *http.Server
	uiErr string

	ipMu    sync.Mutex
	ipCache map[string]bool
	ipAt    time.Time
}

const readTimeout = 60 * time.Second

func (s *Server) ownIPs() map[string]bool {
	s.ipMu.Lock()
	defer s.ipMu.Unlock()
	if s.ipCache != nil && time.Since(s.ipAt) < time.Minute {
		return s.ipCache
	}
	m := map[string]bool{}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && !ipn.IP.IsLinkLocalUnicast() {
			m[ipn.IP.String()] = true
		}
	}
	s.ipCache, s.ipAt = m, time.Now()
	return m
}

func (s *Server) agentPort() string {
	_, port, err := net.SplitHostPort(s.cfg.Listen)
	if err != nil || port == "" {
		return "8443"
	}
	return port
}

func (s *Server) applyUIPort(handler http.Handler) {
	s.st.mu.Lock()
	want := s.st.st.Settings.UIPort
	s.st.mu.Unlock()
	s.uiMu.Lock()
	defer s.uiMu.Unlock()

	need := want == 443 && s.agentPort() != "443"
	if s.uiSrv != nil && !need {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		s.uiSrv.Shutdown(ctx)
		cancel()
		s.uiSrv = nil
		log.Printf("website listener on port 443 stopped")
	}
	if !need {
		s.uiErr = ""
		return
	}
	if s.uiSrv != nil {
		return
	}
	host, _, _ := net.SplitHostPort(s.cfg.Listen)
	addr := net.JoinHostPort(host, fmt.Sprint(want))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.uiErr = fmt.Sprintf("cannot listen on port %d: %v", want, err)
		if errors.Is(err, os.ErrPermission) || strings.Contains(err.Error(), "permission denied") {
			s.uiErr += " (the service needs CAP_NET_BIND_SERVICE; re-run the master installer)"
		}
		log.Printf("website: %s", s.uiErr)
		return
	}
	srv := s.newHTTPSServer(handler, "")
	s.uiSrv, s.uiErr = srv, ""
	go func() {
		if err := srv.ServeTLS(ln, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("website listener: %v", err)
		}
	}()
	log.Printf("website also on https://%s", addr)
}

func (s *Server) uiPortError() string {
	s.uiMu.Lock()
	defer s.uiMu.Unlock()
	return s.uiErr
}

func (s *Server) initialPwPath() string { return filepath.Join(s.cfg.DataDir, "initial-password.txt") }

func New(cfg Config) (*Server, error) {
	if err := os.MkdirAll(filepath.Join(cfg.DataDir, "jobs"), 0o700); err != nil {
		return nil, err
	}
	if cfg.ReleaseDir == "" {
		cfg.ReleaseDir = filepath.Join(cfg.DataDir, "release")
	}
	os.MkdirAll(cfg.ReleaseDir, 0o755)
	st, err := openStore(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open state: %w", err)
	}
	cert, pin, err := loadOrCreateCert(cfg.DataDir, cfg.SANs)
	if err != nil {
		return nil, fmt.Errorf("tls: %w", err)
	}
	s := &Server{cfg: cfg, st: st, sess: newSessions(), keys: keyState{cert: &cert, tlsPin: pin},
		audit: &auditLog{path: filepath.Join(cfg.DataDir, "audit.jsonl")}, wakes: map[string]chan struct{}{},
		hashSem: make(chan struct{}, hashAtOnce)}

	pubPath := cfg.SigningKeyPath
	if pubPath == "" {
		pubPath = "/etc/coremon-master/signing.pub"
	}
	s.repo = &repo{dir: cfg.ReleaseDir}
	if b, err := os.ReadFile(pubPath); err == nil {
		pub, err := sign.ParsePublic(string(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", pubPath, err)
		}
		s.repo.pub = pub
		s.signingKeyB64 = base64.StdEncoding.EncodeToString(pub)
	} else {
		log.Printf("WARNING: no signing key at %s - no releases can be verified", pubPath)
	}
	if err := s.loadKeys(); err != nil {
		return nil, err
	}
	s.raiseMinAgent()
	s.repo.scan()
	s.idx = newIndexSync()
	s.osi.kick = make(chan struct{}, 1)
	s.loadOSInfo()
	s.vul.kick, s.vul.nvd = make(chan struct{}, 1), make(chan struct{}, 1)
	s.loadVulns()
	if !st.st.Settings.GitHubSeeded && cfg.GitHubRepo != "" && ghRepoRe.MatchString(cfg.GitHubRepo) {
		st.st.Settings.GitHubRepo = cfg.GitHubRepo
		st.st.Settings.GitHubSeeded = true
		st.markDirty()
	}

	if st.userByName(adminName) == nil {
		restored := len(st.st.Users) > 0
		pw := randHex(8)
		u := &User{ID: "u_" + randHex(6), Name: adminName, Role: RoleAdmin, PasswordHash: hashPassword(pw), Created: time.Now().Unix(), MustChange: true}
		st.st.Users = append(st.st.Users, u)
		st.st.InitialUser = u.ID
		if err := os.WriteFile(s.initialPwPath(), []byte(pw+"\n"), 0o600); err != nil {
			return nil, err
		}
		if err := st.writeNow(); err != nil {
			return nil, err
		}
		if restored {
			s.audit.write("master", "", "user.admin_restored", adminName, "the permanent admin account was missing and has been restored")
			log.Printf("the admin account was missing: restored, password written to %s", s.initialPwPath())
		} else {
			log.Printf("initial password of user admin written to %s", s.initialPwPath())
		}
	}

	if u := st.userByID(st.st.InitialUser); u != nil && !u.MustChange {
		if _, err := os.Stat(s.initialPwPath()); err == nil {
			u.MustChange = true
			st.markDirty()
		}
	}
	return s, nil
}

func ResetPassword(dataDir, name string) (string, error) {
	st, err := openStore(dataDir)
	if err != nil {
		return "", err
	}
	u := st.userByName(name)
	if u == nil {
		var admins []string
		for _, x := range st.st.Users {
			if x.Role == RoleAdmin {
				admins = append(admins, x.Name)
			}
		}
		return "", fmt.Errorf("there is no user %q; administrators: %s", name, strings.Join(admins, ", "))
	}
	pw := randHex(8)
	wasLocked := u.Locked
	u.PasswordHash, u.MustChange, u.Locked = hashPassword(pw), true, false
	if err := st.writeNow(); err != nil {
		return "", err
	}
	note := "new generated password via coremon-reset on the master (must be changed at the next sign-in)"
	if wasLocked {
		note += "; account unlocked"
	}
	(&auditLog{path: filepath.Join(dataDir, "audit.jsonl")}).write("root", "", "password.reset", name, note)
	return pw, nil
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Referrer-Policy", "no-referrer")

		if !strings.HasPrefix(r.URL.Path, "/api/") {
			hd.Set("X-Robots-Tag", "noindex")
		}
		h.ServeHTTP(w, r)
	})
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/agent/v5", s.handleSealed)
	mux.HandleFunc("GET /dl/core/{version}/{plat}", s.handlePublicCore)
	mux.HandleFunc("GET /install.sh", s.handleInstallScript)
	mux.HandleFunc("GET /dl/agent/{plat}", s.handlePublicAgent)

	mux.HandleFunc("GET /api/ui/session", s.handleSession)
	mux.HandleFunc("POST /api/ui/login", s.handleLogin)
	mux.HandleFunc("POST /api/ui/logout", s.handleLogout)

	view := func(p string, h http.HandlerFunc) { mux.HandleFunc(p, s.requireUI(permView, h)) }
	change := func(p string, h http.HandlerFunc) { mux.HandleFunc(p, s.requireUI(permChange, h)) }
	view("POST /api/ui/password", s.handlePassword)
	view("GET /api/ui/overview", s.handleOverview)
	view("GET /api/ui/agents/{id}", s.handleAgentGet)
	view("POST /api/ui/agents/{id}/ping", s.handlePing)
	change("POST /api/ui/agents/{id}", s.handleAgentUpdate)
	change("POST /api/ui/agents/{id}/approve", s.handleApprove)
	change("POST /api/ui/agents/{id}/remove", s.handleRemove)
	change("POST /api/ui/agents/{id}/core", s.handleCoreUpdate)
	change("POST /api/ui/actions", s.handleActions)
	view("GET /api/ui/jobs", s.handleJobs)
	view("GET /api/ui/jobs/{id}", s.handleJob)
	change("GET /api/ui/tokens", s.handleTokens)
	change("POST /api/ui/tokens", s.handleTokenCreate)
	change("POST /api/ui/tokens/{hash}/delete", s.handleTokenDelete)
	change("POST /api/ui/groups", s.handleGroupCreate)
	change("POST /api/ui/groups/{id}", s.handleGroupUpdate)
	change("POST /api/ui/groups/{id}/delete", s.handleGroupDelete)
	change("POST /api/ui/layout", s.handleLayout)
	view("GET /api/ui/retention", s.handleRetentionPreview)
	view("GET /api/ui/settings", s.handleSettingsGet)
	view("GET /api/ui/identity", s.handleIdentity)
	change("POST /api/ui/identity/{action}", s.handleIdentityAction)
	change("POST /api/ui/settings", s.handleSettingsSave)
	view("GET /api/ui/audit", s.handleAudit)
	view("GET /api/ui/releases", s.handleReleases)
	view("GET /api/ui/vulns/queue", s.handleVulnQueue)
	change("POST /api/ui/releases/check", s.handleReleasesCheck)
	change("POST /api/ui/master/update", s.handleMasterUpdate)
	change("POST /api/ui/osinfo/check", s.handleOSInfoCheck)
	change("POST /api/ui/vulns/check", s.handleVulnCheck)
	change("POST /api/ui/vulns/nvdkey", s.handleNVDKey)
	change("GET /api/ui/users", s.handleUsers)
	change("POST /api/ui/users", s.handleUserCreate)
	change("POST /api/ui/users/{id}", s.handleUserUpdate)
	change("POST /api/ui/users/{id}/delete", s.handleUserDelete)

	sub, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(sub))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache")
			files.ServeHTTP(w, r)
			return
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		// Files only: no directory listings.
		if st, err := fs.Stat(sub, name); err != nil || !st.Mode().IsRegular() || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
	return securityHeaders(mux)
}

func (s *Server) newHTTPSServer(handler http.Handler, addr string) *http.Server {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: s.currentCert,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) { return s.tlsNow(), nil }}
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       readTimeout,
		IdleTimeout:       120 * time.Second,
		TLSConfig:         cfg,
		ErrorLog:          log.New(os.Stderr, "http: ", 0),
	}
	s.tlsMu.Lock()
	if s.tlsCfg == nil {
		s.applyTicketKeys([][32]byte{newTicketKey()})
	}
	s.https = append(s.https, srv)
	s.tlsMu.Unlock()
	return srv
}

func (s *Server) Run(ctx context.Context) error {
	stop := make(chan struct{})
	go s.st.flushLoop(stop)
	go s.schedulerLoop(stop)
	go s.indexLoop(stop)
	go s.osInfoLoop(stop)
	go s.vulnLoop(stop)
	go s.nvdLoop(stop)
	handler := s.routes()
	s.handler = handler
	s.applyUIPort(handler)
	srv := s.newHTTPSServer(handler, s.cfg.Listen)
	errc := make(chan error, 2)
	go func() { errc <- srv.ListenAndServeTLS("", "") }()
	var plain *http.Server
	if s.cfg.HTTPListen != "" {
		host, _, err := net.SplitHostPort(s.cfg.HTTPListen)
		if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("-http must be a loopback address like 127.0.0.1:8080 (got %q)", s.cfg.HTTPListen)
		}
		plain = &http.Server{Addr: s.cfg.HTTPListen, Handler: viaLocalProxy(s.routes()), ReadHeaderTimeout: 10 * time.Second,
			ReadTimeout: readTimeout, IdleTimeout: 120 * time.Second}
		go func() { errc <- plain.ListenAndServe() }()
		log.Printf("plain HTTP UI on http://%s (loopback only, for a local reverse proxy)", s.cfg.HTTPListen)
	}
	log.Printf("coremon master %s listening on https://%s", s.cfg.Version, s.cfg.Listen)
	log.Printf("agent pin: sha256//%s", s.currentPin())
	var err error
	select {
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
		s.uiMu.Lock()
		if s.uiSrv != nil {
			s.uiSrv.Shutdown(sctx)
		}
		s.uiMu.Unlock()
		if plain != nil {
			plain.Shutdown(sctx)
		}
	case err = <-errc:
	}
	close(stop)
	time.Sleep(200 * time.Millisecond)
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}
