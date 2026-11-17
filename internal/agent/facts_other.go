//go:build !linux

package agent

import (
	"os"
	"runtime"
	"strings"

	"coremon/internal/proto"
)

func collectFacts() proto.Facts {
	f := baseFacts()
	f.OSName = runtime.GOOS + " (development build)"
	f.OSID = runtime.GOOS
	f.Virt = "none"
	if os.Getenv("COREMON_FAKE") == "1" {
		f.Gateway, f.GatewayIf = "192.168.5.1", "eth0"
		f.DNS, f.DNSVia = []string{"192.168.5.1", "1.1.1.1"}, "systemd-resolved"
		f.Health = currentHealth()
		f.OSID, f.OSVersion = "debian", "11"
	}
	if os.Getenv("COREMON_FAKE_OS") != "" {
		f.OSID, f.OSVersion, _ = strings.Cut(os.Getenv("COREMON_FAKE_OS"), "/")
	}
	if v := os.Getenv("COREMON_FAKE_IPS"); v != "" {
		// Look like another system (for testing remote probes on one PC).
		f.IPs = strings.Split(v, ",")
	}
	return f
}
