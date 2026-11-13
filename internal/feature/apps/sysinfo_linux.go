//go:build linux

package apps

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func run(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"LANG=C", "LC_ALL=C", "PATH=/usr/sbin:/usr/bin:/sbin:/bin", "SYSTEMD_COLORS=0"}
	out, _ := cmd.Output()
	return string(out)
}

func packageOrigins(pkgs []string) map[string]string {
	if _, err := exec.LookPath("apt-cache"); err != nil {
		return nil
	}
	sources := parseSources(run("apt-cache", "policy"))
	args := append([]string{"policy", "--"}, pkgs...)
	return parseOrigins(run("apt-cache", args...), sources)
}

func collectSystem() *System {
	sys := &System{Active: map[string]bool{}, Enabled: map[string]bool{}, Ports: map[string][]Port{}}
	parseListUnits(run("systemctl", "list-units", "--all", "--plain", "--no-legend", "--no-pager", "--type=service,socket"), sys.Active)
	parseUnitFiles(run("systemctl", "list-unit-files", "--plain", "--no-legend", "--no-pager", "--type=service,socket"), sys.Active)

	var units []string
	for u := range sys.Active {
		for _, p := range UnitPatterns() {
			if unitMatch(p, "*", u) {
				units = append(units, u)
				break
			}
		}
	}
	for u := range sys.Active {
		if !contains(units, u) {
			delete(sys.Active, u)
		}
	}
	if len(units) == 0 {
		return sys
	}
	sys.Enabled = parseIsEnabled(units, run("systemctl", append([]string{"is-enabled"}, units...)...))

	byInode := map[string]sock{}
	for _, f := range []struct{ file, proto string }{{"tcp", "tcp"}, {"tcp6", "tcp"}, {"udp", "udp"}, {"udp6", "udp"}} {
		b, _ := os.ReadFile("/proc/net/" + f.file)
		for _, s := range parseNetProc(string(b), f.proto) {
			byInode[s.inode] = s
		}
	}
	if len(byInode) > 0 {
		pids, _ := filepath.Glob("/proc/[0-9]*")
		for _, pid := range pids {
			fds, _ := os.ReadDir(pid + "/fd")
			unit := ""
			for _, fd := range fds {
				link, err := os.Readlink(pid + "/fd/" + fd.Name())
				if err != nil || !strings.HasPrefix(link, "socket:[") {
					continue
				}
				s, ok := byInode[strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")]
				if !ok {
					continue
				}
				if unit == "" {
					cg, _ := os.ReadFile(pid + "/cgroup")
					if unit = cgroupUnit(string(cg)); unit == "" {
						break
					}
				}
				if _, known := sys.Active[unit]; known {
					sys.Ports[unit] = append(sys.Ports[unit], s.toPort())
				}
				delete(byInode, s.inode)
			}
		}
	}

	var sockets []string
	for _, u := range units {
		if strings.HasSuffix(u, ".socket") {
			sockets = append(sockets, u)
		}
	}
	if len(sockets) > 0 {
		for u, ps := range parseListen(run("systemctl", append([]string{"show", "-p", "Id", "-p", "Listen"}, sockets...)...)) {
			sys.Ports[u] = append(sys.Ports[u], ps...)
		}
	}
	return sys
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
