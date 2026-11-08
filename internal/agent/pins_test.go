package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	pinA = "rIHN9u/4L8W8KmxvAw0rWrom4Xjgl+gCgVz6LHI73iw="
	pinB = "Qx7Fp2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa8E="
	pinC = "ZZZZZZbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb1A="
)

func TestPinsOnlyChangeOverATrustedConnection(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "agent.json")
	a := &Agent{opt: Options{ConfigPath: cfgPath, StateDir: dir}, cfg: Config{MasterURL: "https://m:8443", MasterPins: [2]string{pinA, ""}}}
	a.cl = newClient(a.cfg.MasterURL, a.cfg.MasterPins)
	a.cl.lastPin = pinA // connected with key A

	// Adding the new key while connected with A: accepted and stored.
	if err := a.setPins([2]string{pinA, pinB}); err != nil {
		t.Fatalf("adding a key over a trusted connection: %v", err)
	}
	var stored Config
	if err := loadJSON(cfgPath, &stored); err != nil || stored.MasterPins != [2]string{pinA, pinB} {
		t.Fatalf("keys not stored: %+v %v", stored.MasterPins, err)
	}
	// A pair without the key of the current connection is refused: the
	// agent never drops the key it is talking to, and a pair it has never
	// been connected with cannot be pushed.
	if err := a.setPins([2]string{pinB, pinC}); err == nil || !strings.Contains(err.Error(), "current connection") {
		t.Fatalf("pair without the current key must be refused, got %v", err)
	}
	// After the master switched, the agent connects with B and drops A.
	a.cl.lastPin = pinB
	if err := a.setPins([2]string{pinB, ""}); err != nil {
		t.Fatalf("dropping the old key: %v", err)
	}
	if p, _ := a.cl.getPins(); p != [2]string{pinB, ""} {
		t.Fatalf("live keys: %v", p)
	}
	for _, bad := range [][2]string{{"", ""}, {"not-a-key", ""}, {pinB, pinB}} {
		if err := validPins(bad); err == nil {
			t.Errorf("validPins(%v) must fail", bad)
		}
	}
}

func TestOldConfigKeyMigrates(t *testing.T) {
	c := Config{MasterURL: "https://m:8443/", MasterPin: "sha256//" + pinA}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	if c.MasterPins != [2]string{pinA, ""} || c.MasterPin != "" {
		t.Fatalf("migration: %+v", c)
	}
	os.Remove("") // keep imports tidy
}
