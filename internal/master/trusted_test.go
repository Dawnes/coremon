package master

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTrusted(t *testing.T) {
	got, err := parseTrusted(" 192.168.5.7/24, 10.0.0.7 ,fd00::1/64,, 192.168.5.0/24")
	if err != nil || strings.Join(got, ",") != "192.168.5.0/24,10.0.0.7/32,fd00::/64" {
		t.Fatalf("parse: %v %v", got, err)
	}
	for _, bad := range []string{"0.0.0.0/0", "10.0.0.0/7", "::/0", "2001::/31", "host.lan", "1.2.3.4/33", "::ffff:1.2.3.4"} {
		if _, err := parseTrusted(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	many := ""
	for i := 0; i < 21; i++ {
		many += "10.1." + itoa(i) + ".0/24,"
	}
	if _, err := parseTrusted(many); err == nil {
		t.Error("21 networks accepted")
	}
	set := Settings{TFAMode: tfaUntrusted, TFATrusted: []string{"192.168.5.0/24", "fd00::/64"}}
	for ip, want := range map[string]bool{"192.168.5.20": true, "192.168.6.1": false, "fd00::9": true, "::ffff:192.168.5.3": true, "junk": false} {
		if trustedIP(set, ip) != want {
			t.Errorf("%s: want %v", ip, want)
		}
	}
	set.TFAMode = tfaAlways
	if trustedIP(set, "192.168.5.20") {
		t.Error("trusted while always")
	}
}

func TestTrustedSignIn(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pwb, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	admin := newUIClient(t, srv.URL)
	admin.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pwb))+`"}`)
	admin.do("POST", "/api/ui/users", `{"name":"bob","role":"viewer","password":"secret1"}`)
	s.st.mu.Lock()
	s.st.userByName("bob").TOTPSecret = strings.Repeat("A", 32)
	s.st.mu.Unlock()
	signIn := func() string {
		_, body := newUIClient(t, srv.URL).do("POST", "/api/ui/login", `{"username":"bob","password":"secret1"}`)
		return body
	}
	if b := signIn(); !strings.Contains(b, "code_required") {
		t.Fatalf("code not asked: %s", b)
	}

	// From a trusted network: no code.
	if code, b := admin.do("POST", "/api/ui/settings/access", `{"ui_port":8443,"tfa_mode":"untrusted","tfa_trusted":"127.0.0.1"}`); code != 200 || !strings.Contains(b, `"client_trusted":true`) {
		t.Fatalf("save: %d %s", code, b)
	}
	if b := signIn(); strings.Contains(b, "code_required") || !strings.Contains(b, `"ok":true`) {
		t.Fatalf("trusted sign-in: %s", b)
	}
	found := false
	for _, e := range s.audit.list("", "", 100) {
		if e.Action == "login" && e.Detail == "2FA skipped: trusted network" {
			found = true
		}
	}
	if !found {
		t.Error("no audit note for the skipped code")
	}

	// Each block saves only its own settings.
	for _, c := range []struct{ block, body string }{
		{"agents", `{"heartbeat_sec":45,"check_interval_hours":12}`},
		{"lookups", `{"os_info_days":30,"vuln_days":7,"github_repo":"owner/name"}`},
		{"history", `{"retention_days":5,"probe_results_shown":true}`},
	} {
		if code, b := admin.do("POST", "/api/ui/settings/"+c.block, c.body); code != 200 {
			t.Fatalf("%s: %d %s", c.block, code, b)
		}
	}
	set := s.st.st.Settings
	if set.TFAMode != tfaUntrusted || len(set.TFATrusted) != 1 || set.UIPort != 8443 || set.HeartbeatSec != 45 || set.CheckIntervalHours != 12 ||
		set.OSInfoDays != 30 || set.VulnDays != 7 || set.GitHubRepo != "owner/name" || set.RetentionDays != 5 || !set.ProbeResultsShown {
		t.Errorf("blocks mixed up: %+v", set)
	}
	if code, _ := admin.do("POST", "/api/ui/settings/nothing", `{}`); code != 404 {
		t.Error("unknown block accepted")
	}
	acts := map[string]bool{}
	for _, e := range s.audit.list("", "", 100) {
		acts[e.Action] = true
	}
	for _, a := range []string{"settings.agents", "settings.access", "settings.lookups", "settings.history", "settings.2fa_trusted", "settings.probe_results"} {
		if !acts[a] {
			t.Errorf("no audit entry %s", a)
		}
	}

	// Networks kept while Always; an empty list is always Always.
	admin.do("POST", "/api/ui/settings/access", `{"ui_port":8443,"tfa_mode":"","tfa_trusted":"127.0.0.1"}`)
	if b := signIn(); !strings.Contains(b, "code_required") || len(s.st.st.Settings.TFATrusted) != 1 {
		t.Errorf("always: %s", b)
	}
	admin.do("POST", "/api/ui/settings/access", `{"ui_port":8443,"tfa_mode":"untrusted","tfa_trusted":" "}`)
	if s.st.st.Settings.TFAMode != tfaAlways {
		t.Error("empty list left on untrusted")
	}
	if code, _ := admin.do("POST", "/api/ui/settings/access", `{"ui_port":8443,"tfa_mode":"untrusted","tfa_trusted":"0.0.0.0/0"}`); code != 400 {
		t.Error("too wide accepted")
	}
}
