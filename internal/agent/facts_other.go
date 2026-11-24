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
		if v := os.Getenv("COREMON_FAKE_PVE"); v != "" {
			f.Platform = &proto.Platform{ID: "pve", Name: "Proxmox VE", Version: v}
		}
	}
	if os.Getenv("COREMON_FAKE_OS") != "" {
		f.OSID, f.OSVersion, _ = strings.Cut(os.Getenv("COREMON_FAKE_OS"), "/")
	}
	if v := os.Getenv("COREMON_FAKE_IPS"); v != "" {
		// Look like another system (for testing remote probes on one PC).
		f.IPs = strings.Split(v, ",")
	}
	if os.Getenv("COREMON_FAKE") == "1" {
		f.NIC = &proto.NIC{Iface: "vmbr0", Phys: "enp3s0", MAC: "bc:24:11:5e:0a:17"}
		for i, ip := range f.IPs {
			a := proto.AddrInfo{IP: ip, Prefix: 24, Iface: "vmbr0", Origin: "static"}
			if strings.Contains(ip, ":") {
				a.Prefix, a.Origin = 64, "slaac"
			} else if i > 0 {
				a.Origin = "dhcp"
			}
			f.Addrs = append(f.Addrs, a)
		}
	}
	return f
}
