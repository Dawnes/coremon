// Package agent implements the coremon agent: enrollment, the poll loop,
// command dispatch, the built-in features and self-update.
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

	"coremon/internal/feature"
	"coremon/internal/feature/updates"
	"coremon/internal/proto"
)

// features are the agent's built-in functions.
func features() []feature.Feature {
	return []feature.Feature{updates.New()}
}

// Options configure the agent.
type Options struct {
	ConfigPath string
	StateDir   string
	Version    string
}

// Agent is a running agent.
type Agent struct {
	opt       Options
	cfg       Config
	id        Identity
	cl        *client
	out       *outputBuffer
	feats     map[string]feature.Feature
	infos     []proto.FeatureInfo
	locks     map[string]*sync.Mutex // one action per feature at a time
	mu        sync.Mutex
	running   map[string]bool
	heartbeat int // seconds, as told by the master
	exitReq   chan int
}

// Run starts the agent and blocks until ctx is cancelled or the agent must
// exit (self-update or uninstall). It returns the desired exit code.
func Run(ctx context.Context, opt Options) (int, error) {
	a := &Agent{opt: opt, running: map[string]bool{}, exitReq: make(chan int, 1),
		feats: map[string]feature.Feature{}, locks: map[string]*sync.Mutex{}, heartbeat: 60}
	for _, f := range features() {
		info := f.Info()
		a.feats[info.Name] = f
		a.infos = append(a.infos, info)
		a.locks[info.Name] = &sync.Mutex{}
	}
	if err := loadJSON(opt.ConfigPath, &a.cfg); err != nil {
		return 1, fmt.Errorf("read config %s: %w", opt.ConfigPath, err)
	}
	if err := a.cfg.validate(); err != nil {
		return 1, fmt.Errorf("config: %w", err)
	}
	if err := os.MkdirAll(opt.StateDir, 0o700); err != nil {
		return 1, err
	}
	// Agents before 0.2 installed separate modules here; everything is
	// built in now.
	os.RemoveAll(filepath.Join(opt.StateDir, "modules"))
	a.cl = newClient(a.cfg.MasterURL, a.cfg.MasterPin)

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
		// The master holds the request for up to one heartbeat.
		timeout := time.Duration(a.heartbeat+30) * time.Second
		err := a.cl.postJSON(ctx, "/api/agent/poll", proto.PollRequest{
			Protocol:    proto.ProtocolVersion,
			CoreVersion: a.opt.Version,
			Facts:       collectFacts(),
			Features:    a.infos,
			Running:     a.runningIDs(),
		}, &resp, timeout)
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
		if resp.Heartbeat >= 5 && resp.Heartbeat <= 600 {
			a.heartbeat = resp.Heartbeat
		}
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

// callFeature runs one action of a built-in feature.
func (a *Agent) callFeature(ctx context.Context, c proto.Command, timeout time.Duration) (data json.RawMessage, err error) {
	f := a.feats[c.Feature]
	if f == nil {
		return nil, fmt.Errorf("this agent has no feature %q", c.Feature)
	}
	l := a.locks[c.Feature]
	l.Lock()
	defer l.Unlock()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("internal error in %s.%s: %v", c.Feature, c.Action, p)
		}
	}()
	fc := feature.NewCtx(ctx, c.Params, func(s string) { a.out.add(c.ID, s) })
	res, err := f.Call(fc, c.Action)
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("action timed out after %s", timeout)
	}
	if res != nil {
		data, _ = json.Marshal(res)
	}
	return data, err
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
	case proto.CmdAction:
		data, err = a.callFeature(ctx, c, timeout)
	case proto.CmdCoreUpdate:
		err = selfUpdate(ctx, a.cl, a.id.SigningKey, c.Artifact, logf)
		if err == nil {
			logf("agent replaced; restarting")
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
