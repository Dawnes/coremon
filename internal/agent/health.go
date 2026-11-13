package agent

// System health: failed services, automatic updates, last upgrade.

import (
	"bufio"
	"strings"
	"sync"
	"time"

	"coremon/internal/proto"
)

// System health: failed services, automatic updates, last upgrade. The
// parsers take command output and file contents so they can be tested on
// any platform; collecting is in health_linux.go.

const (
	healthEvery = 5 * time.Minute
	maxFailed   = 10
)

var health struct {
	mu      sync.Mutex
	h       *proto.Health
	running bool
	stale   bool
}

func currentHealth() *proto.Health {
	health.mu.Lock()
	defer health.mu.Unlock()
	due := health.h == nil || health.stale || time.Since(time.Unix(health.h.At, 0)) >= healthEvery
	if due && !health.running {
		health.running = true
		go func() {
			h := collectHealth()
			health.mu.Lock()
			if h != nil {
				h.At = time.Now().Unix()
			}
			health.h, health.running, health.stale = h, false, false
			health.mu.Unlock()
		}()
	}
	return health.h
}

func healthStale() {
	health.mu.Lock()
	health.stale = true
	health.mu.Unlock()
}

func parseFailedUnits(out string) (count int, names []string) {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		count++
		if len(names) < maxFailed {
			names = append(names, clipStr(f[0], 128))
		}
	}
	return count, names
}

func dpkgInstalled(status, pkg string) bool {
	sc := bufio.NewScanner(strings.NewReader(status))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	in := false
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			in = false
		case strings.HasPrefix(line, "Package: "):
			in = strings.TrimSpace(line[9:]) == pkg
		case in && strings.HasPrefix(line, "Status: "):
			return strings.HasSuffix(strings.TrimSpace(line), " installed")
		}
	}
	return false
}

func aptPeriodicOn(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || k != "APT::Periodic::Unattended-Upgrade" {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `";`)
		return v != "" && v != "0" && !strings.EqualFold(v, "false")
	}
	return false
}

func autoUpdates(installed, periodic, timerActive bool) string {
	switch {
	case !installed:
		return "not_installed"
	case periodic && timerActive:
		return "on"
	}
	return "off"
}

func lastUpgrade(logs []string, loc *time.Location) *proto.Upgrade {
	var last *proto.Upgrade
	for _, l := range logs {
		var cmd, by, upg, end string
		flush := func() {
			if upg != "" && end != "" {
				if t, err := time.ParseInLocation("2006-01-02  15:04:05", end, loc); err == nil {
					u := &proto.Upgrade{Time: t.Unix(), Packages: strings.Count(upg, "("), By: upgradedBy(cmd, by)}
					if last == nil || u.Time >= last.Time {
						last = u
					}
				}
			}
			cmd, by, upg, end = "", "", "", ""
		}
		sc := bufio.NewScanner(strings.NewReader(l))
		sc.Buffer(make([]byte, 64<<10), 16<<20)
		for sc.Scan() {
			line := sc.Text()
			k, v, _ := strings.Cut(line, ": ")
			switch k {
			case "Start-Date":
				flush()
			case "Commandline":
				cmd = v
			case "Requested-By":
				by = v
			case "Upgrade":
				upg = v
			case "End-Date":
				end = strings.TrimSpace(v)
			}
		}
		flush()
	}
	return last
}

func upgradedBy(cmd, requestedBy string) string {
	switch {
	case strings.Contains(cmd, "Coremon::Run=1"):
		return "coremon"
	case strings.Contains(cmd, "unattended-upgrade"):
		return "automatic"
	case requestedBy != "":
		name, _, _ := strings.Cut(requestedBy, " (")
		return "user:" + clipStr(strings.TrimSpace(name), 32)
	}
	return "root"
}

func clipStr(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
