package master

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coremon/internal/proto"
)

// storeVersions makes the master believe it stores these agent versions.
func storeVersions(s *Server, versions ...string) {
	s.repo.mu.Lock()
	defer s.repo.mu.Unlock()
	s.repo.core = map[string]map[string]*artifactEntry{}
	for _, v := range versions {
		s.repo.core[v] = map[string]*artifactEntry{"linux-amd64": {Artifact: proto.Artifact{Kind: "core", Version: v, OS: "linux", Arch: "amd64"}}}
	}
}

func TestMinimumAgentVersion(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	storeVersions(s, "0.3.1", "0.3.2", "0.3.3")
	now := time.Now().Unix()
	mk := func(id, ver string) *Agent {
		a := &Agent{ID: id, Name: id, Status: StatusActive, CoreVersion: ver, Protocol: 5, LastSeen: now,
			Facts: proto.Facts{OS: "linux", Arch: "amd64", Hostname: "h-" + id}, Features: []proto.FeatureInfo{{Name: "updates", Actions: []proto.ActionSpec{{Name: "check"}}}},
			State: map[string]json.RawMessage{"updates": json.RawMessage(`{"total":4}`)}, StateAt: map[string]int64{}}
		s.st.st.Agents[id] = a
		return a
	}
	cur, old, ancient := mk("a_cur", "0.3.3"), mk("a_old", "0.3.0"), mk("a_ancient", "0.2.5")

	// Default: the oldest stored version. 0.3.0 is not stored, so it is
	// upgrade only already.
	s.st.mu.Lock()
	if m := s.minAgent(); m != "0.3.1" {
		t.Fatalf("default minimum %q", m)
	}
	if s.belowMin(cur) || !s.belowMin(old) || !s.belowMin(ancient) {
		t.Fatal("classification")
	}
	// The view of an upgrade-only system holds no data.
	v := s.view(old, true)
	if !v.BelowMin || v.MinAgent != "0.3.1" || v.Updates != nil || v.Facts.Hostname != "" || len(v.Features) != 0 || v.State != nil {
		t.Fatalf("view of an upgrade-only system: %+v", v)
	}
	s.st.mu.Unlock()

	// A heartbeat below the minimum stores nothing but the platform.
	s.st.st.Settings.HeartbeatSec = 10
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	s.pollCore(ctx, "a_old", proto.PollRequest{Protocol: 5, CoreVersion: "0.3.0",
		Facts: proto.Facts{Hostname: "changed", OS: "linux", Arch: "arm64"}, Features: []proto.FeatureInfo{{Name: "evil"}}})
	cancel()
	s.st.mu.Lock()
	if old.Facts.Hostname != "h-a_old" || old.Facts.Arch != "arm64" || old.Features[0].Name != "updates" {
		t.Fatalf("heartbeat below the minimum stored data: %+v %+v", old.Facts, old.Features)
	}
	// Only agent updates, uninstall and key slots are handed out.
	check := s.enqueue(old, &Job{Type: proto.CmdAction, Feature: "updates", Action: "check"})
	upd := s.enqueue(old, &Job{Type: proto.CmdCoreUpdate, Artifact: &proto.Artifact{Version: "0.3.3"}})
	cmds := s.takeCommands("a_old")
	s.st.mu.Unlock()
	if len(cmds) != 1 || cmds[0].ID != upd.ID || check.Status != JobQueued {
		t.Fatalf("commands below the minimum: %+v", cmds)
	}
	// A result of something else is discarded.
	s.st.mu.Lock()
	check.Status = JobRunning
	s.st.mu.Unlock()
	res, _ := json.Marshal(map[string]any{"state": map[string]any{"total": 99}})
	s.resultCore("a_old", proto.Result{CommandID: check.ID, OK: true, Data: res}, "")
	if check.Status != JobFailed || !strings.Contains(check.Error, "discarded") || string(old.State["updates"]) != `{"total":4}` {
		t.Fatalf("result below the minimum: %s %q %s", check.Status, check.Error, old.State["updates"])
	}

	// Through the website.
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	set := func(min string) (int, string) {
		return c.do("POST", "/api/ui/settings", `{"check_interval_hours":6,"heartbeat_sec":30,"retention_days":3,"ui_port":8443,"os_info_days":14,"vuln_days":1,"min_agent_version":"`+min+`"}`)
	}
	if code, _ := set("0.3.0"); code != 400 {
		t.Error("a minimum below the stored versions was accepted")
	}
	if code, body := set("0.3.3"); code != 200 {
		t.Fatalf("set minimum: %d %s", code, body)
	}
	s.st.mu.Lock()
	below := s.belowMin(mk("a_mid", "0.3.2"))
	s.st.mu.Unlock()
	if !below {
		t.Fatal("0.3.2 must be below a minimum of 0.3.3")
	}
	if _, body := c.do("POST", "/api/ui/actions", `{"agent_ids":["a_mid"],"feature":"updates","action":"check","params":{}}`); !strings.Contains(body, "lowest allowed") {
		t.Errorf("action on an upgrade-only system: %s", body)
	}
	if code, _ := c.do("POST", "/api/ui/agents/a_mid", `{"report_only":true}`); code != 409 {
		t.Error("report-only switch on an upgrade-only system accepted")
	}
	if code, _ := c.do("POST", "/api/ui/agents/a_mid/ping", `{}`); code != 409 {
		t.Error("ping of an upgrade-only system accepted")
	}
	if code, _ := c.do("POST", "/api/ui/agents/a_cur/core", `{"version":"0.3.2"}`); code != 400 {
		t.Error("downgrade below the minimum accepted")
	}

	// A new release pushes 0.3.1 out: a minimum below the oldest is raised.
	s.st.mu.Lock()
	s.st.st.Settings.MinAgentVersion = "0.3.1"
	s.st.mu.Unlock()
	storeVersions(s, "0.3.2", "0.3.3", "0.3.4")
	s.raiseMinAgent()
	s.st.mu.Lock()
	got := s.st.st.Settings.MinAgentVersion
	s.st.mu.Unlock()
	if got != "0.3.2" {
		t.Fatalf("minimum not raised: %q", got)
	}
	if e := s.audit.list("", "", 1); len(e) == 0 || e[0].Action != "settings.min_agent_raised" {
		t.Fatalf("raise not audited: %+v", e)
	}
}
