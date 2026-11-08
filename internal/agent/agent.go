// Package agent implements agent-core: enrollment, the poll loop, command
// dispatch, module management and self-update. It executes no system
// commands itself beyond those needed to manage modules and itself.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"coremon/internal/proto"
)

// Options configure the agent.
type Options struct {
	ConfigPath string
	StateDir   string
	Version    string
}

// Agent is a running agent-core.
type Agent struct {
	opt     Options
	cfg     Config
	id      Identity
	cl      *client
	mods    *moduleManager
	out     *outputBuffer
	mu      sync.Mutex
	running map[string]bool
	exitReq chan int
}

// Run starts the agent and blocks until ctx is cancelled or the agent must
// exit (self-update or uninstall). It returns the desired exit code.
func Run(ctx context.Context, opt Options) (int, error) {
	a := &Agent{opt: opt, running: map[string]bool{}, exitReq: make(chan int, 1)}
	if err := loadJSON(opt.ConfigPath, &a.cfg); err != nil {
		return 1, fmt.Errorf("read config %s: %w", opt.ConfigPath, err)
	}
	if err := a.cfg.validate(); err != nil {
		return 1, fmt.Errorf("config: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(opt.StateDir, "modules"), 0o700); err != nil {
		return 1, err
	}
	a.cl = newClient(a.cfg.MasterURL, a.cfg.MasterPin)
	a.mods = &moduleManager{dir: filepath.Join(opt.StateDir, "modules")}

	if err := a.ensureEnrolled(ctx); err != nil {
		return 1, err
	}
	a.cl.auth = a.id.AgentID + ":" + a.id.Secret
	a.out = newOutputBuffer(a.cl)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go a.out.loop(ctx)
	go a.pollLoop(ctx)

	log.Printf("coremon agent %s running as %s, master %s", opt.Version, a.id.AgentID, a.cfg.MasterURL)
	select {
	case <-ctx.Done():
		return 0, nil
	case code := <-a.exitReq:
		return code, nil
	}
}

func (a *Agent) identityPath() string { return filepath.Join(a.opt.StateDir, "identity.json") }

func (a *Agent) ensureEnrolled(ctx context.Context) error {
	if err := loadJSON(a.identityPath(), &a.id); err == nil && a.id.AgentID != "" {
		return nil
	}
	if a.cfg.Token == "" {
		return errors.New("not enrolled and no enrollment token in config")
	}
	backoff := 10 * time.Second
	for {
		var resp proto.EnrollResponse
		err := a.cl.postJSON(ctx, "/api/agent/enroll", proto.EnrollRequest{
			Token: a.cfg.Token, CoreVersion: a.opt.Version, Facts: collectFacts(),
		}, &resp, 30*time.Second)
		if err == nil {
			a.id = Identity{AgentID: resp.AgentID, Secret: resp.Secret, SigningKey: resp.SigningKey}
			if err := saveJSON(a.identityPath(), a.id, 0o600); err != nil {
				return fmt.Errorf("save identity: %w", err)
			}
			// The token is single-use; drop it from the config.
			a.cfg.Token = ""
			if err := saveJSON(a.opt.ConfigPath, a.cfg, 0o600); err != nil {
				log.Printf("warning: could not remove token from config: %v", err)
			}
			log.Printf("enrolled as %s; waiting for approval in the master UI", a.id.AgentID)
			return nil
		}
		log.Printf("enrollment failed: %v (retrying in %s)", err, backoff)
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

func (a *Agent) runningIDs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	ids := []string{}
	for id := range a.running {
		ids = append(ids, id)
	}
	return ids
}

func (a *Agent) pollLoop(ctx context.Context) {
	backoff := 2 * time.Second
	for ctx.Err() == nil {
		var resp proto.PollResponse
		err := a.cl.postJSON(ctx, "/api/agent/poll", proto.PollRequest{
			Protocol:    proto.ProtocolVersion,
			CoreVersion: a.opt.Version,
			Facts:       collectFacts(),
			Modules:     a.mods.list(),
			Running:     a.runningIDs(),
		}, &resp, 75*time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			wait := backoff
			var he *httpError
			if errors.As(err, &he) && (he.Status == 401 || he.Status == 403) {
				wait = 5 * time.Minute
				log.Printf("master rejected this agent (%d): revoked or removed; retrying in %s", he.Status, wait)
			} else {
				log.Printf("poll failed: %v (retry in %s)", err, wait)
				if backoff < time.Minute {
					backoff *= 2
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			continue
		}
		backoff = 2 * time.Second
		for _, c := range resp.Commands {
			a.mu.Lock()
			dup := a.running[c.ID]
			if !dup {
				a.running[c.ID] = true
			}
			a.mu.Unlock()
			if !dup {
				go a.execute(ctx, c)
			}
		}
	}
}

func (a *Agent) execute(ctx context.Context, c proto.Command) {
	logf := func(format string, args ...any) { a.out.add(c.ID, fmt.Sprintf(format, args...)) }
	timeout := time.Duration(c.Timeout) * time.Second
	if timeout <= 0 {
		timeout = time.Hour
	}
	var data json.RawMessage
	var err error
	after := func() {}

	switch c.Type {
	case proto.CmdModuleCall:
		data, err = a.mods.call(ctx, c.Module, c.Action, c.Params, timeout, func(s string) { a.out.add(c.ID, s) })
	case proto.CmdModuleInstall:
		name := ""
		if c.Artifact != nil {
			name = c.Artifact.Name
		}
		if !a.cfg.moduleAllowed(name) {
			err = fmt.Errorf("module %q is not allowed by this agent's local policy (allow_modules in %s)", name, a.opt.ConfigPath)
		} else {
			err = a.mods.install(ctx, a.cl, a.id.SigningKey, c.Artifact, logf)
		}
		if err == nil {
			// Report the new module right away instead of on the next poll.
			for _, m := range a.mods.list() {
				if m.Name == name {
					data, _ = json.Marshal(m)
				}
			}
		}
	case proto.CmdModuleRemove:
		err = a.mods.remove(c.Module)
		if err == nil {
			logf("module %s removed", c.Module)
		}
	case proto.CmdCoreUpdate:
		err = selfUpdate(ctx, a.cl, a.id.SigningKey, c.Artifact, logf)
		if err == nil {
			logf("agent-core replaced; restarting")
			after = func() { a.requestExit(exitRestart) }
		}
	case proto.CmdUninstall:
		logf("uninstalling agent from this system")
		after = func() {
			if err := uninstall(a.opt); err != nil {
				log.Printf("uninstall: %v", err)
			}
			a.requestExit(0)
		}
	default:
		err = fmt.Errorf("unknown command type %q", c.Type)
	}

	res := proto.Result{CommandID: c.ID, OK: err == nil, Data: data}
	if err != nil {
		res.Error = err.Error()
		logf("ERROR: %v", err)
	}
	a.out.flush(ctx)
	for attempt := 0; attempt < 5; attempt++ {
		if perr := a.cl.postJSON(ctx, "/api/agent/result", res, nil, 30*time.Second); perr == nil {
			break
		} else {
			log.Printf("result upload failed: %v", perr)
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
		}
	}
	a.mu.Lock()
	delete(a.running, c.ID)
	a.mu.Unlock()
	after()
}

// exitRestart makes systemd (Restart=on-failure) start the new binary.
const exitRestart = 75

func (a *Agent) requestExit(code int) {
	select {
	case a.exitReq <- code:
	default:
	}
}
