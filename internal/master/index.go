package master

// Version index sync. The master downloads one signed file, versions.json,
// from a fixed address in the configured GitHub repository (the "index"
// release), checks its signature, downloads and verifies every listed agent
// binary and a newer master binary, and removes agent versions that are no
// longer listed. No GitHub API is used. Rolling a version out to systems
// stays a manual step.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"coremon/internal/release"
	"coremon/internal/sign"
)

var ghRepoRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}$`)

const (
	indexEvery = 6 * time.Hour
	// minAgentVersion: older agents use separate modules (protocol 1) and
	// are no longer offered.
	minAgentVersion = "0.2.0"
)

type indexVersion struct {
	Version   string   `json:"version"`
	Published string   `json:"published"`
	Platforms []string `json:"platforms"`
	Ready     bool     `json:"ready"` // downloaded and verified
}

type indexStatus struct {
	URL          string         `json:"url"`
	CheckedAt    int64          `json:"checked_at"`
	Error        string         `json:"error,omitempty"`
	Agents       []indexVersion `json:"agents"`
	MasterLatest string         `json:"master_latest"`
	Syncing      bool           `json:"syncing"`
}

type indexSync struct {
	mu   sync.Mutex
	st   indexStatus
	run  sync.Mutex
	hc   *http.Client
	kick chan struct{}
}

func newIndexSync() *indexSync {
	return &indexSync{hc: &http.Client{Timeout: 15 * time.Minute}, kick: make(chan struct{}, 1)}
}

func (x *indexSync) status() indexStatus {
	x.mu.Lock()
	defer x.mu.Unlock()
	s := x.st
	s.Agents = append([]indexVersion{}, x.st.Agents...)
	return s
}

// indexURL is where versions.json is published. Caller must not hold s.st.mu.
func (s *Server) indexURL() string {
	if s.cfg.IndexURL != "" {
		return s.cfg.IndexURL
	}
	s.st.mu.Lock()
	repo := s.st.st.Settings.GitHubRepo
	s.st.mu.Unlock()
	if repo == "" {
		return ""
	}
	return "https://github.com/" + repo + "/releases/download/" + release.IndexTag + "/" + release.IndexFile
}

func (s *Server) indexLoop(stop <-chan struct{}) {
	t := time.NewTimer(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		case <-s.idx.kick:
		}
		if err := s.syncIndex(); err != nil {
			log.Printf("version index: %v", err)
		}
		t.Reset(indexEvery)
	}
}

func (s *Server) kickIndex() {
	select {
	case s.idx.kick <- struct{}{}:
	default:
	}
}

func (s *Server) get(ctx context.Context, url string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "coremon-master/"+s.cfg.Version)
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := s.idx.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail := ""
		if resp.StatusCode >= 500 {
			detail = " (GitHub unavailable; retrying on the next check)"
		} else if resp.StatusCode == http.StatusNotFound {
			detail = " (not published yet?)"
		}
		return nil, fmt.Errorf("GET %s: %s%s", url, resp.Status, detail)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("GET %s: response too large", url)
	}
	return b, nil
}

// syncIndex runs one complete sync. Only one runs at a time.
func (s *Server) syncIndex() (err error) {
	s.idx.run.Lock()
	defer s.idx.run.Unlock()
	url := s.indexURL()
	s.idx.mu.Lock()
	s.idx.st.URL, s.idx.st.Syncing = url, true
	s.idx.mu.Unlock()
	defer func() {
		s.idx.mu.Lock()
		s.idx.st.Syncing = false
		s.idx.st.CheckedAt = time.Now().Unix()
		s.idx.st.Error = ""
		if err != nil {
			s.idx.st.Error = err.Error()
		}
		s.idx.mu.Unlock()
	}()
	if url == "" {
		return errors.New("no GitHub repository configured (Settings)")
	}
	if s.repo.pub == nil {
		return errors.New("master has no release signing key")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	body, err := s.get(ctx, url, 1<<20)
	if err != nil {
		return err
	}
	sig, err := s.get(ctx, url+".sig", 4096)
	if err != nil {
		return err
	}
	idx, err := release.VerifyIndex(s.repo.pub, body, string(sig))
	if err != nil {
		return err
	}

	var errs []string
	// Agents: keep exactly the listed versions (at most KeepAgents).
	agents := idx.Agent
	sort.Slice(agents, func(i, j int) bool { return versionLess(agents[j].Version, agents[i].Version) })
	if len(agents) > release.KeepAgents {
		agents = agents[:release.KeepAgents]
	}
	listed := map[string]bool{}
	var views []indexVersion
	for _, r := range agents {
		if !release.VersionRe.MatchString(r.Version) || versionLess(r.Version, minAgentVersion) {
			continue
		}
		listed[r.Version] = true
		v := indexVersion{Version: r.Version, Published: r.Published, Ready: true}
		for plat, a := range r.Assets {
			v.Platforms = append(v.Platforms, plat)
			if err := s.fetchAsset(ctx, "core", r.Version, plat, a); err != nil {
				errs = append(errs, err.Error())
				v.Ready = false
			}
		}
		sort.Strings(v.Platforms)
		views = append(views, v)
	}
	if len(listed) > 0 {
		s.pruneCore(func(v string) bool { return !listed[v] })
	}

	// Master: only a newer version for this platform. Never a downgrade.
	masterLatest := ""
	for _, r := range idx.Master {
		if release.VersionRe.MatchString(r.Version) && (masterLatest == "" || versionLess(masterLatest, r.Version)) {
			masterLatest = r.Version
		}
	}
	if masterLatest != "" && versionLess(s.cfg.Version, masterLatest) {
		plat := runtime.GOOS + "-" + runtime.GOARCH
		for _, r := range idx.Master {
			if r.Version != masterLatest {
				continue
			}
			if a, ok := r.Assets[plat]; ok {
				if err := s.fetchAsset(ctx, "master", r.Version, plat, a); err != nil {
					errs = append(errs, err.Error())
				}
			}
		}
	}
	s.pruneMaster(masterLatest)
	s.repo.scan()

	s.idx.mu.Lock()
	s.idx.st.Agents, s.idx.st.MasterLatest = views, masterLatest
	s.idx.mu.Unlock()
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// fetchAsset downloads one binary if it is not present yet, verifies its
// signature and checksum, and moves it into the release store.
func (s *Server) fetchAsset(ctx context.Context, kind, version, plat string, a release.Asset) error {
	pm := release.PlatRe.FindStringSubmatch(plat)
	if pm == nil || !release.VersionRe.MatchString(version) {
		return fmt.Errorf("index lists an invalid %s entry %q %q", kind, version, plat)
	}
	if e := s.repo.find(kind, version, pm[1], pm[2]); e != nil && e.SHA256 == a.SHA256 {
		return nil
	}
	name := binName(kind)
	// The signature binds kind, name, version, platform and hash; check it
	// before downloading anything.
	if !sign.Verify(s.repo.pub, sign.Message(kind, name, version, pm[1], pm[2], a.SHA256), a.Signature) {
		return fmt.Errorf("%s %s %s: signature does not verify", kind, version, plat)
	}
	if !strings.HasPrefix(a.URL, "https://") && s.cfg.IndexURL == "" {
		return fmt.Errorf("%s %s %s: download URL must be https", kind, version, plat)
	}
	body, err := s.get(ctx, a.URL, 256<<20)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != strings.ToLower(a.SHA256) {
		return fmt.Errorf("%s %s %s: checksum mismatch", kind, version, plat)
	}
	dst := s.repo.path(kind, version, plat)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst+".tmp", body, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst+".sig", []byte(a.Signature+"\n"), 0o644); err != nil {
		return err
	}
	if err := os.Rename(dst+".tmp", dst); err != nil {
		return err
	}
	log.Printf("version index: downloaded and verified %s %s %s", kind, version, plat)
	return nil
}

func (s *Server) pruneCore(drop func(version string) bool) {
	dirs, _ := os.ReadDir(filepath.Join(s.cfg.ReleaseDir, "core"))
	for _, d := range dirs {
		if d.IsDir() && drop(d.Name()) {
			os.RemoveAll(filepath.Join(s.cfg.ReleaseDir, "core", d.Name()))
			log.Printf("version index: removed agent %s (no longer listed)", d.Name())
		}
	}
}

// pruneMaster keeps only a master version newer than the running one.
func (s *Server) pruneMaster(keep string) {
	dirs, _ := os.ReadDir(filepath.Join(s.cfg.ReleaseDir, "master"))
	for _, d := range dirs {
		v := d.Name()
		if d.IsDir() && (v != keep || !versionLess(s.cfg.Version, v)) {
			os.RemoveAll(filepath.Join(s.cfg.ReleaseDir, "master", v))
		}
	}
}

// cleanupLegacy removes what masters before 0.2 stored: separate modules
// and agents that need them.
func (s *Server) cleanupLegacy() {
	os.RemoveAll(filepath.Join(s.cfg.ReleaseDir, "modules"))
	os.RemoveAll(filepath.Join(s.cfg.DataDir, "staging"))
	s.pruneCore(func(v string) bool { return versionLess(v, minAgentVersion) })
}

// ---- UI handlers ----

func (s *Server) releasesView() map[string]any {
	ml := s.repo.masterLatest()
	return map[string]any{
		"index": s.idx.status(),
		"repo":  s.repo.summary(),
		"master": map[string]any{
			"version": s.cfg.Version,
			"latest":  ml,
			"update":  ml != "" && versionLess(s.cfg.Version, ml),
		},
	}
}

func (s *Server) handleReleases(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.releasesView())
}

func (s *Server) handleReleasesCheck(w http.ResponseWriter, r *http.Request) {
	err := s.syncIndex()
	s.audit.write(actor(r), clientIP(r), "releases.check", "", errString(err))
	writeJSON(w, s.releasesView())
}

func errString(err error) string {
	if err == nil {
		return "ok"
	}
	return err.Error()
}
