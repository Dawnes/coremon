package master

import (
	"testing"
	"time"

	"coremon/internal/proto"
)

func TestProbeRequests(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	now := time.Now().Unix()
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	hb := int64(s.heartbeat())
	s.st.st.Agents["a_on"] = &Agent{ID: "a_on", Name: "web", Status: StatusActive, LastSeen: now, Facts: proto.Facts{IPs: []string{"10.0.0.5"}}}
	s.st.st.Agents["a_off"] = &Agent{ID: "a_off", Name: "db", Status: StatusActive, LastSeen: now - 3600}
	s.st.st.Agents["a_master"] = &Agent{ID: "a_master", Name: "master", Status: StatusActive, LastSeen: now, Facts: proto.Facts{IPs: []string{"10.0.0.1"}}}
	add := func(id, agent, status string, created, started, finished int64, timeout int, err string) {
		s.st.st.Jobs = append(s.st.st.Jobs, &Job{ID: id, AgentID: agent, Type: proto.CmdAction, Feature: "updates", Action: "check",
			Status: status, Created: created, Started: started, Finished: finished, Timeout: timeout, Error: err})
	}
	add("waiting", "a_on", JobQueued, now-5, 0, 0, 60, "")
	add("late", "a_on", JobQueued, now-3*hb, 0, 0, 60, "")
	add("offline", "a_off", JobQueued, now-3*hb, 0, 0, 60, "")
	add("running", "a_on", JobRunning, now-20, now-10, 0, 60, "")
	add("slow", "a_on", JobRunning, now-200, now-120, 0, 60, "")
	add("answered", "a_on", JobSuccess, now-30, now-29, now-20, 60, "")
	add("failed", "a_on", JobFailed, now-30, now-29, now-20, 60, "apt-get exited 100")
	add("gaveup", "a_on", JobTimeout, now-400, now-399, now-60, 60, "no result from agent within timeout")
	add("oldgaveup", "a_on", JobTimeout, now-4000, now-3999, now-1000, 60, "no result from agent within timeout")
	add("mine", "a_master", JobQueued, now-5, 0, 0, 60, "")

	got := map[string]string{}
	for _, p := range s.probeRequests(now, map[string]bool{"10.0.0.1": true}) {
		got[p.ID] = p.State
	}
	want := map[string]string{"waiting": "waiting", "late": "overdue", "offline": "offline", "running": "running", "slow": "overdue", "gaveup": "gave_up"}
	for id, st := range want {
		if got[id] != st {
			t.Errorf("%s: %q, want %q", id, got[id], st)
		}
	}
	for _, id := range []string{"answered", "failed", "oldgaveup", "mine"} {
		if _, ok := got[id]; ok {
			t.Errorf("%s must not be listed", id)
		}
	}
}
