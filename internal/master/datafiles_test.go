package master

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"coremon/internal/proto"
)

// The configuration, the agents' data and the jobs each have their own
// file; agent contacts never rewrite state.dat.
func TestDataFiles(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	now := time.Now().Unix()
	a := &Agent{ID: "a_1", Name: "web-01", Status: StatusActive, CoreVersion: "0.3.8", Protocol: 5, LastSeen: now,
		Facts:   proto.Facts{Hostname: "secret-host", UptimeSec: 3600, Health: &proto.Health{At: now}},
		State:   map[string]json.RawMessage{"updates": json.RawMessage(`{"total":4}`)},
		StateAt: map[string]int64{"updates": now}}
	a.setBoot(now)
	s.st.mu.Lock()
	s.st.st.Agents[a.ID] = a
	j := &Job{ID: "j_1", AgentID: a.ID, Type: proto.CmdAction, Feature: "updates", Action: "check", Status: JobSuccess, Created: now, Finished: now}
	s.st.addJob(j)
	os.MkdirAll(filepath.Join(s.cfg.DataDir, "jobs"), 0o700)
	os.WriteFile(s.st.jobLogPath(j.ID), []byte("12345"), 0o600)
	j.LogSize = 5
	s.st.markDirty()
	if err := s.st.writeNow(); err != nil {
		t.Fatal(err)
	}
	s.st.mu.Unlock()

	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(s.cfg.DataDir, name))
		if err != nil {
			t.Fatal(err)
		}
		js, err := decodeState(b)
		if err != nil {
			t.Fatal(err)
		}
		return js
	}
	st := read(stateFile)
	for _, bad := range []string{"secret-host", `"last_seen"`, `"jobs"`, `"facts"`, `"total"`, `"check_round_at"`} {
		if bytes.Contains(st, []byte(bad)) {
			t.Errorf("state.dat holds %s: %s", bad, st)
		}
	}
	if !bytes.Contains(st, []byte(`"web-01"`)) {
		t.Errorf("state.dat lost the registration: %s", st)
	}
	if ag := read(agentsFileName); !bytes.Contains(ag, []byte("secret-host")) || bytes.Contains(ag, []byte(`"uptime_sec":3600`)) {
		t.Errorf("agents.dat: %s", ag)
	}
	if jb := read(jobsFileName); !bytes.Contains(jb, []byte(`"j_1"`)) || bytes.Contains(jb, []byte(`"log_size":5`)) {
		t.Errorf("jobs.dat: %s", jb)
	}

	// Rhythm: last-seen alone waits 15 minutes; news waits at most a minute;
	// state.dat is not touched by either.
	s.st.mu.Lock()
	stSum, agSum := s.st.sums[stateFile], s.st.sums[agentsFileName]
	t0 := time.Now()
	s.st.agentsAt, s.st.agentsLook = t0, t0
	a.LastSeen, a.Facts.UptimeSec = now+30, 3630
	a.Facts.Health = &proto.Health{At: now + 300}
	a.setBoot(now + 30)
	s.st.markAgents()
	s.st.markJobs()
	s.st.flush(t0.Add(2*time.Minute), false)
	if s.st.sums[agentsFileName] != agSum {
		t.Error("a heartbeat alone must not write agents.dat within 15 minutes")
	}
	s.st.flush(t0.Add(16*time.Minute), false)
	if s.st.sums[agentsFileName] == agSum {
		t.Error("last seen must be written after 15 minutes")
	}
	agSum = s.st.sums[agentsFileName]
	a.CoreVersion = "0.3.9"
	s.st.markAgents()
	s.st.flush(t0.Add(16*time.Minute+30*time.Second), false)
	if s.st.sums[agentsFileName] != agSum {
		t.Error("written again within a minute")
	}
	s.st.flush(t0.Add(17*time.Minute), false)
	if s.st.sums[agentsFileName] == agSum {
		t.Error("news must be written within a minute")
	}
	if s.st.sums[stateFile] != stSum {
		t.Error("agent data rewrote state.dat")
	}
	s.st.mu.Unlock()

	// A restart finds everything again; the uptime comes from the boot time.
	keyPath := filepath.Join(filepath.Dir(s.cfg.DataDir), "signing.pub")
	s2, err := New(Config{DataDir: s.cfg.DataDir, SigningKeyPath: keyPath, Version: "0.3.0"})
	if err != nil {
		t.Fatal(err)
	}
	a2 := s2.st.st.Agents["a_1"]
	if a2 == nil || a2.CoreVersion != "0.3.9" || a2.Facts.Hostname != "secret-host" || a2.Facts.UptimeSec < 3600 || a2.Facts.UptimeSec > 3700 || len(a2.State) != 1 {
		t.Fatalf("agent after a restart: %+v", a2)
	}
	if len(s2.st.st.Jobs) != 1 || s2.st.st.Jobs[0].LogSize != 5 {
		t.Fatalf("jobs after a restart: %+v", s2.st.st.Jobs)
	}

	// The backup carries both files.
	files, _, err := s2.snapshot()
	if err != nil || files[agentsFileName] == nil || files[jobsFileName] == nil {
		t.Fatalf("backup: %v", err)
	}

	// Damaged files lose only their own data.
	os.WriteFile(filepath.Join(s.cfg.DataDir, agentsFileName), []byte("garbage"), 0o600)
	os.WriteFile(filepath.Join(s.cfg.DataDir, jobsFileName), []byte("garbage"), 0o600)
	s3, err := New(Config{DataDir: s.cfg.DataDir, SigningKeyPath: keyPath, Version: "0.3.0"})
	if err != nil {
		t.Fatal(err)
	}
	a3 := s3.st.st.Agents["a_1"]
	if a3 == nil || a3.Name != "web-01" || a3.Facts.Hostname != "" || len(s3.st.st.Jobs) != 0 {
		t.Fatalf("after damaged files: %+v, %d jobs", a3, len(s3.st.st.Jobs))
	}
}

// The boot time only moves on a reboot, not on rounding.
func TestBootTime(t *testing.T) {
	a := &Agent{}
	a.Facts.UptimeSec = 1000
	a.setBoot(10_000)
	if a.BootAt != 9000 {
		t.Fatalf("boot %d", a.BootAt)
	}
	a.Facts.UptimeSec = 1029
	a.setBoot(10_030) // a second off
	if a.BootAt != 9000 {
		t.Errorf("wobble moved the boot time: %d", a.BootAt)
	}
	a.Facts.UptimeSec = 20
	a.setBoot(20_000)
	if a.BootAt != 19_980 {
		t.Errorf("reboot not seen: %d", a.BootAt)
	}
}

// An agent without any results is checked right away, once.
func TestFirstCheck(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	now := time.Now().Unix()
	s.st.mu.Lock()
	s.st.st.CheckRoundAt = now // the round is not due
	s.st.st.Agents["a_1"] = &Agent{ID: "a_1", Name: "web-01", Status: StatusActive, CoreVersion: "0.3.8", Protocol: 5, LastSeen: now,
		Features: []proto.FeatureInfo{{Name: "updates", Actions: []proto.ActionSpec{{Name: "check", Timeout: 60}}}},
		State:    map[string]json.RawMessage{}, StateAt: map[string]int64{}}
	s.st.mu.Unlock()
	count := func() int {
		n := 0
		for _, j := range s.st.st.Jobs {
			if j.Type == proto.CmdAction && j.Action == "check" {
				n++
			}
		}
		return n
	}
	s.schedulerTick()
	if n := count(); n != 1 {
		t.Fatalf("first check: %d jobs", n)
	}
	for _, j := range s.st.st.Jobs {
		j.Status = JobSuccess
	}
	s.schedulerTick()
	if n := count(); n != 1 {
		t.Errorf("checked again without a round: %d jobs", n)
	}
}
