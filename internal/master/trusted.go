package master

// Trusted networks (Settings > General, Website & sign-in): users with 2FA
// sign in from them without the code step (the password is always needed;
// turning 2FA off and backup downloads still ask for the code). Networks in
// the "never trust" list are never trusted, also when a wider trusted
// network contains them: e.g. a reverse proxy or VPN server, through which
// every sign-in would otherwise look trusted.

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

// parseTrusted reads the trusted networks: too wide ranges are refused.
func parseTrusted(text string) ([]string, error) { return parseNets(text, true, "trusted") }

// parseNever reads the never-trusted networks: any width is fine, they
// only take trust away.
func parseNever(text string) ([]string, error) { return parseNets(text, false, "never-trusted") }

// parseNets reads a comma separated list of networks; a bare address is
// one host.
func parseNets(text string, limitWidth bool, what string) ([]string, error) {
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
		if min := map[bool]int{true: 8, false: 32}[p.Addr().Is4()]; limitWidth && p.Bits() < min {
			return nil, fmt.Errorf("%s is too wide: at most /%d", p, min)
		}
		if !seen[p.String()] {
			seen[p.String()] = true
			out = append(out, p.String())
		}
	}
	if len(out) > maxTrusted {
		return nil, fmt.Errorf("at most %d %s networks", maxTrusted, what)
	}
	return out, nil
}

func inNets(nets []string, a netip.Addr) bool {
	for _, n := range nets {
		if p, err := netip.ParsePrefix(n); err == nil && p.Contains(a) {
			return true
		}
	}
	return false
}

func parseIP(ip string) (netip.Addr, bool) {
	a, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap().WithZone(""), true
}

// trustedIP: sign-ins from ip skip the 2FA code step. Never-trust wins.
func trustedIP(set Settings, ip string) bool {
	a, ok := parseIP(ip)
	if set.TFAMode != tfaUntrusted || !ok || inNets(set.TFANever, a) {
		return false
	}
	return inNets(set.TFATrusted, a)
}

// neverIP: ip is in the never-trust list.
func neverIP(set Settings, ip string) bool {
	a, ok := parseIP(ip)
	return ok && inNets(set.TFANever, a)
}

// neverUnused: never-trust entries that touch no trusted network, so have
// no effect.
func neverUnused(set Settings) []string {
	out := []string{}
	for _, n := range set.TFANever {
		p, err := netip.ParsePrefix(n)
		if err != nil {
			continue
		}
		used := false
		for _, t := range set.TFATrusted {
			if q, err := netip.ParsePrefix(t); err == nil && q.Overlaps(p) {
				used = true
				break
			}
		}
		if !used {
			out = append(out, n)
		}
	}
	return out
}

// tfaView: what the page shows about the current address and the lists.
func tfaView(set Settings, ip string) map[string]any {
	return map[string]any{"client_ip": ip, "client_trusted": trustedIP(set, ip), "client_never": neverIP(set, ip), "tfa_never_unused": neverUnused(set)}
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
