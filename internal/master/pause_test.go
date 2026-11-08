package master

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPauseCancelsQueuedJobs(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.2.5")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)

	now := time.Now().Unix()
	s.st.mu.Lock()
	a := &Agent{ID: "a_1", Name: "web-01", Status: "active", DesiredCore: "0.2.9", LastSeen: now}
	s.st.st.Agents[a.ID] = a
	s.st.st.Jobs = []*Job{
		{ID: "j_q1", AgentID: "a_1", Status: JobQueued, Created: now},
		{ID: "j_q2", AgentID: "a_1", Status: JobQueued, Created: now},
		{ID: "j_run", AgentID: "a_1", Status: JobRunning, Created: now, Started: now},
		{ID: "j_other", AgentID: "a_2", Status: JobQueued, Created: now},
	}
	s.st.mu.Unlock()

	if code, body := c.do("POST", "/api/ui/agents/a_1", `{"paused":true}`); code != 200 {
		t.Fatalf("pause: %d %s", code, body)
	}
	s.st.mu.Lock()
	got := map[string]string{}
	for _, j := range s.st.st.Jobs {
		got[j.ID] = j.Status + "/" + j.Error
	}
	desired := a.DesiredCore
	s.st.mu.Unlock()
	want := map[string]string{
		"j_q1": "cancelled/cancelled: system paused", "j_q2": "cancelled/cancelled: system paused",
		"j_run": "running/", "j_other": "queued/",
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s = %q, want %q", id, got[id], w)
		}
	}
	if desired != "" {
		t.Errorf("requested agent version should be cancelled, still %q", desired)
	}
	e := s.audit.list("a_1", "", 10)
	if len(e) == 0 || e[0].Action != "agent.paused" || !strings.Contains(e[0].Detail, "2 queued job(s) cancelled") {
		t.Errorf("audit: %+v", e)
	}
}
