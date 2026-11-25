package updates

// The zypper backend (SUSE Linux Enterprise, openSUSE Leap and Tumbleweed).
// Leap and SLES are upgraded with "zypper update"; Tumbleweed, a rolling
// release, with "zypper dup". Security updates: the packages a waiting
// (needed) security patch replaces. Tumbleweed has no patches: the master
// decides from the CVEs it knows (SecurityFromCVE).

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"coremon/internal/feature"
)

type zypperBackend struct{ rolling bool }

// ZYPP_LOCK_TIMEOUT: wait for another zypper instead of failing at once.
var zypperEnv = []string{"LANG=C", "LC_ALL=C", "ZYPP_LOCK_TIMEOUT=300"}

func (zypperBackend) Refresh(c *feature.Ctx) error {
	_, err := c.Run(zypperEnv, "zypper", "--non-interactive", "refresh")
	return err
}

func (z zypperBackend) List(c *feature.Ctx) (*Snapshot, error) {
	inst, err := c.Output(zypperEnv, "rpm", "-qa", "--qf", `%{NAME}|%{ARCH}|%{EPOCHNUM}:%{VERSION}-%{RELEASE}|%{SOURCERPM}\n`)
	if err != nil {
		return nil, err
	}
	var up string
	if z.rolling {
		up, err = c.Output(zypperEnv, "zypper", "--non-interactive", "--xmlout", "dup", "--dry-run")
	} else {
		up, err = c.Output(zypperEnv, "zypper", "--non-interactive", "--xmlout", "list-updates")
	}
	if err != nil && up == "" {
		return nil, err
	}
	info := ""
	if !z.rolling {
		lp, _ := c.Output(zypperEnv, "zypper", "--non-interactive", "--xmlout", "list-patches", "--category", "security")
		names := neededPatches(lp)
		for len(names) > 0 { // in batches: keep the command line short
			n := min(len(names), 100)
			args := append([]string{"--non-interactive", "info", "-t", "patch", "--"}, names[:n]...)
			out, _ := c.Output(zypperEnv, "zypper", args...)
			info += out + "\n"
			names = names[n:]
		}
	}
	repos, _ := c.Output(zypperEnv, "zypper", "--non-interactive", "--xmlout", "repos")
	s := parseZypper(inst, up, info)
	s.Backend = "zypper"
	s.SecurityFromCVE = z.rolling
	s.NoSources = strings.Contains(repos, "<stream") && zypperRepos(repos) == 0 // only when zypper answered
	s.RefreshedAt = newestMtime("/var/cache/zypp/solv/*/solv")
	s.RebootRequired, s.RebootPkgs = rebootRequired()
	// zypper's own verdict: exit code 102 when a reboot is needed; libzypp
	// also leaves /run/reboot-needed after such an update.
	_, nerr := c.Output(zypperEnv, "zypper", "--non-interactive", "needs-rebooting")
	_, ferr := os.Stat("/run/reboot-needed")
	if exitCode(nerr) == 102 || ferr == nil {
		s.RebootRequired = true
		if len(s.RebootPkgs) == 0 {
			s.RebootPkgs = append(s.RebootPkgs, "core libraries or services were updated")
		}
	}
	return s, nil
}

func (z zypperBackend) Install(c *feature.Ctx, pkgs []string) error {
	args := append([]string{"--non-interactive", "update", "--"}, pkgs...)
	_, err := c.Run(zypperEnv, "zypper", args...)
	return err
}

func (z zypperBackend) UpgradeAll(c *feature.Ctx) error {
	cmd := "update"
	if z.rolling {
		cmd = "dup"
	}
	_, err := c.Run(zypperEnv, "zypper", "--non-interactive", cmd)
	return err
}

type zypperPkg struct {
	name, edition, old, arch, repo string
	install                        bool // newly installed (a dependency)
}

// xmlTag: one tag of zypper's XML output.
type xmlTag struct {
	name  string
	attr  map[string]string
	end   bool // </name>
	empty bool // <name … />
}

var xmlEntities = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'", "&amp;", "&")

// xmlTags: the tags of a simple XML document (zypper's --xmlout), in order;
// text, comments and declarations are skipped.
func xmlTags(s string) []xmlTag {
	var out []xmlTag
	for {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			return out
		}
		s = s[i+1:]
		if strings.HasPrefix(s, "!--") {
			if j := strings.Index(s, "-->"); j >= 0 {
				s = s[j+3:]
				continue
			}
			return out
		}
		j := strings.IndexByte(s, '>')
		if j < 0 {
			return out
		}
		body := s[:j]
		s = s[j+1:]
		if body == "" || body[0] == '?' || body[0] == '!' {
			continue
		}
		t := xmlTag{attr: map[string]string{}}
		if body[0] == '/' {
			t.end, body = true, body[1:]
		}
		if strings.HasSuffix(body, "/") {
			t.empty, body = true, body[:len(body)-1]
		}
		n := strings.IndexAny(body, " \t\r\n")
		if n < 0 {
			n = len(body)
		}
		t.name, body = body[:n], body[n:]
		for {
			body = strings.TrimLeft(body, " \t\r\n")
			eq := strings.IndexByte(body, '=')
			if eq <= 0 || eq+1 >= len(body) {
				break
			}
			k, q := strings.TrimSpace(body[:eq]), body[eq+1]
			if q != '"' && q != '\'' {
				break
			}
			end := strings.IndexByte(body[eq+2:], q)
			if end < 0 {
				break
			}
			t.attr[k] = xmlEntities.Replace(body[eq+2 : eq+2+end])
			body = body[eq+3+end:]
		}
		out = append(out, t)
	}
}

// zypperPkgs reads zypper's XML: list-updates (<update kind="package">) or
// a dry run's install summary (<solvable type="package"> under to-upgrade,
// to-install, …).
func zypperPkgs(out string) []zypperPkg {
	var list []zypperPkg
	var stack []string
	cur := -1
	in := func(name string) bool {
		for _, s := range stack {
			if s == name {
				return true
			}
		}
		return false
	}
	for _, t := range xmlTags(out) {
		if t.end {
			for len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if top == t.name {
					break
				}
			}
			if t.name == "update" || t.name == "solvable" {
				cur = -1
			}
			continue
		}
		switch {
		case t.name == "update" && t.attr["kind"] == "package",
			t.name == "solvable" && t.attr["type"] == "package" && (in("to-upgrade") || in("to-install") || in("to-downgrade")):
			list = append(list, zypperPkg{name: t.attr["name"], edition: t.attr["edition"], old: t.attr["edition-old"],
				arch: t.attr["arch"], repo: t.attr["repository"], install: in("to-install")})
			cur = len(list) - 1
		case t.name == "source" && cur >= 0 && list[cur].repo == "":
			list[cur].repo = t.attr["alias"]
		}
		if t.empty {
			if t.name == "update" || t.name == "solvable" {
				cur = -1
			}
		} else {
			stack = append(stack, t.name)
		}
	}
	return list
}

// neededPatches: the security patches "zypper --xmlout list-patches" lists
// as needed (its XML lists all categories, despite --category).
func neededPatches(out string) []string {
	var names []string
	seen := map[string]bool{}
	for _, t := range xmlTags(out) {
		n := t.attr["name"]
		if t.end || t.name != "update" || t.attr["kind"] != "patch" || t.attr["status"] != "needed" ||
			t.attr["category"] != "security" ||
			n == "" || seen[n] || !PackageNameRe.MatchString(n) {
			continue
		}
		seen[n] = true
		names = append(names, n)
	}
	return names
}

// patchFixes reads "zypper info -t patch": a patch's Conflicts lines
// ("libsystemd0.x86_64 < 257.13-160000.2.1", or without the arch) name the
// package versions it replaces. Key name.arch, or name for any arch; value
// the highest fixed version.
func patchFixes(info string) map[string]string {
	fix := map[string]string{}
	in := false
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || (line[0] != ' ' && line[0] != '\t') {
			k, _, ok := strings.Cut(line, ":")
			in = ok && strings.TrimSpace(k) == "Conflicts"
			continue
		}
		f := strings.Fields(line)
		if !in || len(f) != 3 || f[1] != "<" || strings.Contains(f[0], ":") {
			continue
		}
		k, v := f[0], stripEpoch0(f[2])
		k = strings.TrimSuffix(k, ".noarch") // also listed for every real arch; counts for any
		if old, ok := fix[k]; !ok || RPMVerCmp(v, old) > 0 {
			fix[k] = v
		}
	}
	return fix
}

func parseZypper(inst, up, info string) *Snapshot {
	cur, src := map[string]string{}, map[string]string{}
	for _, f := range qfLines(inst) {
		k, v := f[0]+"."+f[1], stripEpoch0(f[2])
		if old, ok := cur[k]; !ok || RPMVerCmp(v, old) > 0 {
			cur[k] = v
		}
		if len(f) > 3 {
			src[f[0]] = srpmName(f[3])
		}
	}
	fixes := patchFixes(info)
	// fixed: a waiting security patch replaces this installed version.
	fixed := func(name, arch, current string) bool {
		for _, k := range []string{name + "." + arch, name} {
			if v, ok := fixes[k]; ok && current != "" && RPMVerCmp(current, v) < 0 {
				return true
			}
		}
		return false
	}
	s := &Snapshot{}
	seen := map[string]bool{}
	for _, p := range zypperPkgs(up) {
		k := p.name + "." + p.arch
		if p.name == "" || seen[k] || !PackageNameRe.MatchString(p.name) {
			continue
		}
		seen[k] = true
		pk := Package{Name: p.name, Arch: p.arch, Available: p.edition, Origin: p.repo, Current: p.old, Source: src[p.name]}
		if pk.Current == "" {
			pk.Current = cur[k]
		}
		pk.New = p.install || pk.Current == ""
		pk.Security = !pk.New && fixed(p.name, p.arch, pk.Current)
		s.Packages = append(s.Packages, pk)
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

// srpmName: "openssl-3-3.5.0-160000.4.1.src.rpm" → "openssl-3" (the
// name without version and release); "" for "(none)".
func srpmName(s string) string {
	s = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(s), ".src.rpm"), ".nosrc.rpm")
	for range 2 {
		i := strings.LastIndexByte(s, '-')
		if i <= 0 {
			return ""
		}
		s = s[:i]
	}
	return s
}

// zypperRepos: the number of enabled repositories in "zypper --xmlout repos".
func zypperRepos(out string) int {
	n := 0
	for _, t := range xmlTags(out) {
		if !t.end && t.name == "repo" && t.attr["enabled"] == "1" {
			n++
		}
	}
	return n
}

// exitCode: the exit status of a finished command, -1 when it did not run.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

func newestMtime(glob string) int64 {
	var newest int64
	files, _ := filepath.Glob(glob)
	for _, f := range files {
		if st, err := os.Stat(f); err == nil && st.ModTime().Unix() > newest {
			newest = st.ModTime().Unix()
		}
	}
	return newest
}
