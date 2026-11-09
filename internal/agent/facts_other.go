//go:build !linux

package agent

import (
	"os"
	"runtime"

	"coremon/internal/proto"
)

// Non-Linux builds exist only for development and testing.
func collectFacts() proto.Facts {
	f := baseFacts()
	f.OSName = runtime.GOOS + " (development build)"
	f.OSID = runtime.GOOS
	f.Virt = "none"
	if os.Getenv("COREMON_FAKE") == "1" { // simulated network facts for UI tests
		f.Gateway, f.GatewayIf = "192.168.5.1", "eth0"
		f.DNS, f.DNSVia = []string{"192.168.5.1", "1.1.1.1"}, "systemd-resolved"
	}
	return f
}
