package master

// Trusted networks (Settings > General, Website & sign-in): users with 2FA
// sign in from them without the code step (the password is always needed; turning 2FA off and backup
// downloads still ask for the code).

import (
	"fmt"
	"net/netip"
	"strings"
)

const (
	tfaAlways    = ""          // always ask for the code
	tfaUntrusted = "untrusted" // only from networks not in TFATrusted
	maxTrusted   = 20
)

// parseTrusted reads a comma separated list of networks; a bare address
// is one host. Too wide ranges are refused.
func parseTrusted(text string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, f := range strings.Split(text, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		var p netip.Prefix
		if strings.Contains(f, "/") {
			var err error
			if p, err = netip.ParsePrefix(f); err != nil {
				return nil, fmt.Errorf("%q is not a network: use e.g. 192.168.5.0/24", f)
			}
		} else {
			a, err := netip.ParseAddr(f)
			if err != nil {
				return nil, fmt.Errorf("%q is not an IP address", f)
			}
			p = netip.PrefixFrom(a, a.BitLen())
		}
		if p.Addr().Zone() != "" {
			return nil, fmt.Errorf("%q: no zone allowed", f)
		}
		p = p.Masked()
		if p.Addr().Is4In6() {
			return nil, fmt.Errorf("%q: write IPv4 networks in IPv4 form", f)
		}
		if min := map[bool]int{true: 8, false: 32}[p.Addr().Is4()]; p.Bits() < min {
			return nil, fmt.Errorf("%s is too wide: at most /%d", p, min)
		}
		if !seen[p.String()] {
			seen[p.String()] = true
			out = append(out, p.String())
		}
	}
	if len(out) > maxTrusted {
		return nil, fmt.Errorf("at most %d trusted networks", maxTrusted)
	}
	return out, nil
}

// trustedIP: sign-ins from ip skip the 2FA code step.
func trustedIP(set Settings, ip string) bool {
	if set.TFAMode != tfaUntrusted {
		return false
	}
	a, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return false
	}
	a = a.Unmap().WithZone("")
	for _, n := range set.TFATrusted {
		if p, err := netip.ParsePrefix(n); err == nil && p.Contains(a) {
			return true
		}
	}
	return false
}

func tfaModeText(m string) string {
	if m == tfaUntrusted {
		return "from untrusted networks"
	}
	return "always"
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
