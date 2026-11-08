// Package master implements the coremon master: web UI, UI API, agent
// hub, job runner and signed-artifact repository.
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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"coremon/internal/sign"
)

//go:embed web
var webFS embed.FS

// Config configures the master.
type Config struct {
	Listen     string
	HTTPListen string // optional plain-HTTP UI listener, loopback only (reverse proxy)
	DataDir    string
	ReleaseDir string
	SANs       []string
	Version    string
	BuildDate  string // YYYY-MM-DD, set at release build time
	// SigningKeyPath is the release signing public key (root-owned).
	SigningKeyPath string
	// GitHubRepo ("owner/name") publishes the version index.
	GitHubRepo string
	// IndexURL overrides the version index location (testing).
	IndexURL string
}

// Server is the master.
type Server struct {
	cfg           Config
	st            *store
	repo          *repo
	sess          *sessions
	audit         *auditLog
	pin           string
	cert          tls.Certificate
	signingKeyB64 string
	wakes         map[string]chan struct{} // guarded by st.mu
	idx           *indexSync

	handler http.Handler

	purgeMu   sync.Mutex
	nextPurge time.Time

	uiMu  sync.Mutex
	uiSrv *http.Server // extra website listener on port 443
	uiErr string

	ipMu    sync.Mutex
	ipCache map[string]bool
	ipAt    time.Time
}

// ownIPs returns this host's addresses (cached for a minute), used to
// recognise the master's own system among the agents.
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

// agentPort is the port agents use (the -listen address).
func (s *Server) agentPort() string {
	_, port, err := net.SplitHostPort(s.cfg.Listen)
	if err != nil || port == "" {
		return "8443"
	}
	return port
}

// applyUIPort starts or stops the extra website listener so the website is
// reachable on the configured port. Agents always keep using agentPort.
func (s *Server) applyUIPort(handler http.Handler) {
	s.st.mu.Lock()
	want := s.st.st.Settings.UIPort
	s.st.mu.Unlock()
	s.uiMu.Lock()
	defer s.uiMu.Unlock()
	// Only 443 needs its own listener; 8443 is the agents' listener, which
	// always serves the website too.
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
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{s.cert}},
		ErrorLog:  log.New(os.Stderr, "http: ", 0)}
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

// New prepares the data directory, certificate, keys and state.
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
	s := &Server{cfg: cfg, st: st, sess: newSessions(), cert: cert, pin: pin,
		audit: &auditLog{path: filepath.Join(cfg.DataDir, "audit.jsonl")}, wakes: map[string]chan struct{}{}}

	// Release signing public key. In production it lives in a root-owned
	// file written by the installer, so the (unprivileged) master cannot
	// change which code it trusts.
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
	s.cleanupLegacy()
	s.repo.scan()
	s.idx = newIndexSync()
	if !st.st.Settings.GitHubSeeded && cfg.GitHubRepo != "" && ghRepoRe.MatchString(cfg.GitHubRepo) {
		st.st.Settings.GitHubRepo = cfg.GitHubRepo
		st.st.Settings.GitHubSeeded = true
		st.markDirty()
	}

	if st.st.PasswordHash == "" {
		pw := randHex(8)
		st.st.PasswordHash = hashPassword(pw)
		if err := os.WriteFile(s.initialPwPath(), []byte(pw+"\n"), 0o600); err != nil {
			return nil, err
		}
		if err := st.writeNow(); err != nil {
			return nil, err
		}
		log.Printf("initial admin password written to %s", s.initialPwPath())
	}
	return s, nil
}

// ResetPassword sets a new random password and prints it.
func ResetPassword(dataDir string) (string, error) {
	st, err := openStore(dataDir)
	if err != nil {
		return "", err
	}
	pw := randHex(8)
	st.st.PasswordHash = hashPassword(pw)
	if err := st.writeNow(); err != nil {
		return "", err
	}
	return pw, nil
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("X-Frame-Options", "DENY")
		hd.Set("Referrer-Policy", "no-referrer")
		h.ServeHTTP(w, r)
	})
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	// Agent API (authenticated per agent, except enroll which needs a token).
	mux.HandleFunc("POST /api/agent/enroll", s.handleEnroll)
	mux.HandleFunc("POST /api/agent/poll", s.handlePoll)
	mux.HandleFunc("POST /api/agent/output", s.handleOutput)
	mux.HandleFunc("POST /api/agent/result", s.handleResult)
	mux.HandleFunc("GET /api/agent/download/{kind}/{name}/{version}/{os}/{arch}", s.handleDownload)
	mux.HandleFunc("GET /install.sh", s.handleInstallScript)
	mux.HandleFunc("GET /dl/agent/{plat}", s.handlePublicAgent)

	// UI API.
	mux.HandleFunc("GET /api/ui/session", s.handleSession)
	mux.HandleFunc("POST /api/ui/login", s.handleLogin)
	mux.HandleFunc("POST /api/ui/logout", s.handleLogout)
	ui := func(p string, h http.HandlerFunc) { mux.HandleFunc(p, s.requireUI(h)) }
	ui("POST /api/ui/password", s.handlePassword)
	ui("GET /api/ui/overview", s.handleOverview)
	ui("GET /api/ui/agents/{id}", s.handleAgentGet)
	ui("POST /api/ui/agents/{id}", s.handleAgentUpdate)
	ui("POST /api/ui/agents/{id}/approve", s.handleApprove)
	ui("POST /api/ui/agents/{id}/remove", s.handleRemove)
	ui("POST /api/ui/agents/{id}/core", s.handleCoreUpdate)
	ui("POST /api/ui/actions", s.handleActions)
	ui("GET /api/ui/jobs", s.handleJobs)
	ui("GET /api/ui/jobs/{id}", s.handleJob)
	ui("GET /api/ui/tokens", s.handleTokens)
	ui("POST /api/ui/tokens", s.handleTokenCreate)
	ui("POST /api/ui/tokens/{hash}/delete", s.handleTokenDelete)
	ui("POST /api/ui/groups", s.handleGroupCreate)
	ui("POST /api/ui/groups/{id}", s.handleGroupUpdate)
	ui("POST /api/ui/groups/{id}/delete", s.handleGroupDelete)
	ui("POST /api/ui/layout", s.handleLayout)
	ui("GET /api/ui/retention", s.handleRetentionPreview)
	ui("GET /api/ui/settings", s.handleSettingsGet)
	ui("POST /api/ui/settings", s.handleSettingsSave)
	ui("GET /api/ui/audit", s.handleAudit)
	ui("GET /api/ui/releases", s.handleReleases)
	ui("POST /api/ui/releases/check", s.handleReleasesCheck)
	ui("POST /api/ui/master/update", s.handleMasterUpdate)

	// Static UI.
	sub, _ := fs.Sub(webFS, "web")
	files := http.FileServer(http.FS(sub))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
	return securityHeaders(mux)
}

// Run serves until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	stop := make(chan struct{})
	go s.st.flushLoop(stop)
	go s.schedulerLoop(stop)
	go s.indexLoop(stop)
	handler := s.routes()
	s.handler = handler
	s.applyUIPort(handler)
	srv := &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{s.cert}},
		ErrorLog:          log.New(os.Stderr, "http: ", 0),
	}
	errc := make(chan error, 2)
	go func() { errc <- srv.ListenAndServeTLS("", "") }()
	var plain *http.Server
	if s.cfg.HTTPListen != "" {
		host, _, err := net.SplitHostPort(s.cfg.HTTPListen)
		if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("-http must be a loopback address like 127.0.0.1:8080 (got %q)", s.cfg.HTTPListen)
		}
		plain = &http.Server{Addr: s.cfg.HTTPListen, Handler: s.routes(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
		go func() { errc <- plain.ListenAndServe() }()
		log.Printf("plain HTTP UI on http://%s (loopback only, for a local reverse proxy)", s.cfg.HTTPListen)
	}
	log.Printf("coremon master %s listening on https://%s", s.cfg.Version, s.cfg.Listen)
	log.Printf("agent pin: sha256//%s", s.pin)
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
