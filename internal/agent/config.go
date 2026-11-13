package agent

// Agent config and identity files.

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"coremon/internal/seal"
)

type Config struct {
	MasterURL string `json:"master_url"`

	MasterKeys [2]string `json:"master_keys"`
	Token      string    `json:"token,omitempty"`
}

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

func (c *Config) sealed() bool { return c.MasterKeys[0] != "" || c.MasterKeys[1] != "" }

type Identity struct {
	AgentID    string `json:"agent_id"`
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
	if !c.sealed() {
		return errors.New("no master payload key in the config: run a new install command from the master")
	}
	return validKeys(c.MasterKeys)
}

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
