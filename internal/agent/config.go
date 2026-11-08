package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Config is written by the installer (/etc/coremon/agent.json).
type Config struct {
	MasterURL string `json:"master_url"` // https://host:8443
	// MasterPins are the two key slots: base64 sha256 of a master SPKI the
	// agent trusts ("" = empty slot). Two slots allow a key rotation: the
	// new key is added while connected with the old one.
	MasterPins [2]string `json:"master_pins"`
	MasterPin  string    `json:"master_pin,omitempty"` // before 0.2.5: one key
	Token      string    `json:"token,omitempty"`
}

var pinRe = regexp.MustCompile(`^[A-Za-z0-9+/]{43}=$`) // base64 of 32 bytes

func cleanPin(p string) string { return strings.TrimPrefix(strings.TrimSpace(p), "sha256//") }

// validPins checks a slot pair: at least one key, every key well-formed.
func validPins(p [2]string) error {
	if p[0] == "" && p[1] == "" {
		return errors.New("no master key")
	}
	for _, k := range p {
		if k != "" && !pinRe.MatchString(k) {
			return fmt.Errorf("invalid master key fingerprint %q", k)
		}
	}
	if p[0] != "" && p[0] == p[1] {
		return errors.New("both slots hold the same key")
	}
	return nil
}

// Identity is created at enrollment (/var/lib/coremon/identity.json).
type Identity struct {
	AgentID    string `json:"agent_id"`
	Secret     string `json:"secret"`
	SigningKey string `json:"signing_key"`
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
	c.MasterPins[0], c.MasterPins[1] = cleanPin(c.MasterPins[0]), cleanPin(c.MasterPins[1])
	if old := cleanPin(c.MasterPin); old != "" && c.MasterPins[0] == "" && c.MasterPins[1] == "" {
		c.MasterPins[0] = old // config from before 0.2.5
	}
	c.MasterPin = ""
	return validPins(c.MasterPins)
}
