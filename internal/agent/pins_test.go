package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"coremon/internal/seal"
)

func payloadKey(t *testing.T) string {
	k, err := seal.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return k.Public().String()
}

func TestKeysOnlyChangeOverATrustedReply(t *testing.T) {
	A, B, C := payloadKey(t), payloadKey(t), payloadKey(t)
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "agent.json")
	_, ak, _ := ed25519.GenerateKey(rand.Reader)
	a := &Agent{opt: Options{ConfigPath: cfgPath, StateDir: dir}, cfg: Config{MasterURL: "https://m:8443", MasterKeys: [2]string{A, ""}}}
	a.cl = newSealedClient(a.cfg.MasterURL, a.cfg.MasterKeys, ak)
	a.cl.lastPin = A // the last verified reply was signed with key A

	if err := a.setPins([2]string{A, B}); err != nil {
		t.Fatalf("adding a key in a reply signed by a trusted key: %v", err)
	}
	var stored Config
	if err := loadJSON(cfgPath, &stored); err != nil || stored.MasterKeys != [2]string{A, B} {
		t.Fatalf("keys not stored: %+v %v", stored.MasterKeys, err)
	}
	if err := a.setPins([2]string{B, C}); err == nil || !strings.Contains(err.Error(), "current connection") {
		t.Fatalf("pair without the current key must be refused, got %v", err)
	}
	a.cl.lastPin = B // after the switch the master answers with B
	if err := a.setPins([2]string{B, ""}); err != nil {
		t.Fatalf("dropping the old key: %v", err)
	}
	if p, _ := a.cl.getPins(); p != [2]string{B, ""} {
		t.Fatalf("live keys: %v", p)
	}
	for _, bad := range [][2]string{{"", ""}, {"not-a-key", ""}, {B, B}} {
		if err := validKeys(bad); err == nil {
			t.Errorf("validKeys(%v) must fail", bad)
		}
	}
}

func TestConfigFormats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.json")
	// A config without master keys is refused.
	os.WriteFile(path, []byte(`{"master_url":"https://m:8443"}`), 0o600)
	var none Config
	loadJSON(path, &none)
	if err := none.validate(); err == nil || !strings.Contains(err.Error(), "new install command") {
		t.Fatalf("config without keys must be refused, got %v", err)
	}
	k := payloadKey(t)
	os.WriteFile(path, []byte(`{"master_url":"https://m:8443","master_keys":["`+k+`",""]}`), 0o600)
	var cfg Config
	loadJSON(path, &cfg)
	if err := cfg.validate(); err != nil || !cfg.sealed() {
		t.Fatalf("sealed config: %v", err)
	}
}

func TestAgentKeyIsKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.key")
	k1, err := loadAgentKey(path)
	if err != nil {
		t.Fatal(err)
	}
	k2, _ := loadAgentKey(path)
	if !k1.Equal(k2) {
		t.Fatal("the agent key must be created once and then reused")
	}
}
