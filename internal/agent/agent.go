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

	"crypto/ed25519"

	"coremon/internal/feature"
	"coremon/internal/feature/apps"
	"coremon/internal/feature/system"
	"coremon/internal/feature/updates"
	"coremon/internal/proto"
	"coremon/internal/seal"
)

// features are the agent's built-in functions.
func features() []feature.Feature {
	return []feature.Feature{updates.New(), system.New(), apps.New()}
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
	agentKey  ed25519.PrivateKey // the agent's own key (sealed method)
	mode      Mode               // stored on disk; guarded by mu
}

// Mode is the agent's own record of report-only mode. It survives restarts
// and is only changed by an explicit agent.mode command from the master.
type Mode struct {
	ReportOnly bool  `json:"report_only"`
	Since      int64 `json:"since"`
}

func (a *Agent) modePath() string { return filepath.Join(a.opt.StateDir, "mode.json") }

func (a *Agent) reportOnly() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.mode.ReportOnly
}

// setMode changes and stores the mode.
func (a *Agent) setMode(reportOnly bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.mode.ReportOnly == reportOnly {
		return nil
	}
	m := Mode{ReportOnly: reportOnly, Since: time.Now().Unix()}
	if err := saveJSON(a.modePath(), m, 0o600); err != nil {
		return err
	}
	a.mode = m
	if reportOnly {
		log.Printf("report-only mode ON: no checks, installs or reboots until the master allows changes")
	} else {
		log.Printf("report-only mode OFF: changes allowed")
	}
	return nil
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
	if err := loadJSON(a.modePath(), &a.mode); err != nil && !os.IsNotExist(err) {
		// An unreadable mode file must not silently allow changes.
		log.Printf("mode file unreadable (%v): staying in report-only mode until the master decides", err)
		a.mode = Mode{ReportOnly: true}
	}
	if a.mode.ReportOnly {
		log.Printf("starting in report-only mode")
	}
	key, err := loadAgentKey(filepath.Join(opt.StateDir, "agent.key"))
	if err != nil {
		return 1, fmt.Errorf("agent key: %w", err)
	}
	a.agentKey = key
	a.cleanConfig()
	a.cl = newSealedClient(a.cfg.MasterURL, a.cfg.MasterKeys, key)

	if err := a.ensureEnrolled(ctx); err != nil {
		return 1, err
	}
	a.cl.setAgentID(a.id.AgentID)
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
			AgentKey: seal.AgentKey(a.agentKey.Public().(ed25519.PublicKey)),
		}, &resp, 30*time.Second)
		if err == nil {
			a.id = Identity{AgentID: resp.AgentID, SigningKey: resp.SigningKey}
			if err := saveJSON(a.identityPath(), a.id, 0o600); err != nil {
				return fmt.Errorf("save identity: %w", err)
			}
			// The token is single-use; drop it from the config.
			a.dropToken()
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
			ReportOnly:  a.reportOnly(),
			Pins:        pinsNow(a.cl),
		}, &resp, timeout)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			wait := backoff
			var he *httpError
			if errors.As(err, &he) && (he.Status == 401 || he.Status == 403) {
				if a.tokenPending() {
					// The master no longer knows this system (e.g. a new
					// master) and a fresh install command left a token:
					// register again.
					log.Printf("master does not know this agent (%d); enrolling again with the new token", he.Status)
					if err := a.reenroll(ctx); err == nil {
						backoff = 2 * time.Second
						continue
					}
				}
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
		if a.tokenPending() {
			// The master still knows this system: the token from a repeated
			// install command is not needed.
			a.dropToken()
		}
		if a.id.Secret != "" {
			// The first sealed exchange worked: the hand-over is complete and
			// the old shared secret is not needed anymore.
			a.id.Secret = ""
			if err := saveJSON(a.identityPath(), a.id, 0o600); err != nil {
				log.Printf("warning: could not remove the old secret: %v", err)
			} else {
				log.Printf("now using the master's payload key; old shared secret removed")
			}
		}
		// Mode changes are applied first and in order, so an action sent
		// right after "allow changes" is not refused (and one sent after
		// "report only" is).
		for _, c := range resp.Commands {
			if immediate(c.Type) && a.claim(c.ID) {
				a.execute(ctx, c)
			}
		}
		for _, c := range resp.Commands {
			if immediate(c.Type) {
				continue
			}
			switch a.claimLimited(c.ID) {
			case claimed:
				go a.execute(ctx, c)
			case claimBusy:
				go a.reject(ctx, c, errBusy)
			}
		}
	}
}

// immediate commands are small and handled in order before the others,
// outside the running-command limit: mode and key changes (so later
// commands in the same reply see them) and ping.
func immediate(t string) bool {
	return t == proto.CmdSetMode || t == proto.CmdSetPins || t == proto.CmdPing
}

// claim marks a command as running; false if it already is.
func (a *Agent) claim(id string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running[id] {
		return false
	}
	a.running[id] = true
	return true
}

// maxRunning caps the commands in progress at once, so a faulty master
// cannot exhaust the system by sending thousands of commands.
const maxRunning = 16

type claimResult int

const (
	claimDuplicate claimResult = iota // already running: ignore
	claimed
	claimBusy // too many in progress: refuse
)

// claimLimited marks a command as running unless it already is or the
// limit is reached.
func (a *Agent) claimLimited(id string) claimResult {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running[id] {
		return claimDuplicate
	}
	if len(a.running) >= maxRunning {
		return claimBusy
	}
	a.running[id] = true
	return claimed
}

var errBusy = fmt.Errorf("agent busy: %d commands already in progress; try again later", maxRunning)

// reject reports a command as failed without running it.
func (a *Agent) reject(ctx context.Context, c proto.Command, err error) {
	res := proto.Result{CommandID: c.ID, OK: false, Error: err.Error()}
	for attempt := 0; attempt < 3; attempt++ {
		if perr := a.cl.postJSON(ctx, "/api/agent/result", res, nil, 30*time.Second); perr == nil {
			return
		}
		time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
	}
}

// errReportOnly is returned for every action while in report-only mode.
var errReportOnly = errors.New("this system is in report-only mode: it refuses checks, installs and reboots until the master allows changes")

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
		// Report only refuses every action, except the application scan:
		// it only reads.
		if a.reportOnly() && c.Feature != "apps" {
			err = errReportOnly
			break
		}
		data, err = a.callFeature(ctx, c, timeout)
		healthStale() // an install or upgrade changes the last upgrade
	case proto.CmdPing:
		// Answer at once; nothing is done on the system.
		keys, used := a.cl.getPins()
		data, _ = json.Marshal(proto.PingReply{TimeMs: time.Now().UnixMilli(), Version: a.opt.Version, KeyUsed: used, Keys: keys})
	case proto.CmdSetPins:
		var p proto.PinParams
		if err = json.Unmarshal(c.Params, &p); err == nil {
			err = a.setPins(p.Pins)
		}
		if err == nil {
			data, _ = json.Marshal(p)
			logf("master keys: slot 1 %s, slot 2 %s", shortPin(p.Pins[0]), shortPin(p.Pins[1]))
		}
	case proto.CmdSetMode:
		var p proto.ModeParams
		if err = json.Unmarshal(c.Params, &p); err == nil {
			err = a.setMode(p.ReportOnly)
		}
		if err == nil {
			data, _ = json.Marshal(p)
			if p.ReportOnly {
				logf("report-only mode on")
			} else {
				logf("changes allowed")
			}
		}
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
