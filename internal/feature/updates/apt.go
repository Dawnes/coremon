package updates

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"coremon/internal/feature"
)

type aptBackend struct{}

var aptEnv = []string{
	"LANG=C.UTF-8", "LC_ALL=C.UTF-8",
	"DEBIAN_FRONTEND=noninteractive",
	"APT_LISTCHANGES_FRONTEND=none",
	"NEEDRESTART_MODE=a", // Ubuntu: never prompt for service restarts
	"NEEDRESTART_SUSPEND=1",
}

var aptOpts = []string{
	"-q", "-y",
	"-o", "Dpkg::Options::=--force-confdef",
	"-o", "Dpkg::Options::=--force-confold",
	"-o", "DPkg::Lock::Timeout=300",
	// Marks the run as Coremon's in apt's history log (no effect on apt).
	"-o", "Coremon::Run=1",
}

func (aptBackend) Name() string { return "apt" }

func (aptBackend) Refresh(c *feature.Ctx) error {
	_, err := c.Run(aptEnv, "apt-get", "update", "-q", "-o", "DPkg::Lock::Timeout=300")
	return err
}

func (aptBackend) List(c *feature.Ctx) (*Snapshot, error) {
	out, err := c.Output(aptEnv, "apt-get", "-s", "-o", "Debug::NoLocking=1", "dist-upgrade")
	if err != nil {
		return nil, err
	}
	s := parseAptSim(out)
	s.Backend = "apt"
	if st, err := os.Stat("/var/cache/apt/pkgcache.bin"); err == nil {
		s.RefreshedAt = st.ModTime().Unix()
	}
	s.RebootRequired, s.RebootPkgs = rebootRequired()
	return s, nil
}

func (aptBackend) Install(c *feature.Ctx, pkgs []string) error {
	args := append([]string{"install", "--only-upgrade"}, aptOpts...)
	args = append(args, "--")
	args = append(args, pkgs...)
	_, err := c.Run(aptEnv, "apt-get", args...)
	return err
}

func (aptBackend) UpgradeAll(c *feature.Ctx) error {
	args := append([]string{"dist-upgrade"}, aptOpts...)
	_, err := c.Run(aptEnv, "apt-get", args...)
	return err
}

// Inst libssl3 [3.0.11-1~deb12u2] (3.0.13-1~deb12u1 Debian:12.6/stable, Debian-Security:12/stable-security [amd64]) []
var instRe = regexp.MustCompile(`^Inst (\S+) (?:\[([^\]]+)\] )?\((\S+)(?: ([^\[\)]*?))?(?: \[([^\]]+)\])?\)`)
var remvRe = regexp.MustCompile(`^Remv (\S+)`)

func parseAptSim(out string) *Snapshot {
	s := &Snapshot{}
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if m := instRe.FindStringSubmatch(line); m != nil {
			origin := strings.TrimSpace(m[4])
			s.Packages = append(s.Packages, Package{
				Name:      m[1],
				Current:   m[2],
				Available: m[3],
				Origin:    origin,
				Arch:      m[5],
				Security:  isSecurityOrigin(origin),
				New:       m[2] == "",
			})
		} else if m := remvRe.FindStringSubmatch(line); m != nil {
			s.Removals = append(s.Removals, m[1])
		}
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

func isSecurityOrigin(origin string) bool {
	o := strings.ToLower(origin)
	return strings.Contains(o, "-security") || strings.Contains(o, "security:")
}

// rebootRequired checks the Debian/Ubuntu flag file and, outside containers,
// whether a newer kernel is installed than the one running.
func rebootRequired() (bool, []string) {
	var pkgs []string
	flag := false
	if _, err := os.Stat("/var/run/reboot-required"); err == nil {
		flag = true
		if b, err := os.ReadFile("/var/run/reboot-required.pkgs"); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if l = strings.TrimSpace(l); l != "" {
					pkgs = append(pkgs, l)
				}
			}
		}
	}
	if !inContainer() {
		if running, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
			r := strings.TrimSpace(string(running))
			if latest := latestKernel(); latest != "" && latest != r {
				flag = true
				pkgs = append(pkgs, "kernel "+latest+" (running "+r+")")
			}
		}
	}
	return flag, pkgs
}

func inContainer() bool {
	if _, err := os.Stat("/run/systemd/container"); err == nil {
		return true
	}
	if b, err := os.ReadFile("/proc/1/environ"); err == nil && strings.Contains(string(b), "container=") {
		return true
	}
	return false
}

func latestKernel() string {
	files, _ := filepath.Glob("/boot/vmlinuz-*")
	var vers []string
	for _, f := range files {
		vers = append(vers, strings.TrimPrefix(filepath.Base(f), "vmlinuz-"))
	}
	if len(vers) == 0 {
		return ""
	}
	sort.Slice(vers, func(i, j int) bool { return versionLess(vers[i], vers[j]) })
	return vers[len(vers)-1]
}

// versionLess compares kernel version strings numerically segment by segment.
var verSegRe = regexp.MustCompile(`\d+|[^\d]+`)

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
