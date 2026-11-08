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

func TestReportOnly(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.2.8")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)

	now := time.Now().Unix()
	s.st.mu.Lock()
	a := &Agent{ID: "a_1", Name: "web-01", Status: StatusActive, Protocol: 3, LastSeen: now,
		Features: []proto.FeatureInfo{{Name: "updates", Actions: []proto.ActionSpec{{Name: "check"}, {Name: "install", Mutating: true}}}}}
	s.st.st.Agents[a.ID] = a
	s.st.st.Jobs = []*Job{
		{ID: "j_q1", AgentID: "a_1", Type: proto.CmdAction, Status: JobQueued, Created: now},
		{ID: "j_core", AgentID: "a_1", Type: proto.CmdCoreUpdate, Status: JobQueued, Created: now},
		{ID: "j_run", AgentID: "a_1", Type: proto.CmdAction, Status: JobRunning, Created: now, Started: now},
		{ID: "j_other", AgentID: "a_2", Type: proto.CmdAction, Status: JobQueued, Created: now},
	}
	s.st.mu.Unlock()

	if code, body := c.do("POST", "/api/ui/agents/a_1", `{"report_only":true}`); code != 200 {
		t.Fatalf("report only: %d %s", code, body)
	}
	s.st.mu.Lock()
	got := map[string]string{}
	var modeJob *Job
	for _, j := range s.st.st.Jobs {
		got[j.ID] = j.Status
		if j.Type == proto.CmdSetMode {
			modeJob = j
		}
	}
	status := a.Status
	cmds := s.takeCommands("a_1")
	s.st.mu.Unlock()
	for id, want := range map[string]string{"j_q1": JobCancelled, "j_core": JobQueued, "j_run": JobRunning, "j_other": JobQueued} {
		if got[id] != want {
			t.Errorf("%s = %s, want %s", id, got[id], want)
		}
	}
	if status != StatusReportOnly {
		t.Fatalf("status %q", status)
	}
	// The agent is told to switch, and still gets the agent version change.
	if modeJob == nil || !strings.Contains(string(modeJob.Params), `"report_only":true`) {
		t.Fatalf("no mode command for the agent: %+v", modeJob)
	}
	if modeJob.Actor != "admin" {
		t.Errorf("the mode job must carry the user who switched, got %q", modeJob.Actor)
	}
	types := []string{}
	for _, c := range cmds {
		types = append(types, c.Type)
	}
	if strings.Join(types, ",") != "core.update,agent.mode" {
		t.Errorf("commands sent in report only: %v", types)
	}

	// Actions are refused by the master while in report only.
	_, body := c.do("POST", "/api/ui/actions", `{"agent_ids":["a_1"],"feature":"updates","action":"check"}`)
	var res struct{ Jobs, Errors []string }
	json.Unmarshal([]byte(body), &res)
	if len(res.Jobs) != 0 || len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "report only") {
		t.Errorf("action in report only: %s", body)
	}
	// No automatic checks either.
	s.schedulerTick()
	s.st.mu.Lock()
	for _, j := range s.st.st.Jobs {
		if j.AgentID == "a_1" && j.Type == proto.CmdAction && j.Status == JobQueued {
			t.Errorf("scheduler queued %s %s in report only", j.Feature, j.Action)
		}
	}
	// The agent confirmed report only; allow changes sends the reverse.
	a.ReportOnly = true
	for _, j := range s.st.st.Jobs {
		if j.Type == proto.CmdSetMode {
			j.Status = JobSuccess
		}
	}
	s.st.mu.Unlock()
	c.do("POST", "/api/ui/agents/a_1", `{"report_only":false}`)
	s.st.mu.Lock()
	var back *Job
	for _, j := range s.st.st.Jobs {
		if j.Type == proto.CmdSetMode && j.Status == JobQueued {
			back = j
		}
	}
	s.st.mu.Unlock()
	if back == nil || !strings.Contains(string(back.Params), `"report_only":false`) {
		t.Fatalf("allow changes must tell the agent: %+v", back)
	}
	e := s.audit.list("a_1", "", 10)
	if len(e) < 2 || e[0].Action != "agent.allow_changes" || e[1].Action != "agent.report_only" || !strings.Contains(e[1].Detail, "1 queued job(s) cancelled") {
		t.Errorf("audit: %+v", e)
	}
}

func TestPausedBecomesReportOnly(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"agents":{"a_1":{"id":"a_1","status":"paused"}}}`), 0o600)
	st, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.st.Agents["a_1"].Status != StatusReportOnly {
		t.Fatalf("paused system became %q", st.st.Agents["a_1"].Status)
	}
}
