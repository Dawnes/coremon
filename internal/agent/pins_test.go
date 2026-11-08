package agent

import (
	"crypto/ed25519"
	"crypto/rand"
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
	old := Config{MasterURL: "https://m:8443/", MasterPin: "sha256//rIHN9u/4L8W8KmxvAw0rWrom4Xjgl+gCgVz6LHI73iw="}
	if err := old.validate(); err != nil || old.sealed() || old.legacyPins()[0] == "" {
		t.Fatalf("config from before 0.2.5: %v %+v", err, old)
	}
	newer := Config{MasterURL: "https://m:8443", MasterKeys: [2]string{payloadKey(t), ""}}
	if err := newer.validate(); err != nil || !newer.sealed() {
		t.Fatalf("sealed config: %v", err)
	}
	if err := (&Config{MasterURL: "https://m"}).validate(); err == nil {
		t.Fatal("a config without any key must be refused")
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
