//go:build linux

package agent

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"coremon/internal/feature/apps"
	"coremon/internal/proto"
)

func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"LANG=C", "LC_ALL=C", "PATH=/usr/sbin:/usr/bin:/sbin:/bin", "SYSTEMD_COLORS=0"}
	out, err := cmd.Output()
	return string(out), err
}

// dnfAutomatic: automatic updates with dnf-automatic. The install timer
// always installs; the plain timer only when apply_updates is on.
func dnfAutomatic() string {
	active := func(unit string) bool {
		out, _ := run("systemctl", "is-active", unit)
		return strings.TrimSpace(out) == "active"
	}
	installed := false
	for _, dir := range []string{"/usr/lib/systemd/system/", "/lib/systemd/system/"} {
		if _, err := os.Stat(dir + "dnf-automatic.timer"); err == nil {
			installed = true
		}
	}
	conf, _ := os.ReadFile("/etc/dnf/automatic.conf")
	apply := dnfApplyOn(string(conf))
	return autoUpdates(installed, true, active("dnf-automatic-install.timer") || (apply && active("dnf-automatic.timer")))
}

func readGz(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		return ""
	}
	b, _ := io.ReadAll(io.LimitReader(z, 64<<20))
	return string(b)
}

func collectHealth() *proto.Health {
	h := &proto.Health{FailedCount: -1}
	if apps.OpenRC() {
		// OpenRC: the services whose daemon died ("crashed").
		if out, err := run("rc-status", "--crashed"); err == nil {
			h.FailedCount, h.Failed = parseFailedUnits(out) // one name per line
		}
		return h // no automatic-updates tool, no upgrade history
	}
	if out, err := run("systemctl", "list-units", "--state=failed", "--plain", "--no-legend", "--no-pager"); err == nil {
		h.FailedCount, h.Failed = parseFailedUnits(out)
	}
	status, _ := os.ReadFile("/var/lib/dpkg/status")
	installed := dpkgInstalled(string(status), "unattended-upgrades")
	periodic := false
	if out, err := run("apt-config", "dump", "APT::Periodic::Unattended-Upgrade"); err == nil {
		periodic = aptPeriodicOn(out)
	}
	active, _ := run("systemctl", "is-active", "apt-daily-upgrade.timer")
	h.AutoUpdates = autoUpdates(installed, periodic, strings.TrimSpace(active) == "active")
	if len(status) == 0 {
		h.AutoUpdates = ""
		if _, err := exec.LookPath("dnf"); err == nil {
			h.AutoUpdates = dnfAutomatic()
		}
	}
	cur, _ := os.ReadFile("/var/log/apt/history.log")
	h.LastUpgrade = lastUpgrade([]string{readGz("/var/log/apt/history.log.1.gz"), string(cur)}, time.Local)
	return h
}
