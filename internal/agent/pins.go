package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
)

// Master key slots and re-enrollment.

func pinsNow(cl *client) [2]string {
	p, _ := cl.getPins()
	return p
}

func shortPin(p string) string {
	if p == "" {
		return "(empty)"
	}
	if len(p) > 12 {
		return p[:6] + "…" + p[len(p)-6:]
	}
	return p
}

// setPins replaces both key slots. The new pair must still contain the key
// of the connection the command arrived on: a key can only be added over a
// connection verified with a key the agent already trusts, and the agent
// can never be told to drop the key it is talking to right now.
func (a *Agent) setPins(p [2]string) error {
	p[0], p[1] = cleanPin(p[0]), cleanPin(p[1])
	if err := validPins(p); err != nil {
		return err
	}
	_, last := a.cl.getPins()
	if last == "" || (p[0] != last && p[1] != last) {
		return errors.New("refused: the new keys must include the key of the current connection")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg.MasterPins == p {
		return nil
	}
	cfg := a.cfg
	cfg.MasterPins = p
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

// reenroll registers this system again with the token from the config,
// replacing the identity the master no longer knows.
func (a *Agent) reenroll(ctx context.Context) error {
	old := a.id
	os.Remove(a.identityPath())
	a.id = Identity{}
	if err := a.ensureEnrolled(ctx); err != nil {
		a.id = old
		saveJSON(a.identityPath(), old, 0o600)
		log.Printf("re-enrollment failed: %v", err)
		return err
	}
	a.cl.auth = a.id.AgentID + ":" + a.id.Secret
	return nil
}
