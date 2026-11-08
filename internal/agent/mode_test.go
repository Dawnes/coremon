package agent

import (
	"testing"
)

func TestModeIsStoredAndKept(t *testing.T) {
	dir := t.TempDir()
	a := &Agent{opt: Options{StateDir: dir}, running: map[string]bool{}}
	if a.reportOnly() {
		t.Fatal("new agent must allow changes")
	}
	if err := a.setMode(true); err != nil {
		t.Fatal(err)
	}
	// A restarted agent reads the stored mode.
	b := &Agent{opt: Options{StateDir: dir}}
	if err := loadJSON(b.modePath(), &b.mode); err != nil || !b.mode.ReportOnly {
		t.Fatalf("stored mode not restored: %+v %v", b.mode, err)
	}
	if err := a.setMode(false); err != nil || a.reportOnly() {
		t.Fatal("allow changes failed")
	}
	if !a.claim("c1") || a.claim("c1") {
		t.Fatal("a command must be claimed only once")
	}
}
