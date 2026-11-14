package master

import (
	"testing"
	"time"

	"coremon/internal/proto"
)

func TestSchedulerRounds(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	now := time.Now().Unix()
	s.st.mu.Lock()
	for _, id := range []string{"a_1", "a_2", "a_3"} {
		s.st.st.Agents[id] = &Agent{ID: id, Name: id, Status: StatusActive, Protocol: 5, LastSeen: now,
			StateAt:  map[string]int64{"updates": now - 60}, // checked a minute ago
			Features: []proto.FeatureInfo{{Name: "updates", Actions: []proto.ActionSpec{{Name: "check"}}}}}
	}
	s.st.st.Agents["a_off"] = &Agent{ID: "a_off", Name: "off", Status: StatusActive, Protocol: 5, LastSeen: now - 3600,
		Features: []proto.FeatureInfo{{Name: "updates", Actions: []proto.ActionSpec{{Name: "check"}}}}}
	s.st.st.Jobs = nil
	s.st.mu.Unlock()
	checks := func() int {
		s.st.mu.Lock()
		defer s.st.mu.Unlock()
		n := 0
		for _, j := range s.st.st.Jobs {
			if j.Actor == "scheduler" && j.Action == "check" {
				n++
			}
		}
		return n
	}
	// The first round starts at once and covers every online system,
	// whenever each was checked last.
	s.schedulerTick()
	if n := checks(); n != 3 {
		t.Fatalf("first round: %d checks, want 3", n)
	}
	s.schedulerTick()
	if n := checks(); n != 3 {
		t.Fatalf("round repeated: %d checks", n)
	}
	// The round is one row on the Jobs page.
	s.st.mu.Lock()
	entries, _ := s.jobsGrouped("", false, "", 0, 10)
	for _, j := range s.st.st.Jobs {
		j.Status, j.Finished = JobSuccess, now
	}
	// The next round comes one interval later.
	s.st.st.CheckRoundAt -= int64(s.st.st.Settings.CheckIntervalHours) * 3600
	s.st.mu.Unlock()
	round := 0
	for _, e := range entries {
		if e.Action == "check" {
			round = len(e.Batch)
		}
	}
	if round != 3 {
		t.Fatalf("round on the Jobs page: %d checks in one row, want 3", round)
	}
	s.schedulerTick()
	if n := checks(); n != 6 {
		t.Fatalf("second round: %d checks, want 6", n)
	}
}
