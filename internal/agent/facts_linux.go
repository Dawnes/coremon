//go:build linux

package agent

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"coremon/internal/proto"
)

func collectFacts() proto.Facts {
	f := baseFacts()
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		f.Kernel = strings.TrimSpace(string(b))
	}
	if osr, err := os.Open("/etc/os-release"); err == nil {
		sc := bufio.NewScanner(osr)
		for sc.Scan() {
			k, v, ok := strings.Cut(sc.Text(), "=")
			if !ok {
				continue
			}
			v = strings.Trim(v, `"'`)
			switch k {
			case "PRETTY_NAME":
				f.OSName = v
			case "ID":
				f.OSID = v
			case "VERSION_ID":
				f.OSVersion = v
			}
		}
		osr.Close()
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		if fs := strings.Fields(string(b)); len(fs) > 0 {
			v, _ := strconv.ParseFloat(fs[0], 64)
			f.UptimeSec = int64(v)
		}
	}
	// Assigned memory. Inside an LXC container, lxcfs shows the container's
	// limit here, not the host's memory.
	if m, err := os.Open("/proc/meminfo"); err == nil {
		sc := bufio.NewScanner(m)
		for sc.Scan() {
			fs := strings.Fields(sc.Text())
			if len(fs) >= 2 && fs[0] == "MemTotal:" {
				v, _ := strconv.ParseUint(fs[1], 10, 64)
				f.MemTotal = v * 1024
				break
			}
		}
		m.Close()
	}
	f.Virt = detectVirt()
	read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }
	f.Gateway, f.GatewayIf = defaultGateway(read("/proc/net/route"))
	f.DNS, f.DNSVia = dnsServers(read)
	f.Health = currentHealth()
	return f
}

var virtCache string

func detectVirt() string {
	if virtCache != "" {
		return virtCache
	}
	v := "unknown"
	if out, err := exec.Command("systemd-detect-virt").Output(); err == nil || len(out) > 0 {
		if s := strings.TrimSpace(string(out)); s != "" {
			v = s
		}
	}
	virtCache = v
	return v
}
