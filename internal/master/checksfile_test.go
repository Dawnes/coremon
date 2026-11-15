package master

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
	s.recordService(d.Services[0], time.Millisecond, "", true, nil, time.Now())
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
	if d2 == nil || !d2.Up || d2.RTTUs != 3000 || len(d2.Hist) != 1 || !d2.Services[0].Up || len(d2.Services[0].Hist) != 1 {
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
	s3.recordService(s3.st.st.Locals[d.ID].Services[0], 0, "", false, errors.New("refused"), time.Now())
	s3.loc.mu.Unlock()
	s3.st.mu.Unlock()
	if !s3.checksDirty {
		t.Error("a failed check must mark checks.dat too")
	}
}
