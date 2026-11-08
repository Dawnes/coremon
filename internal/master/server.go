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
	// SigningKeyPath is the release signing public key (root-owned).
	SigningKeyPath string
	// GitHubRepo ("owner/name") is the default release source.
	GitHubRepo string
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
	gh            *github
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
		log.Printf("WARNING: no signing key at %s - modules cannot be deployed", pubPath)
	}
	s.repo.scan()
	s.gh = newGitHub()
	if !st.st.Settings.GitHubSeeded && cfg.GitHubRepo != "" && ghRepoRe.MatchString(cfg.GitHubRepo) {
		st.st.Settings.GitHubRepo = cfg.GitHubRepo
		st.st.Settings.GitHubAutoImport = true
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
	ui("POST /api/ui/agents/{id}/modules", s.handleModuleOp)
	ui("POST /api/ui/agents/{id}/core", s.handleCoreUpdate)
	ui("POST /api/ui/actions", s.handleActions)
	ui("GET /api/ui/jobs", s.handleJobs)
	ui("GET /api/ui/jobs/{id}", s.handleJob)
	ui("GET /api/ui/tokens", s.handleTokens)
	ui("POST /api/ui/tokens", s.handleTokenCreate)
	ui("POST /api/ui/tokens/{hash}/delete", s.handleTokenDelete)
	ui("GET /api/ui/repo", s.handleRepo)
	ui("POST /api/ui/repo/rescan", s.handleRepoRescan)
	ui("GET /api/ui/settings", s.handleSettingsGet)
	ui("POST /api/ui/settings", s.handleSettingsSave)
	ui("GET /api/ui/audit", s.handleAudit)
	ui("GET /api/ui/github", s.handleGitHub)
	ui("POST /api/ui/github/check", s.handleGitHubCheck)
	ui("POST /api/ui/github/import", s.handleGitHubImport)
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
	go s.githubLoop(stop)
	srv := &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           s.routes(),
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
