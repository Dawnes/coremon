package master

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coremon/internal/probe"
	"coremon/internal/proto"
)

// Certificate Validity: the web address gives the name and port, the
// defaults follow the agreed design, and what the check saw is kept.
func TestCertificateCheck(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.st.mu.Lock()
	s.st.st.Locals["l_web"] = &LocalDevice{ID: "l_web", Name: "web", Checks: Checks{Address: "10.1.0.8"}}
	s.st.mu.Unlock()
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	d := s.st.st.Locals["l_web"]

	code, b := c.do("POST", "/api/ui/locals/l_web/services", `{"preset":"cert","host":"https://Shop.Example.com:8443/login","interval":3600}`)
	if code != 200 {
		t.Fatalf("add: %d %s", code, b)
	}
	sv := d.Services[0]
	if sv.Method != probe.MethodCert || sv.Host != "shop.example.com" || sv.Port != 8443 || !sv.CertValid || !sv.CertTime ||
		sv.CertDays != 10 || sv.Interval != 3600 || sv.Name != "Certificate Validity" {
		t.Fatalf("defaults: %+v", sv)
	}
	if st := probe.Steps(sv.spec()); strings.Join(st, ",") != "connect,tls,cert,cert_time,cert_days" {
		t.Errorf("steps %v", st)
	}
	// Renaming keeps the port of the web address.
	if code, b := c.do("POST", "/api/ui/locals/l_web/services/"+sv.ID, `{"preset":"cert","name":"Shop"}`); code != 200 || sv.Port != 8443 || sv.Name != "Shop" {
		t.Errorf("rename: %d %s %+v", code, b, sv)
	}
	for _, x := range []struct{ body, want string }{
		{`{"host":"http://shop.example.com"}`, "only https"},
		{`{"host":"shop.example.com:0"}`, "port"},
	} {
		if code, b := c.do("POST", "/api/ui/locals/l_web/services/"+sv.ID, x.body); code != 400 || !strings.Contains(b, x.want) {
			t.Errorf("%s: %d %s", x.body, code, b)
		}
	}
	if code, b := c.do("POST", "/api/ui/locals/l_web/services/"+sv.ID, `{"host":"example.org","cert_valid":false,"cert_time":true,"cert_days":0}`); code != 200 ||
		sv.Host != "example.org" || sv.Port != 443 || sv.CertValid || sv.CertDays != 0 {
		t.Errorf("change: %d %s %+v", code, b, sv)
	}

	// A remote probe gets the new settings.
	s.st.mu.Lock()
	d.Probe = "a_probe"
	l := s.probeList("a_probe")
	d.Probe = ""
	s.st.mu.Unlock()
	if len(l.Checks) != 2 || l.Checks[1].Kind != "cert" || !l.Checks[1].CertTime || l.Checks[1].Host != "example.org" {
		t.Errorf("probe list: %+v", l.Checks)
	}

	// A failed check keeps what the server presented, cleaned, in checks.dat.
	info := &proto.CertInfo{Addr: "93.184.216.34:443", TLS: "TLS 1.3", Chain: []proto.CertEntry{{Subject: "CN=example.org\x07", Names: []string{"example.org", "www.example.org"}, NotAfter: time.Now().Unix()}}}
	s.st.mu.Lock()
	s.loc.mu.Lock()
	s.recordService(sv, probe.Outcome{Failed: probe.StepCertDays, Err: errors.New("certificate expires in 3 days (needs 10)"), Cert: info}, time.Now())
	s.loc.mu.Unlock()
	js, _ := s.checksJSON()
	v := s.localDeviceView(d, true)
	s.st.mu.Unlock()
	got := v.Local.Services[0]
	if got.Cert == nil || got.Cert.Chain[0].Subject != "CN=example.org" || len(got.Cert.Chain[0].Names) != 2 || !got.CertTime {
		t.Errorf("view: %+v", got)
	}
	var cd checksData
	json.Unmarshal(js, &cd)
	if r := cd.Devices["l_web"].Services[sv.ID]; r.Cert == nil || r.Cert.TLS != "TLS 1.3" {
		t.Errorf("checks.dat: %+v", r)
	}
	// Another method drops the certificate settings and the hour heartbeat.
	if code, b := c.do("POST", "/api/ui/locals/l_web/services/"+sv.ID, `{"preset":"","method":"tcp","port":443}`); code != 200 || sv.CertTime || sv.Interval != defaultSvcInterval || sv.Cert != nil {
		t.Errorf("to tcp: %d %s %+v", code, b, sv)
	}
}
