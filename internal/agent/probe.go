package agent

// Remote probe: the agent checks agentless devices for the master (ping,
// TCP, HTTPS, DNS and certificates), on its own schedule, from a complete check list the master
// sends. The list is kept in memory only: after a restart the agent starts
// empty and the master sends the list again (unless an admin cleared it).

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"coremon/internal/probe"
	"coremon/internal/proto"
)

const (
	probeMaxChecks   = 500 // the master allows 200; a little room
	probeAtOnce      = 32
	probeRetryEvery  = 30 * time.Second
	probeKeep        = 5000 // results kept while the master cannot be reached
	probeSendAt      = 500  // results waiting: send right away
	probeBatch       = 1000 // results per message
	probeMinInterval = 10
)

type probeFuncs struct {
	ping func(ctx context.Context, ip net.IP, timeout time.Duration) (time.Duration, error)
	svc  func(ctx context.Context, addr string, s probe.Service, timeout time.Duration) probe.Outcome
}

// svcSpec: the service check of a tcp, https or dns check.
func svcSpec(c proto.ProbeCheck) probe.Service {
	return probe.Service{Method: c.Kind, Port: c.Port, Greeting: c.Greeting, Host: c.Host, Path: c.Path, Codes: c.Codes,
		CertValid: c.CertValid, CertDays: c.CertDays, RType: c.RType, Expect: c.Expect, MaxMs: c.MaxMs, CertTime: c.CertTime, Remote: c.Remote}
}

type pcheck struct {
	c     proto.ProbeCheck
	next  time.Time
	busy  bool
	fails int
	state int // 0 not decided, 1 up, 2 down
}

type presult struct {
	r  proto.ProbeResult
	at time.Time
}

type prober struct {
	mu      sync.Mutex
	ver     string
	checks  map[string]*pcheck
	results []presult
	urgent  chan struct{}
	fn      probeFuncs
	sem     chan struct{}
}

func newProber() *prober {
	return &prober{checks: map[string]*pcheck{}, urgent: make(chan struct{}, 1), sem: make(chan struct{}, probeAtOnce),
		fn: probeFuncs{ping: probe.Ping, svc: probe.Check}}
}

func validProbeList(l proto.ProbeList) error {
	if len(l.Checks) > probeMaxChecks {
		return fmt.Errorf("too many checks: %d (at most %d)", len(l.Checks), probeMaxChecks)
	}
	if len(l.Version) > 64 {
		return errors.New("invalid list version")
	}
	seen := map[string]bool{}
	for _, c := range l.Checks {
		switch {
		case c.ID == "" || len(c.ID) > 64 || seen[c.ID]:
			return fmt.Errorf("invalid or duplicate check id %q", c.ID)
		case net.ParseIP(c.Address) == nil:
			return fmt.Errorf("check %s: %q is not an IP address", c.ID, c.Address)
		case c.Kind != "ping" && c.Kind != probe.MethodTCP && c.Kind != probe.MethodHTTPS && c.Kind != probe.MethodDNS && c.Kind != probe.MethodCert:
			return fmt.Errorf("check %s: unknown kind %q", c.ID, c.Kind)
		case c.Interval < probeMinInterval || c.Interval > 86400:
			return fmt.Errorf("check %s: invalid interval", c.ID)
		case c.Timeout < 1 || c.Timeout > 60:
			return fmt.Errorf("check %s: invalid timeout", c.ID)
		case c.Retries < 0 || c.Retries > 10 || c.DownAfter < 0 || c.DownAfter > 10:
			return fmt.Errorf("check %s: invalid retries", c.ID)
		}
		if c.Kind != "ping" {
			if err := probe.Validate(svcSpec(c)); err != nil {
				return fmt.Errorf("check %s: %v", c.ID, err)
			}
		}
		seen[c.ID] = true
	}
	return nil
}

// setList replaces the whole list. Checks that did not change keep their
// schedule and state; new ones start within 20 seconds, spread out.
func (p *prober) setList(l proto.ProbeList, now time.Time) error {
	if err := validProbeList(l); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	old := p.checks
	p.checks = map[string]*pcheck{}
	for i, c := range l.Checks {
		if o := old[c.ID]; o != nil && o.c == c {
			p.checks[c.ID] = o
			continue
		}
		p.checks[c.ID] = &pcheck{c: c, next: now.Add(time.Duration(i%20) * time.Second)}
	}
	p.ver = l.Version
	if len(p.checks) == 0 {
		p.ver = ""
	}
	return nil
}

// clear drops the list and every result not sent yet.
func (p *prober) clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.checks, p.results, p.ver = map[string]*pcheck{}, nil, ""
}

func (p *prober) version() (string, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ver, len(p.checks)
}

// take: the version and up to max results waiting (oldest first), removed
// until putBack returns them after a failed send.
func (p *prober) take(max int, now time.Time) (proto.ProbeReport, []presult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := min(max, len(p.results))
	taken := append([]presult(nil), p.results[:n]...)
	p.results = append([]presult(nil), p.results[n:]...)
	rep := proto.ProbeReport{Version: p.ver}
	for _, x := range taken {
		r := x.r
		r.AgoMs = now.Sub(x.at).Milliseconds()
		if r.AgoMs < 0 {
			r.AgoMs = 0
		}
		rep.Results = append(rep.Results, r)
	}
	return rep, taken
}

func (p *prober) putBack(rs []presult) {
	if len(rs) == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.results = append(append([]presult(nil), rs...), p.results...)
	if over := len(p.results) - probeKeep; over > 0 {
		p.results = p.results[over:] // the oldest go first
	}
}

func (p *prober) pending() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.results)
}

func (p *prober) signal() {
	select {
	case p.urgent <- struct{}{}:
	default:
	}
}

// due marks the checks whose time has come as busy and returns them.
func (p *prober) due(now time.Time) []proto.ProbeCheck {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []proto.ProbeCheck
	for _, c := range p.checks {
		if !c.busy && !now.Before(c.next) {
			c.busy = true
			out = append(out, c.c)
		}
	}
	return out
}

// run performs one check.
func (p *prober) run(ctx context.Context, c proto.ProbeCheck) proto.ProbeResult {
	timeout := time.Duration(c.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(ctx, 4*timeout+time.Second) // HTTPS may need a second connection
	defer cancel()
	res := proto.ProbeResult{ID: c.ID}
	var rtt time.Duration
	var err error
	if c.Kind == "ping" {
		rtt, err = p.fn.ping(ctx, net.ParseIP(c.Address), timeout)
		var se *probe.SetupError
		if errors.As(err, &se) {
			res.Setup = true
		}
	} else {
		o := p.fn.svc(ctx, c.Address, svcSpec(c), timeout)
		rtt, res.Banner, err, res.Cert = o.RTT, o.Detail, o.Err, o.Cert
		if err != nil {
			res.Failed = o.Failed
		}
		if len(res.Banner) > 200 {
			res.Banner = res.Banner[:200]
		}
	}
	res.OK = err == nil
	if err != nil {
		res.Error = err.Error()
		if len(res.Error) > 200 {
			res.Error = res.Error[:200]
		}
	} else {
		res.RTTUs = rtt.Microseconds()
	}
	return res
}

// record stores a result and plans the next check, like the master does:
// a ping every interval; a TCP check every interval while up, 30 s apart
// after a failure. A status change is sent to the master right away.
func (p *prober) record(c proto.ProbeCheck, res proto.ProbeResult, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	pc := p.checks[c.ID]
	if pc == nil || pc.c != c {
		return // cleared or changed meanwhile: the result is of no use
	}
	pc.busy = false
	if res.Setup {
		pc.next = now.Add(time.Duration(c.Interval) * time.Second)
		p.results = append(p.results, presult{res, now})
		return
	}
	good := res.OK
	if c.Kind == "tcp" && c.Reverse {
		good = !res.OK
	}
	state := pc.state
	if good {
		pc.fails, state = 0, 1
	} else {
		pc.fails++
		limit := c.DownAfter
		if c.Kind != "ping" {
			limit = c.Retries + 1
		}
		if pc.fails >= max(limit, 1) {
			state = 2
		}
	}
	switch {
	case c.Kind == "ping" || good:
		pc.next = now.Add(time.Duration(c.Interval) * time.Second)
	default:
		pc.next = now.Add(probeRetryEvery)
	}
	changed := state != pc.state
	pc.state = state
	p.results = append(p.results, presult{res, now})
	if over := len(p.results) - probeKeep; over > 0 {
		p.results = p.results[over:]
	}
	if changed || len(p.results) >= probeSendAt {
		p.signal()
	}
}

// loop runs the checks until ctx ends.
func (p *prober) loop(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		for _, c := range p.due(time.Now()) {
			go func(c proto.ProbeCheck) {
				select {
				case p.sem <- struct{}{}:
				case <-ctx.Done():
					return
				}
				res := p.run(ctx, c)
				<-p.sem
				if ctx.Err() == nil {
					p.record(c, res, time.Now())
				}
			}(c)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// sendLoop sends results right away when a status changed or many wait.
func (a *Agent) probeSendLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-a.pr.urgent:
		}
		for a.pr.pending() > 0 && ctx.Err() == nil {
			rep, taken := a.pr.take(probeBatch, time.Now())
			if err := a.cl.postJSON(ctx, "/api/agent/probe", rep, nil, 30*time.Second); err != nil {
				a.pr.putBack(taken)
				log.Printf("probe results not sent: %v (they go with the next poll)", err)
				break
			}
		}
	}
}
