package master

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"coremon/internal/proto"
	"coremon/internal/release"
	"coremon/internal/sign"
)

// Local release store, filled from the version index:
//
//	release/core/<version>/<os>-<arch>/coremon-agent        (+ .sig)
//	release/master/<version>/<os>-<arch>/coremon-master     (+ .sig)

type artifactEntry struct {
	proto.Artifact
	Path  string `json:"-"`
	Size  int64  `json:"size"`
	Error string `json:"error,omitempty"`
}

type repo struct {
	mu      sync.RWMutex
	dir     string
	pub     ed25519.PublicKey
	core    map[string]map[string]*artifactEntry // version -> platform -> entry
	masters map[string]map[string]*artifactEntry // version -> platform -> entry
	errors  []string
}

var segRe = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,64}$`)

func binName(kind string) string {
	if kind == "master" {
		return "coremon-master"
	}
	return "coremon-agent"
}

func (r *repo) scan() {
	core := map[string]map[string]*artifactEntry{}
	masters := map[string]map[string]*artifactEntry{}
	var errs []string

	add := func(kind, version, plat, file string) {
		pm := release.PlatRe.FindStringSubmatch(plat)
		if pm == nil || !segRe.MatchString(version) {
			return
		}
		st, err := os.Stat(file)
		if err != nil || !st.Mode().IsRegular() {
			return
		}
		e := &artifactEntry{Path: file, Size: st.Size()}
		e.Kind, e.Name, e.Version, e.OS, e.Arch = kind, binName(kind), version, pm[1], pm[2]
		sum, err := sign.FileSHA256(file)
		if err != nil {
			e.Error = err.Error()
		}
		e.SHA256 = sum
		sig, err := os.ReadFile(file + ".sig")
		if err != nil {
			e.Error = "missing signature"
		} else {
			e.Signature = strings.TrimSpace(string(sig))
			if r.pub == nil {
				e.Error = "master has no signing public key"
			} else if !sign.Verify(r.pub, sign.Message(kind, e.Name, version, e.OS, e.Arch, sum), e.Signature) {
				e.Error = "signature does not verify"
			}
		}
		if e.Error != "" {
			errs = append(errs, fmt.Sprintf("%s %s %s: %s", kind, version, plat, e.Error))
			return
		}
		m := core
		if kind == "master" {
			m = masters
		} else {
			// Public: agents check the release signature and checksum.
			e.URL = fmt.Sprintf("/dl/core/%s/%s-%s", version, e.OS, e.Arch)
		}
		if m[version] == nil {
			m[version] = map[string]*artifactEntry{}
		}
		m[version][plat] = e
	}
	for _, kind := range []string{"core", "master"} {
		files, _ := filepath.Glob(filepath.Join(r.dir, kind, "*", "*", binName(kind)))
		for _, f := range files {
			plat := filepath.Base(filepath.Dir(f))
			ver := filepath.Base(filepath.Dir(filepath.Dir(f)))
			add(kind, ver, plat, f)
		}
	}
	r.mu.Lock()
	r.core, r.masters, r.errors = core, masters, errs
	r.mu.Unlock()
}

// latest returns the highest version among keys.
func latest[T any](m map[string]T) string {
	var vs []string
	for v := range m {
		vs = append(vs, v)
	}
	if len(vs) == 0 {
		return ""
	}
	sort.Slice(vs, func(i, j int) bool { return versionLess(vs[i], vs[j]) })
	return vs[len(vs)-1]
}

func (r *repo) masterLatest() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return latest(r.masters)
}

func (r *repo) coreLatest() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return latest(r.core)
}

func (r *repo) coreVersions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return keysSorted(r.core)
}

func (r *repo) find(kind, version, goos, goarch string) *artifactEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	plat := goos + "-" + goarch
	if kind == "master" {
		return r.masters[version][plat]
	}
	return r.core[version][plat]
}

func (r *repo) path(kind, version, plat string) string {
	return filepath.Join(r.dir, kind, version, plat, binName(kind))
}

type repoVersion struct {
	Version   string   `json:"version"`
	Platforms []string `json:"platforms"`
}

type repoSummary struct {
	CoreLatest   string        `json:"core_latest"`
	Core         []repoVersion `json:"core"` // newest first
	MasterLatest string        `json:"master_latest"`
	Errors       []string      `json:"errors"`
	SigningKey   string        `json:"signing_key"`
}

func keysSorted[T any](m map[string]T) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return versionLess(out[j], out[i]) })
	return out
}

func (r *repo) summary() repoSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := repoSummary{CoreLatest: latest(r.core), Core: []repoVersion{}, Errors: r.errors, MasterLatest: latest(r.masters)}
	for _, v := range keysSorted(r.core) {
		s.Core = append(s.Core, repoVersion{Version: v, Platforms: keysSorted(r.core[v])})
	}
	if s.Errors == nil {
		s.Errors = []string{}
	}
	if r.pub != nil {
		s.SigningKey = base64.StdEncoding.EncodeToString(r.pub)
	}
	return s
}

func versionLess(a, b string) bool { return release.VersionLess(a, b) }
