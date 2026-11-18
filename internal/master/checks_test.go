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
	if code, _ := c.do("POST", "/api/ui/agents/a_web/checks", `{"address":"10.1.0.6"}`); code != 409 {
		t.Error("checks settings before any service")
	}
	// The first service: the first usable address (IPv4 first), checked by the master, no ping.
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

	// All services gone and no ping: the checks block goes too (nothing is checked).
	for len(a.Checks.Services) > 0 {
		if code, b := c.do("POST", "/api/ui/locals/a_web/services/"+a.Checks.Services[0].ID+"/delete", ""); code != 200 {
			t.Fatalf("delete: %d %s", code, b)
		}
		if a.Checks == nil {
			break
		}
	}
	if a.Checks != nil {
		t.Error("empty checks kept")
	}
	if code, _ := c.do("POST", "/api/ui/agents/a_p/probe/enabled", `{"on":false}`); code != 200 {
		t.Error("probe switch-off blocked by a removed device")
	}
}
