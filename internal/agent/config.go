package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"coremon/internal/seal"
)

// Config is written by the installer (/etc/coremon/agent.json).
type Config struct {
	MasterURL string `json:"master_url"` // https://host:8443
	// MasterKeys are the two slots of master payload keys the agent trusts
	// ("" = empty slot). Two slots allow a key rotation: the new key is
	// added in a reply signed with a key the agent already trusts.
	MasterKeys [2]string `json:"master_keys"`
	// Before 0.3.0 agents pinned the master's HTTPS certificate instead.
	// Only used once, for the hand-over to the payload key.
	MasterPins [2]string `json:"master_pins,omitempty"`
	MasterPin  string    `json:"master_pin,omitempty"`
	Token      string    `json:"token,omitempty"`
}

var pinRe = regexp.MustCompile(`^[A-Za-z0-9+/]{43}=$`) // base64 of 32 bytes

func cleanPin(p string) string { return strings.TrimPrefix(strings.TrimSpace(p), "sha256//") }

// validKeys checks a pair of payload key slots.
func validKeys(p [2]string) error {
	if p[0] == "" && p[1] == "" {
		return errors.New("no master key")
	}
	for _, k := range p {
		if k == "" {
			continue
		}
		if _, err := seal.ParsePublic(k); err != nil {
			return fmt.Errorf("invalid master key %q", k)
		}
	}
	if p[0] != "" && p[0] == p[1] {
		return errors.New("both slots hold the same key")
	}
	return nil
}

// sealed reports whether the config has payload keys (0.3.0+).
func (c *Config) sealed() bool { return c.MasterKeys[0] != "" || c.MasterKeys[1] != "" }

// legacyPins are the HTTPS key fingerprints of a config from before 0.3.0.
func (c *Config) legacyPins() [2]string {
	p := [2]string{cleanPin(c.MasterPins[0]), cleanPin(c.MasterPins[1])}
	if p[0] == "" && p[1] == "" {
		p[0] = cleanPin(c.MasterPin)
	}
	return p
}

// Identity is created at enrollment (/var/lib/coremon/identity.json).
type Identity struct {
	AgentID    string `json:"agent_id"`
	Secret     string `json:"secret,omitempty"` // before 0.3.0 only; removed after the hand-over
	SigningKey string `json:"signing_key"`      // release signing key
}

func loadJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func saveJSON(path string, v any, mode os.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (c *Config) validate() error {
	if !strings.HasPrefix(c.MasterURL, "https://") {
		return errors.New("master_url must start with https://")
	}
	c.MasterURL = strings.TrimRight(c.MasterURL, "/")
	if c.sealed() {
		return validKeys(c.MasterKeys)
	}
	p := c.legacyPins()
	if p[0] == "" && p[1] == "" {
		return errors.New("no master key in the config: run a new install command")
	}
	for _, k := range p {
		if k != "" && !pinRe.MatchString(k) {
			return fmt.Errorf("invalid master key fingerprint %q", k)
		}
	}
	return nil
}

// loadAgentKey reads the agent's own key, creating it on first use.
func loadAgentKey(path string) (ed25519.PrivateKey, error) {
	if b, err := os.ReadFile(path); err == nil {
		seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if err != nil || len(seed) != ed25519.SeedSize {
			return nil, errors.New("agent key file is damaged")
		}
		return ed25519.NewKeyFromSeed(seed), nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	_, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(k.Seed())+"\n"), 0o600); err != nil {
		return nil, err
	}
	return k, nil
}
