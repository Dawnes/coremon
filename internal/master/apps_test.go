package master

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coremon/internal/feature/apps"
	"coremon/internal/proto"
)

func TestApplications(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	a := &Agent{ID: "a_1", Name: "web-01", Status: StatusReportOnly, CoreVersion: "0.3.4", Protocol: 5, LastSeen: time.Now().Unix(),
		Features: []proto.FeatureInfo{
			{Name: "updates", Actions: []proto.ActionSpec{{Name: "check"}}},
			{Name: "apps", Actions: []proto.ActionSpec{{Name: "scan", Timeout: 120}}}},
		State: map[string]json.RawMessage{}, StateAt: map[string]int64{}}
	s.st.st.Agents["a_1"] = a
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)

	// Report only: the scan is allowed (and handed out), a check is not.
	if _, body := c.do("POST", "/api/ui/actions", `{"agent_ids":["a_1"],"feature":"updates","action":"check","params":{}}`); !strings.Contains(body, "report only") {
		t.Fatalf("check on a report-only system: %s", body)
	}
	_, body := c.do("POST", "/api/ui/actions", `{"agent_ids":["a_1"],"feature":"apps","action":"scan","params":{}}`)
	var res struct{ Jobs []string }
	json.Unmarshal([]byte(body), &res)
	if len(res.Jobs) != 1 {
		t.Fatalf("scan on a report-only system: %s", body)
	}
	s.st.mu.Lock()
	cmds := s.takeCommands("a_1")
	s.st.mu.Unlock()
	if len(cmds) != 1 || cmds[0].Feature != "apps" {
		t.Fatalf("scan not handed out: %+v", cmds)
	}

	// The scan arrives with an update check result too; it is checked.
	s.st.mu.Lock()
	a.Status = StatusActive
	j := s.enqueue(a, &Job{Type: proto.CmdAction, Feature: "updates", Action: "check"})
	j.Status = JobRunning
	s.st.mu.Unlock()
	st := apps.State{ScannedAt: time.Now().Add(time.Hour).Unix(), Apps: []apps.App{
		{Name: "PHP 8.2", Installed: true, Version: "8.2.26\x1b[2J", Full: "8.2.26-1~deb12u1", Package: "php8.2-cli"},
		{Name: "Nginx", Installed: false, Version: "should go"},
	}}
	data, _ := json.Marshal(map[string]any{"state": map[string]any{"total": 0}, "states": map[string]any{"apps": st, "evil": "x"}})
	s.resultCore("a_1", proto.Result{CommandID: j.ID, OK: true, Data: data}, "")
	var got apps.State
	if json.Unmarshal(a.State["apps"], &got) != nil || len(got.Apps) != 2 {
		t.Fatalf("apps state not stored: %s", a.State["apps"])
	}
	if got.Apps[0].Version != "8.2.26[2J" || got.Apps[1].Version != "" || got.ScannedAt > time.Now().Unix() {
		t.Fatalf("apps state not cleaned: %+v", got)
	}
	if _, ok := a.State["evil"]; ok {
		t.Fatal("an unknown extra state was stored")
	}
	// Too many rows are refused.
	many := apps.State{Apps: make([]apps.App, maxApps+1)}
	raw, _ := json.Marshal(many)
	if _, err := checkState("apps", raw); err == nil {
		t.Fatal("oversized application list accepted")
	}
}
