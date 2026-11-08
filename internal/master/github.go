package master

// GitHub release mirroring. The master polls the configured repository's
// releases, downloads the release tarball, verifies every artifact's
// signature against the pinned release key, and only then adds it to the
// local repository. Rolling a version out to agents stays a manual step.

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var ghRepoRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}$`)
var versionRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.]+)?$`)

const ghCheckEvery = 6 * time.Hour

type ghAsset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt string    `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Assets      []ghAsset `json:"assets"`
}

type ghReleaseView struct {
	Version   string `json:"version"`
	Published string `json:"published"`
	URL       string `json:"url"`
	Imported  bool   `json:"imported"`
	HasAsset  bool   `json:"has_asset"`
}

type ghStatus struct {
	Repo      string          `json:"repo"`
	CheckedAt int64           `json:"checked_at"`
	Error     string          `json:"error,omitempty"`
	Latest    string          `json:"latest"`
	Releases  []ghReleaseView `json:"releases"`
	Importing string          `json:"importing,omitempty"`
	LastImport string         `json:"last_import,omitempty"`
}

type github struct {
	mu       sync.Mutex
	st       ghStatus
	raw      map[string]ghRelease // version -> release
	importMu sync.Mutex
	hc       *http.Client
	kick     chan struct{}
}

func newGitHub() *github {
	return &github{hc: &http.Client{Timeout: 15 * time.Minute}, kick: make(chan struct{}, 1), raw: map[string]ghRelease{}}
}

func (g *github) status() ghStatus {
	g.mu.Lock()
	defer g.mu.Unlock()
	s := g.st
	s.Releases = append([]ghReleaseView(nil), g.st.Releases...)
	if s.Releases == nil {
		s.Releases = []ghReleaseView{}
	}
	return s
}

func (s *Server) ghSettings() (repo string, auto bool) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	return s.st.st.Settings.GitHubRepo, s.st.st.Settings.GitHubAutoImport
}

func (s *Server) githubLoop(stop <-chan struct{}) {
	t := time.NewTimer(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		case <-s.gh.kick:
		}
		s.githubCheck()
		t.Reset(ghCheckEvery)
	}
}

func (s *Server) githubCheck() {
	repo, auto := s.ghSettings()
	if repo == "" {
		s.gh.mu.Lock()
		s.gh.st = ghStatus{}
		s.gh.mu.Unlock()
		return
	}
	rels, err := s.fetchReleases(repo)
	s.gh.mu.Lock()
	s.gh.st.Repo, s.gh.st.CheckedAt = repo, time.Now().Unix()
	if err != nil {
		s.gh.st.Error = err.Error()
		s.gh.mu.Unlock()
		log.Printf("github: %v", err)
		return
	}
	s.gh.st.Error = ""
	s.gh.raw = map[string]ghRelease{}
	var views []ghReleaseView
	for _, r := range rels {
		v := strings.TrimPrefix(r.TagName, "v")
		if r.Draft || r.Prerelease || !versionRe.MatchString(v) {
			continue
		}
		s.gh.raw[v] = r
		view := ghReleaseView{Version: v, Published: r.PublishedAt, URL: r.HTMLURL, Imported: s.repo.hasCoreVersion(v)}
		view.HasAsset = assetFor(r, v) != nil
		views = append(views, view)
	}
	sort.Slice(views, func(i, j int) bool { return versionLess(views[j].Version, views[i].Version) })
	s.gh.st.Releases = views
	s.gh.st.Latest = ""
	if len(views) > 0 {
		s.gh.st.Latest = views[0].Version
	}
	var toImport string
	if auto && len(views) > 0 && !views[0].Imported && views[0].HasAsset {
		toImport = views[0].Version
	}
	s.gh.mu.Unlock()
	if toImport != "" {
		if err := s.githubImport(toImport, "scheduler"); err != nil {
			log.Printf("github: auto-import %s: %v", toImport, err)
		}
	}
}

func assetFor(r ghRelease, version string) *ghAsset {
	want := "coremon-" + version + ".tar.gz"
	for i := range r.Assets {
		if r.Assets[i].Name == want {
			return &r.Assets[i]
		}
	}
	return nil
}

func (s *Server) ghGet(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "coremon-master/"+s.cfg.Version)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := s.gh.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s %s", url, resp.Status, strings.TrimSpace(string(b)))
	}
	return resp, nil
}

func (s *Server) fetchReleases(repo string) ([]ghRelease, error) {
	if !ghRepoRe.MatchString(repo) {
		return nil, fmt.Errorf("invalid repository %q (expected owner/name)", repo)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := s.ghGet(ctx, "https://api.github.com/repos/"+repo+"/releases?per_page=20")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var rels []ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&rels); err != nil {
		return nil, fmt.Errorf("decode releases: %w", err)
	}
	return rels, nil
}

// githubImport downloads one release, verifies it in a staging directory
// and merges it into the local repository.
func (s *Server) githubImport(version, actor string) (err error) {
	if !versionRe.MatchString(version) {
		return errors.New("invalid version")
	}
	if !s.gh.importMu.TryLock() {
		return errors.New("another import is running")
	}
	defer s.gh.importMu.Unlock()
	s.gh.mu.Lock()
	rel, ok := s.gh.raw[version]
	s.gh.st.Importing = version
	s.gh.mu.Unlock()
	defer func() {
		s.gh.mu.Lock()
		s.gh.st.Importing = ""
		if err != nil {
			s.gh.st.LastImport = fmt.Sprintf("%s failed: %v", version, err)
		} else {
			s.gh.st.LastImport = version + " imported"
			for i := range s.gh.st.Releases {
				if s.gh.st.Releases[i].Version == version {
					s.gh.st.Releases[i].Imported = true
				}
			}
		}
		s.gh.mu.Unlock()
		detail := "ok"
		if err != nil {
			detail = err.Error()
		}
		s.audit.write(actor, "", "github.import", version, detail)
	}()
	if !ok {
		return errors.New("release not found; check GitHub first")
	}
	asset := assetFor(rel, version)
	if asset == nil {
		return fmt.Errorf("release has no coremon-%s.tar.gz asset", version)
	}
	if s.repo.pub == nil {
		return errors.New("master has no release signing key")
	}

	staging := filepath.Join(s.cfg.DataDir, "staging", version)
	os.RemoveAll(staging)
	defer os.RemoveAll(staging)
	if err := os.MkdirAll(staging, 0o700); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	resp, err := s.ghGet(ctx, asset.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := extractRelease(io.LimitReader(resp.Body, 512<<20), "coremon-"+version+"/release/", staging); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	// Verify everything before it touches the live repository.
	check := &repo{dir: staging, pub: s.repo.pub}
	check.scan()
	if len(check.errors) > 0 {
		return fmt.Errorf("verification failed: %s", strings.Join(check.errors, "; "))
	}
	if !check.hasCoreVersion(version) {
		return fmt.Errorf("release contains no agent-core %s", version)
	}
	for _, sub := range []string{"core", "master", "modules"} {
		if err := copyTree(filepath.Join(staging, sub), filepath.Join(s.cfg.ReleaseDir, sub)); err != nil {
			return err
		}
	}
	s.repo.scan()
	log.Printf("github: imported release %s", version)
	return nil
}

// extractRelease writes regular files below prefix in a .tar.gz to dst.
func extractRelease(r io.Reader, prefix, dst string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	files := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := path.Clean(strings.TrimPrefix(h.Name, "./"))
		if !strings.HasPrefix(name+"/", prefix) || h.Typeflag != tar.TypeReg {
			continue
		}
		rel := strings.TrimPrefix(name, strings.TrimSuffix(prefix, "/"))
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" || strings.Contains(rel, "..") || path.IsAbs(rel) || h.Size > 256<<20 {
			return fmt.Errorf("refusing entry %q", h.Name)
		}
		if files++; files > 500 {
			return errors.New("too many files")
		}
		out := filepath.Join(dst, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, io.LimitReader(tr, h.Size))
		f.Close()
		if err != nil {
			return err
		}
	}
	if files == 0 {
		return errors.New("archive contains no release files")
	}
	return nil
}

func copyTree(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		tmp := target + ".tmp"
		if err := os.WriteFile(tmp, b, 0o644); err != nil {
			return err
		}
		return os.Rename(tmp, target)
	})
}

// ---- UI handlers ----

func (s *Server) handleGitHub(w http.ResponseWriter, r *http.Request) {
	repo, auto := s.ghSettings()
	writeJSON(w, map[string]any{
		"status":      s.gh.status(),
		"repo":        repo,
		"auto_import": auto,
		"master": map[string]any{
			"version": s.cfg.Version,
			"latest":  s.repo.masterLatest(),
			"update":  s.repo.masterLatest() != "" && versionLess(s.cfg.Version, s.repo.masterLatest()),
		},
	})
}

func (s *Server) handleGitHubCheck(w http.ResponseWriter, r *http.Request) {
	s.githubCheck()
	s.handleGitHub(w, r)
}

func (s *Server) handleGitHubImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version string `json:"version"`
	}
	if !readJSON(w, r, &req, 1024) {
		return
	}
	if err := s.githubImport(req.Version, actorAdmin); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.handleGitHub(w, r)
}
