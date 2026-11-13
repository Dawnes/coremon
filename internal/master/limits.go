package master

import (
	"net"
	"strings"
	"time"
	"unicode/utf8"

	"coremon/internal/proto"
)

func isIPv4(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil && !strings.Contains(s, ":")
}

const (
	maxPollBody    = 256 << 10
	maxEnrollBody  = 64 << 10
	maxFactLen     = 256
	maxIPs         = 32
	maxDNS         = 8
	maxFailedUnits = 10
	maxFeatures    = 16
	maxActions     = 32
	maxNameLen     = 64
)

func notFuture(t int64) int64 {
	if now := time.Now().Unix(); t > now {
		return now
	}
	return t
}

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

func sanitizeFacts(f proto.Facts) proto.Facts {
	for _, p := range []*string{&f.Hostname, &f.OSName, &f.OSID, &f.OSVersion, &f.Kernel, &f.OS, &f.Arch, &f.Virt} {
		*p = cleanText(*p, maxFactLen)
	}
	ips := []string{}
	for _, s := range f.IPs {
		if ip := net.ParseIP(s); ip != nil && len(ips) < maxIPs {
			ips = append(ips, ip.String())
		}
	}
	f.IPs = ips

	if !isIPv4(f.Gateway) {
		f.Gateway, f.GatewayIf = "", ""
	}
	f.GatewayIf = cleanText(f.GatewayIf, 32)
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
		c.At = notFuture(c.At)
		if c.FailedCount < -1 || c.FailedCount > 100000 {
			c.FailedCount = -1
		}
		if len(c.Failed) > maxFailedUnits {
			c.Failed = c.Failed[:maxFailedUnits]
		}
		names := make([]string, 0, len(c.Failed))
		for _, n := range c.Failed {
			if n = cleanText(n, 128); n != "" {
				names = append(names, n)
			}
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
			if user, ok := strings.CutPrefix(lu.By, "user:"); ok {
				lu.By = "user:" + cleanText(user, 32)
			} else if lu.By != "coremon" && lu.By != "automatic" && lu.By != "root" {
				lu.By = ""
			}
			lu.Time = notFuture(lu.Time)
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

func sanitizeFeatures(in []proto.FeatureInfo) []proto.FeatureInfo {
	if len(in) > maxFeatures {
		in = in[:maxFeatures]
	}
	out := make([]proto.FeatureInfo, 0, len(in))
	for _, f := range in {
		f.Name, f.Title = cleanText(f.Name, maxNameLen), cleanText(f.Title, maxNameLen)
		if len(f.Actions) > maxActions {
			f.Actions = f.Actions[:maxActions]
		}
		acts := make([]proto.ActionSpec, 0, len(f.Actions))
		for _, a := range f.Actions {
			a.Name, a.Title = cleanText(a.Name, maxNameLen), cleanText(a.Title, maxNameLen)
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
