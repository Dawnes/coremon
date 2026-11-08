package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Config is written by the installer (/etc/coremon/agent.json).
type Config struct {
	MasterURL string `json:"master_url"` // https://host:8443
	MasterPin string `json:"master_pin"` // base64 sha256 of the master's SPKI
	Token     string `json:"token,omitempty"`
	// AllowModules is the local policy: which modules the master may install.
	// ["*"] = any signed module, [] = none, or a list of names.
	AllowModules []string `json:"allow_modules"`
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
	c.MasterPin = strings.TrimPrefix(strings.TrimSpace(c.MasterPin), "sha256//")
	if c.MasterPin == "" {
		return errors.New("master_pin is required")
	}
	return nil
}

func (c *Config) moduleAllowed(name string) bool {
	for _, a := range c.AllowModules {
		if a == "*" || a == name {
			return true
		}
	}
	return false
}
