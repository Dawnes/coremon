package master

// The right-hand block of Settings > General: showing received probe
// results, and trusted networks from which users with 2FA sign in without
// the code step (the password is always needed; turning 2FA off and backup
// downloads still ask for the code).

import (
	"fmt"
	"net/http"
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

// handleSettingsMore saves the right-hand block only.
func (s *Server) handleSettingsMore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProbeResultsShown bool   `json:"probe_results_shown"`
		TFAMode           string `json:"tfa_mode"`
		TFATrusted        string `json:"tfa_trusted"`
	}
	if !readJSON(w, r, &req, 8192) {
		return
	}
	if req.TFAMode != tfaAlways && req.TFAMode != tfaUntrusted {
		http.Error(w, "2FA mode must be always or untrusted", http.StatusBadRequest)
		return
	}
	nets, err := parseTrusted(req.TFATrusted)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(nets) == 0 {
		req.TFAMode = tfaAlways // no networks: always ask
	}
	ip := clientIP(r)
	s.st.mu.Lock()
	old := s.st.st.Settings
	set := &s.st.st.Settings
	set.ProbeResultsShown, set.TFAMode, set.TFATrusted = req.ProbeResultsShown, req.TFAMode, nets
	now := *set
	s.st.markDirty()
	s.st.mu.Unlock()
	if old.ProbeResultsShown != now.ProbeResultsShown {
		s.audit.write(actor(r), ip, "settings.probe_results", "", "show received probe results on Remote Probes: "+onOff(now.ProbeResultsShown))
	}
	if old.TFAMode != now.TFAMode || strings.Join(old.TFATrusted, ",") != strings.Join(now.TFATrusted, ",") {
		s.audit.write(actor(r), ip, "settings.2fa_trusted", "", fmt.Sprintf("ask 2FA code: %s; trusted networks: %s", tfaModeText(now.TFAMode), orNone(strings.Join(now.TFATrusted, ", "))))
	}
	writeJSON(w, map[string]any{"settings": now, "client_ip": ip, "client_trusted": trustedIP(now, ip)})
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
