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

	"coremon/internal/proto"
)

// run runs a fixed, read-only command with a timeout.
func run(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = []string{"LANG=C", "LC_ALL=C", "PATH=/usr/sbin:/usr/bin:/sbin:/bin", "SYSTEMD_COLORS=0"}
	out, err := cmd.Output()
	return string(out), err
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
		h.AutoUpdates = "" // not a dpkg system: unknown
	}
	cur, _ := os.ReadFile("/var/log/apt/history.log")
	h.LastUpgrade = lastUpgrade([]string{readGz("/var/log/apt/history.log.1.gz"), string(cur)}, time.Local)
	return h
}
