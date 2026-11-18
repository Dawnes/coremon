package master

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coremon/internal/probe"
)

func TestServices(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	open := map[int]bool{22: true}
	s.svcCheck = func(ctx context.Context, addr string, spec probe.Service, timeout time.Duration) probe.Outcome {
		if spec.Method == probe.MethodHTTPS {
			return probe.Outcome{RTT: time.Millisecond, Detail: "certificate valid until 2026-01-01", Failed: probe.StepCertDays,
				Err: errors.New("certificate expires in 9 days (needs 14)")}
		}
		if open[spec.Port] {
			b := ""
			if spec.Greeting {
				b = "SSH-2.0-OpenSSH_9.2"
			}
			return probe.Outcome{RTT: 2 * time.Millisecond, Detail: b}
		}
		return probe.Outcome{Failed: probe.StepConnect, Err: errors.New("connection refused")}
	}
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	_, body := c.do("POST", "/api/ui/locals", `{"name":"fw","address":"10.0.0.1"}`)
	var dv agentView
	json.Unmarshal([]byte(body), &dv)
	id := dv.ID
	base := "/api/ui/locals/" + id

	// The ping cannot be paused or removed while nothing else would be checked.
	for _, st := range []string{"paused", "removed"} {
		if code, _ := c.do("POST", base+"/pingstate", `{"state":"`+st+`"}`); code != 409 {
			t.Errorf("ping %s without services", st)
		}
	}

	// A default service keeps its own port and greeting; a custom one needs a port.
	for body, want := range map[string]int{
		`{"preset":"ssh","port":2222,"greeting":false}`:                 200,
		`{"preset":"","name":"web-alt","port":8080,"reverse":true}`:     200,
		`{"preset":"","name":"no port"}`:                                400,
		`{"preset":"","name":"x","port":70000}`:                         400,
		`{"preset":"http","interval":30}`:                               400,
		`{"preset":"http","retries":11}`:                                400,
		`{"preset":"http","timeout":0}`:                                 400,
		`{"preset":"nope"}`:                                             400,
		`{"preset":"","name":"bad<name>","port":81}`:                    400,
		`{"preset":"","name":"telnet closed","port":23,"reverse":true}`: 200,
	} {
		if code, b := c.do("POST", base+"/services", body); code != want {
			t.Errorf("%s: %d %s, want %d", body, code, b, want)
		}
	}
	d := s.st.st.Locals[id]
	if len(d.Services) != 3 {
		t.Fatalf("services: %d", len(d.Services))
	}
	var ssh, web, tel *Service
	for _, sv := range d.Services {
		switch sv.Name {
		case "SSH":
			ssh = sv
		case "web-alt":
			web = sv
		case "telnet closed":
			tel = sv
		}
	}
	if ssh == nil || ssh.Port != 22 || !ssh.Greeting || ssh.Interval != 300 || ssh.Retries != 3 || ssh.Timeout != 5 {
		t.Fatalf("default service: %+v", ssh)
	}

	// SSH up with its greeting; port 8080 closed but reversed: up; telnet closed and reversed: up.
	for _, sv := range []*Service{ssh, web, tel} {
		s.checkService(id, sv.ID)
	}
	if !ssh.Up || ssh.Banner != "SSH-2.0-OpenSSH_9.2" || !web.Up || !tel.Up {
		t.Errorf("first checks: ssh %+v web %+v tel %+v", ssh, web, tel)
	}

	// Down only after the retries; the retries come 30 s apart.
	open[22] = false
	for i := 1; i <= 4; i++ {
		s.checkService(id, ssh.ID)
		if wantUp := i <= 3; ssh.Up != wantUp {
			t.Errorf("after %d failed check(s): up=%v", i, ssh.Up)
		}
	}
	if next := s.loc.next[ssh.ID]; time.Until(next) > 31*time.Second {
		t.Errorf("a down service is checked again within 30 s, next in %s", time.Until(next))
	}
	open[22], open[8080] = true, true
	s.checkService(id, ssh.ID)
	s.checkService(id, web.ID) // reachable while reversed: counts as a failure
	if !ssh.Up || ssh.Fails != 0 || web.Fails != 1 || !strings.Contains(web.LastError, "reverse") {
		t.Errorf("recovery and reverse: ssh %+v web %+v", ssh, web)
	}
	if next := s.loc.next[ssh.ID]; time.Until(next) < 290*time.Second {
		t.Error("an up service waits for its heartbeat interval")
	}

	// The device view counts services that are down.
	web.Checked, web.Up = true, false
	_, body = c.do("GET", "/api/ui/agents/"+id, "")
	var full struct {
		Agent   agentView   `json:"agent"`
		Presets []svcPreset `json:"presets"`
	}
	json.Unmarshal([]byte(body), &full)
	if L := full.Agent.Local; L.SvcTotal != 3 || L.SvcDown != 1 || len(L.Services) != 3 || len(full.Presets) < 20 {
		t.Errorf("view: %+v", L)
	}

	// Ping paused: the status comes from the services (reversed ones only count when there are no others).
	if code, b := c.do("POST", base+"/pingstate", `{"state":"paused"}`); code != 200 || !strings.Contains(b, `"ping_mode":"paused"`) {
		t.Fatalf("ping paused: %d %s", code, b)
	}
	if code, _ := c.do("POST", base+"/ping", ""); code != 409 {
		t.Error("ping now while the ping is paused")
	}
	_, body = c.do("GET", "/api/ui/overview", "")
	if !strings.Contains(body, `"ping":false`) || !strings.Contains(body, `"online":true`) {
		t.Errorf("ping off, ssh up: %s", body)
	}
	ssh.Up = false
	_, body = c.do("GET", "/api/ui/overview", "")
	if strings.Contains(body, `"online":true`) {
		t.Errorf("ping off, ssh down: still online: %s", body)
	}

	// Editing the port starts the check over; deleting the last service with ping off is refused.
	if code, _ := c.do("POST", base+"/services/"+web.ID, `{"port":8081}`); code != 200 || web.Port != 8081 || web.Checked {
		t.Error("edit did not start over")
	}
	c.do("POST", base+"/services/"+web.ID+"/delete", "")
	c.do("POST", base+"/services/"+tel.ID+"/delete", "")
	if code, _ := c.do("POST", base+"/services/"+ssh.ID+"/delete", ""); code != 409 || len(d.Services) != 1 {
		t.Error("last service deleted while the ping is paused")
	}
	if code, _ := c.do("POST", base+"/services/"+ssh.ID+"/pause", `{"paused":true}`); code != 409 {
		t.Error("last running check paused")
	}

	// Removed and added again: the ping starts over. A paused service is not checked and does not count.
	c.do("POST", base+"/pingstate", `{"state":"on"}`)
	if code, _ := c.do("POST", base+"/pingstate", `{"state":"removed"}`); code != 200 {
		t.Errorf("removing the ping while SSH runs: %d", code)
	}
	if d.PingMode != pingRemoved || d.Hist != nil {
		t.Errorf("ping removed: mode %q hist %v", d.PingMode, d.Hist)
	}
	c.do("POST", base+"/pingstate", `{"state":"on"}`)
	if code, _ := c.do("POST", base+"/services/"+ssh.ID+"/pause", `{"paused":true}`); code != 200 || !ssh.Paused {
		t.Error("pause with the ping running")
	}
	s.st.mu.Lock()
	s.loc.mu.Lock()
	due := s.dueServices(s.checkDevs(), time.Now().Add(time.Hour))
	s.loc.mu.Unlock()
	s.st.mu.Unlock()
	if len(due) != 0 {
		t.Errorf("a paused service is due: %v", due)
	}
	_, body = c.do("GET", "/api/ui/agents/"+id, "")
	if !strings.Contains(body, `"svc_total":0`) || !strings.Contains(body, `"svc_paused":1`) || !strings.Contains(body, `"paused":true`) {
		t.Errorf("paused service in the view: %s", body)
	}
	_, body = c.do("GET", "/api/ui/audit", "")
	for _, a := range []string{"device.service_added", "device.service_changed", "device.service_removed", "device.ping_paused", "device.ping_removed", "device.ping_added", "device.service_paused"} {
		if !strings.Contains(body, a) {
			t.Errorf("audit log lacks %s", a)
		}
	}
}

func TestHTTPSService(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.svcCheck = func(ctx context.Context, addr string, spec probe.Service, timeout time.Duration) probe.Outcome {
		if spec.CertDays > 0 {
			return probe.Outcome{RTT: time.Millisecond, Detail: "certificate valid until 2026-01-01", Failed: probe.StepCertDays,
				Err: errors.New("certificate expires in 9 days (needs 14)")}
		}
		return probe.Outcome{RTT: time.Millisecond, Detail: "HTTP 200 · certificate valid until 2027-01-01"}
	}
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	_, body := c.do("POST", "/api/ui/locals", `{"name":"web","address":"10.0.0.2"}`)
	var dv agentView
	json.Unmarshal([]byte(body), &dv)
	base := "/api/ui/locals/" + dv.ID

	for body, want := range map[string]int{
		`{"preset":"https"}`: 400, // no hostname
		`{"preset":"https","host":"www.example.com","codes":"abc"}`:                          400,
		`{"preset":"https","host":"www.example.com","path":"no-slash"}`:                      400,
		`{"preset":"https","host":"bad host"}`:                                               400,
		`{"preset":"https","host":"www.example.com","cert_days":400}`:                        400,
		`{"preset":"https","host":"WWW.Example.com","codes":" 200-299, 301","reverse":true}`: 200,
		`{"preset":"proxmox","host":"pve.example.com"}`:                                      200,
		`{"preset":"","name":"alt","port":8443,"method":"https","host":"10.0.0.2"}`:          200,
		`{"preset":"","name":"x","port":8444,"method":"ftp"}`:                                400,
	} {
		if code, b := c.do("POST", base+"/services", body); code != want {
			t.Errorf("%s: %d %s, want %d", body, code, b, want)
		}
	}
	d := s.st.st.Locals[dv.ID]
	byName := map[string]*Service{}
	for _, sv := range d.Services {
		byName[sv.Name] = sv
	}
	h, p, alt := byName["HTTPS"], byName["Proxmox"], byName["alt"]
	if h == nil || h.Method != "https" || h.Host != "www.example.com" || h.Path != "/" || h.Codes != "200-299,301" || !h.CertValid || h.CertDays != 14 || h.Reverse {
		t.Fatalf("https preset: %+v", h)
	}
	if p == nil || p.Method != "https" || p.CertValid || p.CertDays != 14 || p.Port != 8006 {
		t.Errorf("proxmox preset: %+v", p)
	}
	if alt == nil || alt.Method != "https" || !alt.CertValid {
		t.Errorf("custom https: %+v", alt)
	}

	// A failed sub-probe is stored, kept in the hour slot and shown.
	s.checkService(dv.ID, h.ID)
	if h.Failed != probe.StepCertDays || h.LastError != "certificate expires in 9 days (needs 14)" || len(h.Hist) != 1 || h.Hist[0].F != probe.StepCertDays {
		t.Errorf("failed sub-probe: %+v", h)
	}
	_, body = c.do("GET", "/api/ui/agents/"+dv.ID, "")
	if !strings.Contains(body, `"steps":["connect","tls","cert","cert_days","http"]`) || !strings.Contains(body, `"failed":"cert_days"`) || !strings.Contains(body, `"svc_methods"`) {
		t.Errorf("view: %s", body)
	}

	// Without the days check it passes; back to TCP clears the HTTPS settings.
	if code, b := c.do("POST", base+"/services/"+h.ID, `{"cert_days":0}`); code != 200 || h.Checked {
		t.Fatalf("days off: %d %s", code, b)
	}
	s.checkService(dv.ID, h.ID)
	if h.Failed != "" || !h.Up || !strings.HasPrefix(h.Banner, "HTTP 200") {
		t.Errorf("passed: %+v", h)
	}
	if code, b := c.do("POST", base+"/services/"+h.ID, `{"method":"tcp","reverse":true}`); code != 200 || h.Method != "" || h.Host != "" || h.Codes != "" || !h.Reverse {
		t.Errorf("back to tcp: %d %s %+v", code, b, h)
	}

	// The remote probe list carries the HTTPS settings.
	s.st.mu.Lock()
	d.Probe = "a_x"
	l := s.probeList("a_x")
	s.st.mu.Unlock()
	found := false
	for _, ck := range l.Checks {
		if ck.ID == p.ID {
			found = ck.Kind == "https" && ck.Host == "pve.example.com" && ck.Path == "/" && ck.Codes == "200-399" && !ck.CertValid && ck.CertDays == 14
		}
	}
	if !found {
		t.Errorf("probe list: %+v", l.Checks)
	}
}

// The real check, against listeners on this machine.
func TestTCPCheck(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	greet := make(chan bool, 4)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			if <-greet {
				c.Write([]byte("220 mail.example ESMTP ready\r\n"))
			}
			time.Sleep(300 * time.Millisecond)
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	ctx := context.Background()
	greet <- true
	if o := serviceCheck(ctx, "127.0.0.1", probe.Service{Port: port, Greeting: true}, 2*time.Second); o.Err != nil || o.Detail != "220 mail.example ESMTP ready" {
		t.Errorf("greeting: %+v", o)
	}
	greet <- false
	if o := serviceCheck(ctx, "127.0.0.1", probe.Service{Port: port}, 2*time.Second); o.Err != nil {
		t.Errorf("connect only: %+v", o)
	}
	greet <- false
	if o := serviceCheck(ctx, "127.0.0.1", probe.Service{Port: port, Greeting: true}, time.Second); o.Err == nil || !strings.Contains(o.Err.Error(), "no greeting") || o.Failed != probe.StepGreeting {
		t.Errorf("silent service with greeting required: %+v", o)
	}
	ln2, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := ln2.Addr().(*net.TCPAddr).Port
	ln2.Close()
	if o := serviceCheck(ctx, "127.0.0.1", probe.Service{Port: closed}, time.Second); o.Err == nil || !strings.Contains(o.Err.Error(), "refused") || o.Failed != probe.StepConnect {
		t.Errorf("closed port: %+v", o)
	}
}
