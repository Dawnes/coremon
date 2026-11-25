package updates

// The dnf backend (Rocky Linux, AlmaLinux, RHEL). dnf 4 and dnf 5 take the
// same commands here; on these systems yum is dnf.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"coremon/internal/feature"
)

type dnfBackend struct{}

var dnfEnv = []string{"LANG=C.UTF-8", "LC_ALL=C.UTF-8"}

func (dnfBackend) Refresh(c *feature.Ctx) error {
	_, err := c.Run(dnfEnv, "dnf", "-q", "-y", "--refresh", "makecache")
	return err
}

// dnfQF: one line per package; dnf 5 needs the \n, dnf 4 adds its own.
const dnfQF = `%{name}|%{arch}|%{evr}|%{repoid}\n`

func (dnfBackend) List(c *feature.Ctx) (*Snapshot, error) {
	inst, err := c.Output(dnfEnv, "rpm", "-qa", "--qf", `%{NAME}|%{ARCH}|%{EPOCHNUM}:%{VERSION}-%{RELEASE}\n`)
	if err != nil {
		return nil, err
	}
	args := []string{"-q", "repoquery", "--upgrades", "--latest-limit=1", "--qf", dnfQF}
	up, err := c.Output(dnfEnv, "dnf", append([]string{"--cacheonly"}, args...)...)
	if err != nil { // no usable cache yet: dnf fetches the lists
		if up, err = c.Output(dnfEnv, "dnf", args...); err != nil {
			return nil, err
		}
	}
	// Security advisories for the waiting updates; none when the
	// repositories publish no advisories.
	adv, _ := c.Output(dnfEnv, "dnf", "-q", "--cacheonly", "updateinfo", "list", "--security")
	s := parseDnf(inst, up, adv)
	s.Backend = "dnf"
	s.RefreshedAt = dnfRefreshed()
	restart, _ := c.Output(dnfEnv, "dnf", "-q", "needs-restarting", "-r")
	s.RebootRequired, s.RebootPkgs = dnfReboot(restart)
	if flag, pkgs := rebootRequired(); flag {
		s.RebootRequired = true
		s.RebootPkgs = append(s.RebootPkgs, pkgs...)
	}
	return s, nil
}

func (dnfBackend) Install(c *feature.Ctx, pkgs []string) error {
	args := append([]string{"-y", "upgrade", "--"}, pkgs...)
	_, err := c.Run(dnfEnv, "dnf", args...)
	return err
}

func (dnfBackend) UpgradeAll(c *feature.Ctx) error {
	_, err := c.Run(dnfEnv, "dnf", "-y", "upgrade")
	return err
}

// stripEpoch0: "0:1.2-3" → "1.2-3", as dnf writes versions.
func stripEpoch0(v string) string { return strings.TrimPrefix(v, "0:") }

func qfLines(out string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), `\n`))
		if f := strings.Split(line, "|"); len(f) >= 3 && f[0] != "" {
			rows = append(rows, f)
		}
	}
	return rows
}

func parseDnf(inst, up, adv string) *Snapshot {
	cur := map[string]string{} // name.arch → newest installed version
	for _, f := range qfLines(inst) {
		k, v := f[0]+"."+f[1], stripEpoch0(f[2])
		if old, ok := cur[k]; !ok || RPMVerCmp(v, old) > 0 {
			cur[k] = v
		}
	}
	tokens := strings.Fields(adv)
	s := &Snapshot{}
	seen := map[string]bool{}
	for _, f := range qfLines(up) {
		name, arch, avail := f[0], f[1], stripEpoch0(f[2])
		if seen[name+"."+arch] {
			continue
		}
		seen[name+"."+arch] = true
		p := Package{Name: name, Arch: arch, Available: avail}
		if len(f) >= 4 {
			p.Origin = f[3]
		}
		p.Current = cur[name+"."+arch]
		p.New = p.Current == ""
		p.Security = advisoryFor(tokens, p)
		s.Packages = append(s.Packages, p)
	}
	sort.Slice(s.Packages, func(i, j int) bool {
		a, b := s.Packages[i], s.Packages[j]
		if a.Security != b.Security {
			return a.Security
		}
		return a.Name < b.Name
	})
	return s
}

// advisoryFor: a security advisory names a newer version of the package
// than the one installed (written name-[epoch:]version-release.arch).
func advisoryFor(tokens []string, p Package) bool {
	pre, suf := p.Name+"-", "."+p.Arch
	for _, t := range tokens {
		if !strings.HasPrefix(t, pre) || !strings.HasSuffix(t, suf) || len(t) <= len(pre)+len(suf) {
			continue
		}
		v := t[len(pre) : len(t)-len(suf)]
		if v[0] < '0' || v[0] > '9' { // another package: openssl-libs-… for openssl
			continue
		}
		if p.Current == "" || RPMVerCmp(stripEpoch0(v), p.Current) > 0 {
			return true
		}
	}
	return false
}

// dnfReboot reads "dnf needs-restarting -r": the updated core packages
// that want a reboot.
func dnfReboot(out string) (bool, []string) {
	if !strings.Contains(out, "Reboot is required") {
		return false, nil
	}
	var pkgs []string
	for _, line := range strings.Split(out, "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "* ") {
			if n := strings.TrimSpace(t[2:]); PackageNameRe.MatchString(n) {
				pkgs = append(pkgs, n)
			}
		}
	}
	return true, pkgs
}

// dnfRefreshed: when the package lists were last fetched.
func dnfRefreshed() int64 {
	var newest int64
	for _, g := range []string{"/var/cache/dnf/*/repodata/repomd.xml", "/var/cache/libdnf5/*/repodata/repomd.xml"} {
		files, _ := filepath.Glob(g)
		for _, f := range files {
			if st, err := os.Stat(f); err == nil && st.ModTime().Unix() > newest {
				newest = st.ModTime().Unix()
			}
		}
	}
	return newest
}
