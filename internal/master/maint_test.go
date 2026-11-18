package master

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coremon/internal/proto"
)

func TestMaintenance(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	now := time.Now().Unix()
	s.st.mu.Lock()
	s.st.st.Agents["a_p"] = &Agent{ID: "a_p", Name: "probe-1", Status: StatusActive, LastSeen: now, ProbeCap: true, ProbeOn: true, CoreVersion: "0.3.9",
		Facts: proto.Facts{IPs: []string{"10.9.0.2"}}}
	s.st.st.Agents["a_w"] = &Agent{ID: "a_w", Name: "web", Status: StatusActive, LastSeen: now, CoreVersion: "0.3.9",
		Facts: proto.Facts{IPs: []string{"10.9.0.3"}}}
	s.st.st.Jobs = append(s.st.st.Jobs,
		&Job{ID: "auto", AgentID: "a_w", Type: proto.CmdAction, Feature: "updates", Action: "check", Actor: "scheduler", Status: JobQueued, Created: now},
		&Job{ID: "mine", AgentID: "a_w", Type: proto.CmdAction, Feature: "updates", Action: "check", Actor: "admin", Status: JobQueued, Created: now})
	s.st.mu.Unlock()
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	_, body := c.do("POST", "/api/ui/locals", `{"name":"cam","address":"10.9.0.50","probe":"a_p"}`)
	var dv agentView
	json.Unmarshal([]byte(body), &dv)
	cam := s.st.st.Locals[dv.ID]
	if cam == nil || cam.Probe != "a_p" {
		t.Fatalf("cam: %s", body)
	}

	if code, _ := c.do("POST", "/api/ui/devices/a_w/maintenance", `{"minutes":7}`); code != 400 {
		t.Error("odd duration accepted")
	}

	// Agent: automatic jobs cancelled, the user's own stay.
	if code, b := c.do("POST", "/api/ui/devices/a_w/maintenance", `{"minutes":0}`); code != 200 {
		t.Fatalf("start: %d %s", code, b)
	}
	s.st.mu.Lock()
	w := s.st.st.Agents["a_w"]
	if w.Maint == nil || w.Maint.Until != 0 || w.Maint.By != "admin" {
		t.Errorf("maint: %+v", w.Maint)
	}
	for _, j := range s.st.st.Jobs {
		if j.ID == "auto" && j.Status != JobCancelled || j.ID == "mine" && j.Status != JobQueued {
			t.Errorf("job %s: %s", j.ID, j.Status)
		}
	}
	w.LastSeen = now - 3600 // agent not connected
	s.st.mu.Unlock()
	_, b := c.do("POST", "/api/ui/actions", `{"agent_ids":["a_w"],"feature":"updates","action":"check","params":{}}`)
	if !strings.Contains(b, "not connected") {
		t.Errorf("action while offline in maintenance: %s", b)
	}
	if code, _ := c.do("POST", "/api/ui/agents/a_w/core", `{"version":"0.3.9"}`); code != 409 {
		t.Errorf("version change while offline in maintenance: %d", code)
	}
	if _, b := c.do("GET", "/api/ui/agents/a_w", ""); !strings.Contains(b, `"maint":{`) {
		t.Errorf("view without maint: %s", b)
	}
	c.do("POST", "/api/ui/devices/a_w/maintenance/end", "")
	if w.Maint != nil {
		t.Error("not ended")
	}

	// Agentless with a remote probe: its checks leave the probe's list and come back.
	s.st.mu.Lock()
	before := len(s.probeList("a_p").Checks)
	s.st.mu.Unlock()
	c.do("POST", "/api/ui/devices/"+cam.ID+"/maintenance", `{"minutes":5}`)
	s.st.mu.Lock()
	during := len(s.probeList("a_p").Checks)
	until := cam.Maint.Until
	s.st.mu.Unlock()
	if before != 1 || during != 0 || until-time.Now().Unix() < 290 {
		t.Errorf("probe list %d -> %d, until in %d s", before, during, until-time.Now().Unix())
	}

	// Ends by itself when the time is up.
	s.st.mu.Lock()
	s.maintExpire(time.Unix(until, 0))
	after := len(s.probeList("a_p").Checks)
	s.st.mu.Unlock()
	if cam.Maint != nil || after != 1 {
		t.Errorf("expire: maint %+v, list %d", cam.Maint, after)
	}
	var started, ended int
	for _, e := range s.audit.list("", "", 100) {
		switch {
		case e.Action == "device.maintenance_started":
			started++
		case e.Action == "device.maintenance_ended" && (e.Actor == "admin" || e.Detail == "time is up"):
			ended++
		}
	}
	if started != 2 || ended != 2 {
		t.Errorf("audit: %d started, %d ended", started, ended)
	}
}
