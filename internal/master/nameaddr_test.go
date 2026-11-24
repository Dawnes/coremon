package master

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An agentless device known by a name: accepted in lower case, never an
// address conflict, sent to a probe as it is; the address its checks went
// to is shown, with the known device that has it.
func TestNameAddress(t *testing.T) {
	for in, want := range map[string]string{"NAS.Example.LAN.": "nas.example.lan", "nas": "nas", " 10.0.0.9 ": "10.0.0.9"} {
		if got, err := parseLocalAddr(in); err != nil || got != want {
			t.Errorf("parseLocalAddr(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"1.2.3", "bad name", "a..b", "x.localhost", "-x.lan"} {
		if _, err := parseLocalAddr(bad); err == nil {
			t.Errorf("parseLocalAddr(%q) accepted", bad)
		}
	}

	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.st.st.Agents["a_1"] = &Agent{ID: "a_1", Name: "web-01", Status: StatusActive}
	s.st.st.Agents["a_1"].Facts.IPs = []string{"192.168.5.41"}
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)

	for _, n := range []string{"shop", "shop2"} {
		if code, b := c.do("POST", "/api/ui/locals", `{"name":"`+n+`","address":"Shop.Example.org"}`); code != 200 {
			t.Fatalf("%s: %d %s", n, code, b)
		}
	}
	var d *LocalDevice
	for _, x := range s.st.st.Locals {
		d = x
	}
	if d.Address != "shop.example.org" {
		t.Fatalf("address %q", d.Address)
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	d.noteIP("192.168.5.41")
	v := s.localDeviceView(d, false)
	if v.Local.Resolved != "192.168.5.41" || v.Local.PointsTo == nil || v.Local.PointsTo.Name != "web-01" || len(v.Conflicts) != 0 {
		t.Errorf("view: resolved %q points to %+v conflicts %+v", v.Local.Resolved, v.Local.PointsTo, v.Conflicts)
	}
	d.Probe = "a_probe"
	if l := s.probeList("a_probe"); len(l.Checks) != 1 || l.Checks[0].Address != "shop.example.org" {
		t.Errorf("probe list: %+v", l.Checks)
	}
	// An IP address device never keeps a resolved address.
	ip := &Checks{Address: "10.0.0.9"}
	ip.noteIP("10.0.0.10")
	if ip.Resolved != "" {
		t.Error("resolved kept for an IP address")
	}
}
