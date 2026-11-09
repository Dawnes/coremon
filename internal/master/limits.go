package master

import (
	"net"
	"strings"
	"unicode/utf8"

	"coremon/internal/proto"
)

func isIPv4(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil && !strings.Contains(s, ":")
}

// Limits on what an agent may report. An enrolled system (even one still
// waiting for approval) must not be able to bloat the master's state.
const (
	maxPollBody    = 256 << 10 // heartbeat request
	maxEnrollBody  = 64 << 10
	maxFactLen     = 256 // any text fact
	maxIPs         = 32
	maxDNS         = 8
	maxFailedUnits = 10
	maxFeatures    = 16
	maxActions     = 32
	maxNameLen     = 64 // feature and action names, titles
)

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) && len(s) > 0 {
		s = s[:len(s)-1]
	}
	return s
}

// sanitizeFacts cuts reported facts to sane sizes.
func sanitizeFacts(f proto.Facts) proto.Facts {
	for _, p := range []*string{&f.Hostname, &f.OSName, &f.OSID, &f.OSVersion, &f.Kernel, &f.OS, &f.Arch, &f.Virt} {
		*p = clip(strings.TrimSpace(*p), maxFactLen)
	}
	if len(f.IPs) > maxIPs {
		f.IPs = f.IPs[:maxIPs]
	}
	for i := range f.IPs {
		f.IPs[i] = clip(f.IPs[i], 64)
	}
	// Network facts: IPv4 addresses only, a short interface name.
	if !isIPv4(f.Gateway) {
		f.Gateway, f.GatewayIf = "", ""
	}
	f.GatewayIf = clip(strings.TrimSpace(f.GatewayIf), 32)
	var dns []string
	for _, s := range f.DNS {
		if isIPv4(s) && len(dns) < maxDNS {
			dns = append(dns, s)
		}
	}
	f.DNS = dns
	if f.DNSVia != "systemd-resolved" {
		f.DNSVia = ""
	}
	if h := f.Health; h != nil {
		c := *h
		if c.FailedCount < -1 || c.FailedCount > 100000 {
			c.FailedCount = -1
		}
		if len(c.Failed) > maxFailedUnits {
			c.Failed = c.Failed[:maxFailedUnits]
		}
		names := make([]string, 0, len(c.Failed))
		for _, n := range c.Failed {
			names = append(names, clip(strings.TrimSpace(n), 128))
		}
		c.Failed = names
		switch c.AutoUpdates {
		case "on", "off", "not_installed":
		default:
			c.AutoUpdates = ""
		}
		if u := c.LastUpgrade; u != nil {
			lu := *u
			if lu.Packages < 0 || lu.Packages > 100000 {
				lu.Packages = 0
			}
			if lu.By != "coremon" && lu.By != "automatic" && lu.By != "root" && !strings.HasPrefix(lu.By, "user:") {
				lu.By = ""
			}
			lu.By = clip(lu.By, 40)
			c.LastUpgrade = &lu
		}
		f.Health = &c
	}
	if f.UptimeSec < 0 {
		f.UptimeSec = 0
	}
	if f.CPUs < 0 || f.CPUs > 1<<16 {
		f.CPUs = 0
	}
	return f
}

// sanitizeFeatures cuts the reported feature list to sane sizes.
func sanitizeFeatures(in []proto.FeatureInfo) []proto.FeatureInfo {
	if len(in) > maxFeatures {
		in = in[:maxFeatures]
	}
	out := make([]proto.FeatureInfo, 0, len(in))
	for _, f := range in {
		f.Name, f.Title = clip(f.Name, maxNameLen), clip(f.Title, maxNameLen)
		if len(f.Actions) > maxActions {
			f.Actions = f.Actions[:maxActions]
		}
		acts := make([]proto.ActionSpec, 0, len(f.Actions))
		for _, a := range f.Actions {
			a.Name, a.Title = clip(a.Name, maxNameLen), clip(a.Title, maxNameLen)
			if a.Timeout <= 0 || a.Timeout > 24*3600 {
				a.Timeout = 3600
			}
			acts = append(acts, a)
		}
		f.Actions = acts
		out = append(out, f)
	}
	return out
}
