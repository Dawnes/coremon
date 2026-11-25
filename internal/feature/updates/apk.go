package updates

// The apk backend (Alpine Linux). apk does not mark security updates: the
// master decides from the CVEs it knows (SecurityFromCVE).

import (
	"os"
	"sort"
	"strings"

	"coremon/internal/feature"
)

type apkBackend struct{}

var apkEnv = []string{"LANG=C", "LC_ALL=C"}

func (apkBackend) Refresh(c *feature.Ctx) error {
	_, err := c.Run(apkEnv, "apk", "update")
	return err
}

func (apkBackend) List(c *feature.Ctx) (*Snapshot, error) {
	out, err := c.Output(apkEnv, "apk", "version", "-l", "<")
	if err != nil && out == "" {
		return nil, err
	}
	s := parseApk(out)
	s.Backend, s.SecurityFromCVE = "apk", true
	s.RefreshedAt = newestMtime("/var/cache/apk/APKINDEX.*.tar.gz")
	if s.RefreshedAt == 0 {
		s.RefreshedAt = newestMtime("/lib/apk/db/*")
	}
	s.RebootRequired, s.RebootPkgs = apkReboot()
	return s, nil
}

func (apkBackend) Install(c *feature.Ctx, pkgs []string) error {
	args := append([]string{"upgrade"}, pkgs...)
	_, err := c.Run(apkEnv, "apk", args...)
	return err
}

func (apkBackend) UpgradeAll(c *feature.Ctx) error {
	_, err := c.Run(apkEnv, "apk", "upgrade")
	return err
}

// SplitApk: "curl-8.11.0-r2" → "curl", "8.11.0-r2" (the version is the
// last two parts: version and -rN).
func SplitApk(s string) (string, string, bool) {
	r := strings.LastIndexByte(s, '-')
	if r <= 0 || !strings.HasPrefix(s[r+1:], "r") {
		return "", "", false
	}
	v := strings.LastIndexByte(s[:r], '-')
	if v <= 0 {
		return "", "", false
	}
	return s[:v], s[v+1:], true
}

// parseApk reads "apk version -l '<'": "curl-8.11.0-r2   < 8.11.1-r0".
func parseApk(out string) *Snapshot {
	s := &Snapshot{}
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[1] != "<" {
			continue
		}
		name, cur, ok := SplitApk(f[0])
		if !ok || seen[name] || !PackageNameRe.MatchString(name) {
			continue
		}
		seen[name] = true
		s.Packages = append(s.Packages, Package{Name: name, Current: cur, Available: f[2], Origin: "Alpine"})
	}
	sort.Slice(s.Packages, func(i, j int) bool { return s.Packages[i].Name < s.Packages[j].Name })
	return s
}

// apkReboot: Alpine's kernel files carry no version (vmlinuz-lts), so a
// replaced kernel shows as the running kernel's modules being gone.
func apkReboot() (bool, []string) {
	if inContainer() {
		return false, nil
	}
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return false, nil
	}
	r := strings.TrimSpace(string(b))
	if _, err := os.Stat("/lib/modules/" + r); os.IsNotExist(err) {
		if _, err := os.Stat("/lib/modules"); err == nil {
			return true, []string{"kernel replaced (running " + r + ")"}
		}
	}
	return false, nil
}

// ApkVerCmp compares Alpine package versions (as apk does): -1, 0 or 1.
// Format: digits separated by dots, an optional letter, suffixes such as
// _alpha2, _rc1, _p3, _git…, then -rN.
func ApkVerCmp(a, b string) int {
	ra, rb := "", ""
	if i := strings.LastIndex(a, "-r"); i >= 0 {
		a, ra = a[:i], a[i+2:]
	}
	if i := strings.LastIndex(b, "-r"); i >= 0 {
		b, rb = b[:i], b[i+2:]
	}
	if c := apkMain(a, b); c != 0 {
		return c
	}
	return rpmNumCmp(numOr0(ra), numOr0(rb))
}

func numOr0(s string) string {
	if s == "" || strings.Trim(s, "0123456789") != "" {
		return "0"
	}
	return s
}

// apkSuffix: the order of the _suffixes; those before the release (alpha …
// rc) are older than none, those after (cvs … p) newer.
var apkSuffix = map[string]int{"alpha": -4, "beta": -3, "pre": -2, "rc": -1, "": 0, "cvs": 1, "svn": 2, "git": 3, "hg": 4, "p": 5}

func apkMain(a, b string) int {
	va, sa, _ := strings.Cut(a, "_")
	vb, sb, _ := strings.Cut(b, "_")
	// Numbers, then the letter.
	na, la := apkLetter(va)
	nb, lb := apkLetter(vb)
	pa, pb := strings.Split(na, "."), strings.Split(nb, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		if i >= len(pa) {
			return -1
		}
		if i >= len(pb) {
			return 1
		}
		if c := rpmNumCmp(numOr0(pa[i]), numOr0(pb[i])); c != 0 {
			return c
		}
	}
	if la != lb {
		if la < lb {
			return -1
		}
		return 1
	}
	// Suffixes one by one: _rc1_p2 …
	xa, xb := strings.Split(sa, "_"), strings.Split(sb, "_")
	if sa == "" {
		xa = nil
	}
	if sb == "" {
		xb = nil
	}
	for i := 0; i < len(xa) || i < len(xb); i++ {
		var ka, kb, da, db string
		if i < len(xa) {
			ka, da = apkSplitSuffix(xa[i])
		}
		if i < len(xb) {
			kb, db = apkSplitSuffix(xb[i])
		}
		if apkSuffix[ka] != apkSuffix[kb] {
			if apkSuffix[ka] < apkSuffix[kb] {
				return -1
			}
			return 1
		}
		if c := rpmNumCmp(numOr0(da), numOr0(db)); c != 0 {
			return c
		}
	}
	return 0
}

func apkLetter(v string) (string, string) {
	if n := len(v); n > 0 && v[n-1] >= 'a' && v[n-1] <= 'z' {
		return v[:n-1], v[n-1:]
	}
	return v, ""
}

func apkSplitSuffix(s string) (string, string) {
	i := 0
	for i < len(s) && (s[i] < '0' || s[i] > '9') {
		i++
	}
	return s[:i], s[i:]
}
