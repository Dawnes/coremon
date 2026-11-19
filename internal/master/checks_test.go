package master

import (
	"context"
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

// A default service only takes the check methods that suit it; a custom port takes any.
func TestPresetMethods(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.svcCheck = func(ctx context.Context, addr string, spec probe.Service, timeout time.Duration) probe.Outcome {
		return probe.Outcome{RTT: time.Millisecond}
	}
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	_, body := c.do("POST", "/api/ui/locals", `{"name":"fw","address":"10.0.0.1"}`)
	var dv agentView
	json.Unmarshal([]byte(body), &dv)
	base := "/api/ui/locals/" + dv.ID + "/services"
	for body, want := range map[string]int{
		`{"preset":"ssh","method":"https","host":"fw.example.com"}`:          400,
		`{"preset":"smtp","method":"https","host":"fw.example.com"}`:         400,
		`{"preset":"proxmox","method":"tcp"}`:                                200,
		`{"preset":"https","host":"fw.example.com"}`:                         200,
		`{"preset":"","name":"alt","port":8443,"method":"https","host":"x"}`: 200,
		`{"preset":"dns"}`:                                 400, // no name to resolve
		`{"preset":"dns","host":"bad name"}`:               400,
		`{"preset":"dns","host":"x.com","rtype":"SOA"}`:    400,
		`{"preset":"dns","method":"tcp","name":"DNS tcp"}`: 200,
		`{"preset":"ssh","method":"dns","host":"x.com"}`:   400,
	} {
		if code, b := c.do("POST", base, body); code != want {
			t.Errorf("%s: %d %s, want %d", body, code, b, want)
		}
	}
	if p := presetByKey("https"); len(p.Methods) != 2 || p.Methods[0] != probe.MethodHTTPS {
		t.Errorf("https methods: %v", p.Methods)
	}
	if p := presetByKey("ssh"); len(p.Methods) != 1 || p.Methods[0] != probe.MethodTCP {
		t.Errorf("ssh methods: %v", p.Methods)
	}
	// DNS: the DNS query by default (type A, answer within 3 s); the fields reach the check and a probe's list.
	if code, b := c.do("POST", base, `{"preset":"dns","name":"Resolver","host":"Example.COM","expect":" 192.0.2.1 "}`); code != 200 {
		t.Fatalf("dns: %d %s", code, b)
	}
	var sv *Service
	for _, x := range s.st.st.Locals[dv.ID].Services {
		if x.Name == "Resolver" {
			sv = x
		}
	}
	if sv == nil || sv.kind() != probe.MethodDNS || sv.Host != "example.com" || sv.RType != "A" || sv.Expect != "192.0.2.1" || sv.MaxMs != 3000 || sv.Reverse {
		t.Fatalf("dns service: %+v", sv)
	}
	if sp := sv.spec(); sp.RType != "A" || sp.MaxMs != 3000 || strings.Join(probe.Steps(sp), ",") != "query,rcode,records,expect,time" {
		t.Errorf("dns spec: %+v", sp)
	}
	sid := sv.ID
	if code, b := c.do("POST", base+"/"+sid, `{"rtype":"mx","max_ms":0,"expect":""}`); code != 200 || sv.RType != "MX" || sv.MaxMs != 0 || sv.Expect != "" {
		t.Errorf("dns edit: %d %s %+v", code, b, sv)
	}
	if code, _ := c.do("POST", base+"/"+sid, `{"method":"tcp"}`); code != 200 || sv.RType != "" || sv.Host != "" || sv.kind() != probe.MethodTCP {
		t.Errorf("dns to tcp: %+v", sv)
	}
}

// An agent device can have services too: checked from outside by the
// master or by another agent, on one of its own addresses or another one.
func TestAgentDeviceServices(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.svcCheck = func(ctx context.Context, addr string, spec probe.Service, timeout time.Duration) probe.Outcome {
		if addr == "10.1.0.5" && spec.Port == 22 {
			return probe.Outcome{RTT: time.Millisecond}
		}
		return probe.Outcome{Failed: probe.StepConnect, Err: errors.New("connection refused")}
	}
	now := time.Now().Unix()
	s.st.mu.Lock()
	s.st.st.Agents["a_web"] = &Agent{ID: "a_web", Name: "web", Status: StatusActive, LastSeen: now, CoreVersion: "0.3.9",
		Facts: proto.Facts{IPs: []string{"127.0.0.1", "fe80::1", "fd00::5", "10.1.0.5"}}}
	s.st.st.Agents["a_p"] = &Agent{ID: "a_p", Name: "probe", Status: StatusActive, LastSeen: now, ProbeCap: true, ProbeOn: true, CoreVersion: "0.3.10",
		Facts: proto.Facts{IPs: []string{"10.9.0.2"}}}
	s.st.st.Agents["a_new"] = &Agent{ID: "a_new", Name: "new", Status: StatusPending, Facts: proto.Facts{IPs: []string{"10.1.0.9"}}}
	s.st.mu.Unlock()
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	a := s.st.st.Agents["a_web"]

	if code, _ := c.do("POST", "/api/ui/locals/a_new/services", `{"preset":"ssh"}`); code != 409 {
		t.Error("service on a pending agent")
	}
	// Address and checker can be set before any service: the first usable
	// address (IPv4 first) by default, checked by the master, no ping.
	if code, b := c.do("POST", "/api/ui/agents/a_web/checks", `{}`); code != 200 || a.Checks == nil || a.Checks.Address != "10.1.0.5" || len(a.Checks.Services) != 0 {
		t.Fatalf("checks settings before any service: %d %s", code, b)
	}
	code, body := c.do("POST", "/api/ui/locals/a_web/services", `{"preset":"ssh"}`)
	var v agentView
	json.Unmarshal([]byte(body), &v)
	if code != 200 || a.Checks == nil || a.Checks.Address != "10.1.0.5" || a.Checks.Probe != "" || a.Checks.pinging() ||
		v.Monitor != "agent" || v.Checks == nil || len(v.Checks.Services) != 1 || v.Checks.Ping {
		t.Fatalf("first service: %d %s", code, body)
	}
	c.do("POST", "/api/ui/locals/a_web/services", `{"preset":"http"}`)
	sid := a.Checks.Services[0].ID
	if code, b := c.do("POST", "/api/ui/locals/a_web/services/"+sid+"/check", ""); code != 200 || !a.Checks.Services[0].Up {
		t.Errorf("check now: %d %s", code, b)
	}
	// An agentless device may not take the address the master checks there.
	if code, _ := c.do("POST", "/api/ui/locals", `{"name":"dup","address":"10.1.0.5"}`); code != 409 {
		t.Error("agentless device on the agent's address")
	}
	// Not checked by itself; checked by another agent: in that agent's list.
	if code, _ := c.do("POST", "/api/ui/agents/a_web/checks", `{"probe":"a_web"}`); code != 409 {
		t.Error("checked by its own agent")
	}
	if code, b := c.do("POST", "/api/ui/agents/a_web/checks", `{"probe":"a_p","address":"10.1.0.7"}`); code != 200 || a.Checks.Probe != "a_p" || a.Checks.Address != "10.1.0.7" {
		t.Fatalf("checked by a_p: %d %s", code, b)
	}
	s.st.mu.Lock()
	l := s.probeList("a_p")
	s.st.mu.Unlock()
	if len(l.Checks) != 2 || l.Checks[0].Address != "10.1.0.7" {
		t.Errorf("probe list: %+v", l.Checks)
	}
	// Results from the probe are stored on the agent device.
	s.st.mu.Lock()
	s.probeIntake(s.st.st.Agents["a_p"], &proto.ProbeReport{Results: []proto.ProbeResult{{ID: sid, OK: false, Error: "refused", Failed: probe.StepConnect}}}, time.Now())
	v = s.view(a, false)
	s.st.mu.Unlock()
	if a.Checks.Services[0].LastError != "refused" || v.Checks == nil || v.Checks.Probe == nil {
		t.Errorf("probe result: %+v", a.Checks.Services[0])
	}
	// Maintenance: out of the probe's list.
	c.do("POST", "/api/ui/devices/a_web/maintenance", `{"minutes":5}`)
	s.st.mu.Lock()
	n := len(s.probeList("a_p").Checks)
	s.st.mu.Unlock()
	if n != 0 {
		t.Errorf("checks in maintenance: %d", n)
	}
	c.do("POST", "/api/ui/devices/a_web/maintenance/end", "")

	// Saved and restored with the results.
	s.st.mu.Lock()
	js, _ := s.checksJSON()
	s.st.mu.Unlock()
	if !strings.Contains(string(js), `"a_web"`) {
		t.Errorf("checks.dat: %s", js)
	}

	// All services gone: the address and checker stay; the probe keeps the
	// device until another checker is chosen.
	for len(a.Checks.Services) > 0 {
		if code, b := c.do("POST", "/api/ui/locals/a_web/services/"+a.Checks.Services[0].ID+"/delete", ""); code != 200 {
			t.Fatalf("delete: %d %s", code, b)
		}
	}
	if a.Checks == nil || a.Checks.Probe != "a_p" {
		t.Fatal("settings lost with the last service")
	}
	if code, _ := c.do("POST", "/api/ui/agents/a_p/probe/enabled", `{"on":false}`); code != 409 {
		t.Error("probe switched off while still chosen")
	}
	c.do("POST", "/api/ui/agents/a_web/checks", `{"probe":""}`)
	if code, _ := c.do("POST", "/api/ui/agents/a_p/probe/enabled", `{"on":false}`); code != 200 {
		t.Error("probe switch-off blocked")
	}
}
