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
	"coremon/internal/sign"
)

// Release layout (produced by the build script, signed on the build machine):
//
//	release/core/<version>/<os>-<arch>/coremon-agent        (+ .sig)
//	release/master/<version>/<os>-<arch>/coremon-master     (+ .sig)
//	release/modules/<name>/<version>/<os>-<arch>/<name>     (+ .sig)

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
	core    map[string]map[string]*artifactEntry            // version -> platform -> entry
	masters map[string]map[string]*artifactEntry            // version -> platform -> entry
	modules map[string]map[string]map[string]*artifactEntry // name -> version -> platform -> entry
	errors  []string
}

var segRe = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,64}$`)
var platRe = regexp.MustCompile(`^([a-z0-9]+)-([a-z0-9]+)$`)

func (r *repo) scan() {
	core := map[string]map[string]*artifactEntry{}
	masters := map[string]map[string]*artifactEntry{}
	mods := map[string]map[string]map[string]*artifactEntry{}
	var errs []string

	add := func(kind, name, version, plat, file string) {
		pm := platRe.FindStringSubmatch(plat)
		if pm == nil || !segRe.MatchString(version) || !segRe.MatchString(name) {
			return
		}
		st, err := os.Stat(file)
		if err != nil || !st.Mode().IsRegular() {
			return
		}
		e := &artifactEntry{Path: file, Size: st.Size()}
		e.Kind, e.Name, e.Version, e.OS, e.Arch = kind, name, version, pm[1], pm[2]
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
			} else if !sign.Verify(r.pub, sign.Message(kind, name, version, e.OS, e.Arch, sum), e.Signature) {
				e.Error = "signature does not verify"
			}
		}
		if e.Error != "" {
			errs = append(errs, fmt.Sprintf("%s %s %s %s: %s", kind, name, version, plat, e.Error))
			return
		}
		if kind == "master" {
			if masters[version] == nil {
				masters[version] = map[string]*artifactEntry{}
			}
			masters[version][plat] = e
		} else if kind == "core" {
			e.URL = fmt.Sprintf("/api/agent/download/core/%s/%s/%s/%s", name, version, e.OS, e.Arch)
			if core[version] == nil {
				core[version] = map[string]*artifactEntry{}
			}
			core[version][plat] = e
		} else {
			e.URL = fmt.Sprintf("/api/agent/download/module/%s/%s/%s/%s", name, version, e.OS, e.Arch)
			if mods[name] == nil {
				mods[name] = map[string]map[string]*artifactEntry{}
			}
			if mods[name][version] == nil {
				mods[name][version] = map[string]*artifactEntry{}
			}
			mods[name][version][plat] = e
		}
	}

	versions, _ := filepath.Glob(filepath.Join(r.dir, "core", "*", "*", "coremon-agent"))
	for _, f := range versions {
		plat := filepath.Base(filepath.Dir(f))
		ver := filepath.Base(filepath.Dir(filepath.Dir(f)))
		add("core", "coremon-agent", ver, plat, f)
	}
	mfiles, _ := filepath.Glob(filepath.Join(r.dir, "modules", "*", "*", "*", "*"))
	for _, f := range mfiles {
		plat := filepath.Base(filepath.Dir(f))
		ver := filepath.Base(filepath.Dir(filepath.Dir(f)))
		name := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(f))))
		if filepath.Base(f) != name {
			continue
		}
		add("module", name, ver, plat, f)
	}
	mbins, _ := filepath.Glob(filepath.Join(r.dir, "master", "*", "*", "coremon-master"))
	for _, f := range mbins {
		plat := filepath.Base(filepath.Dir(f))
		ver := filepath.Base(filepath.Dir(filepath.Dir(f)))
		add("master", "coremon-master", ver, plat, f)
	}

	r.mu.Lock()
	r.core, r.modules, r.masters, r.errors = core, mods, masters, errs
	r.mu.Unlock()
}

func (r *repo) masterLatest() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return latest(r.masters)
}

func (r *repo) hasCoreVersion(v string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.core[v]) > 0
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

func (r *repo) coreLatest() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return latest(r.core)
}

func (r *repo) moduleLatest(name string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return latest(r.modules[name])
}

func (r *repo) find(kind, name, version, goos, goarch string) *artifactEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	plat := goos + "-" + goarch
	switch kind {
	case "core":
		return r.core[version][plat]
	case "master":
		return r.masters[version][plat]
	}
	return r.modules[name][version][plat]
}

type repoModule struct {
	Name      string   `json:"name"`
	Latest    string   `json:"latest"`
	Versions  []string `json:"versions"`
	Platforms []string `json:"platforms"`
}

type repoSummary struct {
	CoreLatest    string       `json:"core_latest"`
	CoreVersions  []string     `json:"core_versions"`
	CorePlatforms []string     `json:"core_platforms"`
	Modules       []repoModule `json:"modules"`
	Errors        []string     `json:"errors"`
	SigningKey    string       `json:"signing_key"`
	MasterLatest  string       `json:"master_latest"`
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
	s := repoSummary{CoreLatest: latest(r.core), CoreVersions: keysSorted(r.core), Errors: r.errors, Modules: []repoModule{}, MasterLatest: latest(r.masters)}
	if s.CoreLatest != "" {
		s.CorePlatforms = keysSorted(r.core[s.CoreLatest])
	}
	for name, vs := range r.modules {
		m := repoModule{Name: name, Latest: latest(vs), Versions: keysSorted(vs)}
		m.Platforms = keysSorted(vs[m.Latest])
		s.Modules = append(s.Modules, m)
	}
	sort.Slice(s.Modules, func(i, j int) bool { return s.Modules[i].Name < s.Modules[j].Name })
	if s.Errors == nil {
		s.Errors = []string{}
	}
	if r.pub != nil {
		s.SigningKey = base64.StdEncoding.EncodeToString(r.pub)
	}
	return s
}

var verSegRe = regexp.MustCompile(`\d+|[^\d]+`)

// versionLess compares versions numerically segment by segment.
func versionLess(a, b string) bool {
	pa, pb := verSegRe.FindAllString(a, -1), verSegRe.FindAllString(b, -1)
	for i := 0; i < len(pa) && i < len(pb); i++ {
		if pa[i] == pb[i] {
			continue
		}
		na, ea := atoi(pa[i])
		nb, eb := atoi(pb[i])
		if ea && eb {
			return na < nb
		}
		return pa[i] < pb[i]
	}
	return len(pa) < len(pb)
}

func atoi(s string) (int, bool) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}
