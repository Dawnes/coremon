package master

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"coremon/internal/probe"
)

// Round-trip times: five-minute slots with average, lowest and highest, 24 hours.
func TestRTTSlots(t *testing.T) {
	base := int64(1_800_000_000) // a multiple of 300
	var rs []rttSlot
	rs = addRTT(rs, true, 2000, base+10)
	rs = addRTT(rs, true, 6000, base+70)
	rs = addRTT(rs, false, 0, base+130)
	rs = addRTT(rs, false, 0, base+310)
	if len(rs) != 2 {
		t.Fatalf("slots: %+v", rs)
	}
	p := recentRTT(rs, base+320)
	if p[0].Avg != 4000 || p[0].Min != 2000 || p[0].Max != 6000 || p[0].N != 2 || p[0].Fail != 1 {
		t.Fatalf("first slot: %+v", p[0])
	}
	if p[1].N != 0 || p[1].Fail != 1 || p[1].Avg != 0 {
		t.Fatalf("unanswered slot: %+v", p[1])
	}
	rs = addRTT(rs, true, 1000, base+histSpan+400)
	if len(rs) != 1 || rs[0].Min != 1000 {
		t.Fatalf("older than 24 hours must go: %+v", rs)
	}
}

// A shorter ping interval applies right away; services keep their schedule.
func TestReplanPings(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	now := time.Unix(1_800_000_000, 0)
	d := &LocalDevice{ID: "l_0123456789abcdef", Name: "printer", Address: "10.0.0.7", LastCheck: now.Unix() - 30,
		Services: []*Service{{ID: "s_0123456789abcdef", Name: "IPP", Port: 631, Interval: 300, Retries: 3, Timeout: 5}}}
	old := &LocalDevice{ID: "l_1123456789abcdef", Name: "old", Address: "10.0.0.8", LastCheck: now.Unix() - 600}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	s.st.st.Locals[d.ID], s.st.st.Locals[old.ID] = d, old
	s.loc.next[d.ID] = now.Add(3570 * time.Second)
	s.loc.next[old.ID] = now.Add(3000 * time.Second)
	s.loc.next["s_0123456789abcdef"] = now.Add(3000 * time.Second)
	s.replanPings(60, now)
	if got := s.loc.next[d.ID]; !got.Equal(now.Add(30 * time.Second)) {
		t.Errorf("ping: next %v, want 30 s after now", got.Sub(now))
	}
	if got := s.loc.next[old.ID]; !got.Equal(now) {
		t.Errorf("overdue ping: next %v, want now", got.Sub(now))
	}
	if got := s.loc.next["s_0123456789abcdef"]; !got.Equal(now.Add(3000 * time.Second)) {
		t.Errorf("service moved: %v", got.Sub(now))
	}
	s.replanPings(3600, now)
	if got := s.loc.next[d.ID]; !got.Equal(now.Add(30 * time.Second)) {
		t.Errorf("a longer interval must not delay a planned ping: %v", got.Sub(now))
	}
}

// Check results live in checks.dat, not in state.dat, and survive a restart.
func TestChecksFile(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	d := &LocalDevice{ID: "l_0123456789abcdef", Name: "printer", Address: "10.0.0.7",
		Services: []*Service{{ID: "s_0123456789abcdef", Name: "IPP", Port: 631, Interval: 300, Retries: 3, Timeout: 5}}}
	s.st.mu.Lock()
	s.st.st.Locals[d.ID] = d
	s.loc.mu.Lock()
	s.record(d, 3*time.Millisecond, nil, time.Now())
	s.recordService(d.Services[0], probe.Outcome{RTT: time.Millisecond}, time.Now())
	s.loc.mu.Unlock()
	s.st.markDirty()
	s.st.writeNow()
	s.st.mu.Unlock()

	js, err := readStateJSON(s.cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(js, []byte(`"hist"`)) || bytes.Contains(js, []byte(`"rtt_us"`)) || !bytes.Contains(js, []byte(`"printer"`)) {
		t.Fatalf("state.dat must hold the configuration only: %s", js)
	}
	if !s.checksDirty {
		t.Fatal("a check result must mark checks.dat for writing")
	}
	s.saveChecks()
	if s.checksDirty {
		t.Error("still marked after saving")
	}

	// A new master on the same data finds the results again.
	keyPath := filepath.Join(filepath.Dir(s.cfg.DataDir), "signing.pub")
	s2, err := New(Config{DataDir: s.cfg.DataDir, SigningKeyPath: keyPath, Version: "0.3.0"})
	if err != nil {
		t.Fatal(err)
	}
	d2 := s2.st.st.Locals[d.ID]
	if d2 == nil || !d2.Up || d2.RTTUs != 3000 || len(d2.Hist) != 1 || len(d2.RTT) != 1 || d2.RTT[0].Max != 3000 || !d2.Services[0].Up || len(d2.Services[0].Hist) != 1 {
		t.Fatalf("results after a restart: %+v", d2)
	}

	// The backup carries checks.dat.
	files, _, err := s2.snapshot()
	if err != nil || files["checks.dat"] == nil {
		t.Fatalf("backup without checks.dat: %v", err)
	}

	// A damaged checks.dat only loses the results; the configuration stays.
	os.WriteFile(filepath.Join(s.cfg.DataDir, checksFileName), []byte("garbage"), 0o600)
	s3, err := New(Config{DataDir: s.cfg.DataDir, SigningKeyPath: keyPath, Version: "0.3.0"})
	if err != nil {
		t.Fatal(err)
	}
	if d3 := s3.st.st.Locals[d.ID]; d3 == nil || d3.Name != "printer" || d3.Checked || len(d3.Services) != 1 {
		t.Fatalf("after a damaged checks.dat: %+v", d3)
	}

	// Failed checks are results too.
	s3.st.mu.Lock()
	s3.loc.mu.Lock()
	s3.recordService(s3.st.st.Locals[d.ID].Services[0], probe.Outcome{Failed: probe.StepConnect, Err: errors.New("refused")}, time.Now())
	s3.loc.mu.Unlock()
	s3.st.mu.Unlock()
	if !s3.checksDirty {
		t.Error("a failed check must mark checks.dat too")
	}
}
