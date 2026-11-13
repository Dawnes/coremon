package agent

// Master key slots and re-enrollment.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"coremon/internal/seal"
)

// Master key slots, config clean-up, re-enrollment.

func pinsNow(cl *client) [2]string {
	p, _ := cl.getPins()
	return p
}

func (c *client) setAgentID(id string) {
	c.mu.Lock()
	c.agentID = id
	c.mu.Unlock()
}

func shortPin(p string) string {
	if p == "" {
		return "(empty)"
	}
	f := seal.Fingerprint(p)
	return f[:6] + "…" + f[len(f)-6:]
}

func (a *Agent) setPins(p [2]string) error {
	if err := validKeys(p); err != nil {
		return err
	}
	// A new key is only accepted alongside the key of the current connection.
	_, last := a.cl.getPins()
	if last == "" || (p[0] != last && p[1] != last) {
		return errors.New("refused: the new keys must include the key of the current connection")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg.MasterKeys == p {
		return nil
	}
	cfg := a.cfg
	cfg.MasterKeys = p
	if err := saveJSON(a.opt.ConfigPath, cfg, 0o600); err != nil {
		return fmt.Errorf("store master keys: %w", err)
	}
	a.cfg = cfg
	a.cl.setPins(p)
	log.Printf("master keys updated: slot 1 %s, slot 2 %s", shortPin(p[0]), shortPin(p[1]))
	return nil
}

func (a *Agent) tokenPending() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg.Token != ""
}

func (a *Agent) dropToken() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg.Token = ""
	if err := saveJSON(a.opt.ConfigPath, a.cfg, 0o600); err != nil {
		log.Printf("warning: could not remove token from config: %v", err)
	}
}

func (a *Agent) reenroll(ctx context.Context) error {
	old := a.id
	os.Remove(a.identityPath())
	a.id = Identity{}
	a.cl.setAgentID("")
	if err := a.ensureEnrolled(ctx); err != nil {
		a.id = old
		saveJSON(a.identityPath(), old, 0o600)
		a.cl.setAgentID(old.AgentID)
		log.Printf("re-enrollment failed: %v", err)
		return err
	}
	a.cl.setAgentID(a.id.AgentID)
	return nil
}
