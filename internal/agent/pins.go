package agent

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"coremon/internal/seal"
)

// Master key slots, the hand-over from the old method, re-enrollment.

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

// setPins replaces both payload key slots. The new pair must still contain
// the key that signed the reply carrying this command: a key can only be
// added in a reply signed by a key the agent already trusts, and the agent
// never drops the key it is talking to.
func (a *Agent) setPins(p [2]string) error {
	if err := validKeys(p); err != nil {
		return err
	}
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

// handOver moves an agent of the old method (pinned HTTPS certificate,
// shared secret) to the payload key: over that still-pinned connection it
// registers its own key and receives the master's payload keys. Trust is
// carried over without a gap.
func (a *Agent) handOver(ctx context.Context) error {
	if err := loadJSON(a.identityPath(), &a.id); err != nil || a.id.AgentID == "" || a.id.Secret == "" {
		return errors.New("this config has no payload key and the system is not enrolled the old way: run a new install command")
	}
	cl := newLegacyClient(a.cfg.MasterURL, a.cfg.legacyPins())
	cl.auth = a.id.AgentID + ":" + a.id.Secret
	backoff := 10 * time.Second
	for {
		var resp struct {
			MasterKeys [2]string `json:"master_keys"`
		}
		err := cl.postJSON(ctx, "/api/agent/migrate", map[string]string{
			"agent_key": seal.AgentKey(a.agentKey.Public().(ed25519.PublicKey)),
		}, &resp, 30*time.Second)
		if err == nil {
			if err = validKeys(resp.MasterKeys); err == nil {
				a.mu.Lock()
				cfg := a.cfg
				cfg.MasterKeys, cfg.MasterPins, cfg.MasterPin = resp.MasterKeys, [2]string{}, ""
				err = saveJSON(a.opt.ConfigPath, cfg, 0o600)
				if err == nil {
					a.cfg = cfg
				}
				a.mu.Unlock()
				if err == nil {
					log.Printf("hand-over: received the master's payload key %s", shortPin(resp.MasterKeys[0]))
					return nil
				}
			}
		}
		log.Printf("hand-over to the payload key failed: %v (retrying in %s)", err, backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		if backoff < 5*time.Minute {
			backoff *= 2
		}
	}
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
