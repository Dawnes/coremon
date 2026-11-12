package master

// Checks on what an agent sends besides its facts: command results, job
// output, and how often and how many connections. A taken-over system can
// lie about itself, but must not be able to harm the master or others.

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"coremon/internal/feature/apps"
	"coremon/internal/feature/updates"
)

const (
	maxStateBytes   = 2 << 20 // a stored function state (e.g. the update list)
	maxStatePkgs    = 5000
	maxResultErrLen = 2000
)

// checkState validates a function's state before the master stores it. The
// update list is checked field by field; other states only by size.
func checkState(feature string, raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) > maxStateBytes {
		return nil, fmt.Errorf("result too large (%d bytes, at most %d)", len(raw), maxStateBytes)
	}
	if feature == "apps" {
		return checkApps(raw)
	}
	if feature != "updates" {
		return raw, nil
	}
	var s updates.Snapshot
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, errors.New("result is not a valid update list")
	}
	if len(s.Packages) > maxStatePkgs || len(s.Removals) > maxStatePkgs || len(s.RebootPkgs) > maxStatePkgs {
		return nil, fmt.Errorf("result lists more than %d packages", maxStatePkgs)
	}
	if s.Backend != "apt" {
		s.Backend = ""
	}
	s.CheckedAt, s.RefreshedAt = notFuture(s.CheckedAt), notFuture(s.RefreshedAt)
	sec := 0
	for i := range s.Packages {
		p := &s.Packages[i]
		if !updates.PackageNameRe.MatchString(p.Name) {
			return nil, fmt.Errorf("result has an invalid package name %q", cleanText(p.Name, 64))
		}
		p.Arch = cleanText(p.Arch, 16)
		p.Current = cleanText(p.Current, 128)
		p.Available = cleanText(p.Available, 128)
		p.Origin = cleanText(p.Origin, 128)
		if p.Security && !p.New {
			sec++
		}
	}
	for _, list := range [][]string{s.Removals, s.RebootPkgs} {
		for _, n := range list {
			if !updates.PackageNameRe.MatchString(n) {
				return nil, fmt.Errorf("result has an invalid package name %q", cleanText(n, 64))
			}
		}
	}
	// The counts must agree with the list.
	if s.Total < 0 || s.Total > len(s.Packages) {
		s.Total = len(s.Packages)
	}
	if s.Security < 0 || s.Security > s.Total {
		s.Security = min(sec, s.Total)
	}
	out, err := json.Marshal(s)
	if err != nil || len(out) > maxStateBytes {
		return nil, errors.New("result too large")
	}
	return out, nil
}

// extraStates may come along with another function's result (the
// application scan with every update check, install and upgrade).
var extraStates = map[string]bool{"apps": true}

const maxApps = 64

// checkApps validates an application scan: a short list of rows with plain
// names and cleaned versions.
func checkApps(raw json.RawMessage) (json.RawMessage, error) {
	var s apps.State
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, errors.New("result is not a valid application list")
	}
	if len(s.Apps) > maxApps {
		return nil, fmt.Errorf("result lists more than %d applications", maxApps)
	}
	s.ScannedAt = notFuture(s.ScannedAt)
	for i := range s.Apps {
		p := &s.Apps[i]
		p.Name = cleanText(p.Name, 40)
		p.Version, p.Full = cleanText(p.Version, 64), cleanText(p.Full, 128)
		if p.Package != "" && !updates.PackageNameRe.MatchString(p.Package) {
			p.Package = ""
		}
		if !p.Installed {
			p.Version, p.Full, p.Package = "", "", ""
		}
	}
	return json.Marshal(s)
}

// rateLimiter allows each agent about rate requests per second with a
// burst; a normal agent sends a few per minute.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64
	burst   float64
}

type bucket struct {
	tokens float64
	last   time.Time
}

const (
	agentRate  = 10 // requests per second, sustained
	agentBurst = 50
)

func (l *rateLimiter) allow(id string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.buckets == nil {
		l.buckets = map[string]*bucket{}
		if l.rate == 0 {
			l.rate, l.burst = agentRate, agentBurst
		}
	}
	b := l.buckets[id]
	if b == nil {
		if len(l.buckets) > 10000 { // forget idle agents
			for k, v := range l.buckets {
				if now.Sub(v.last) > time.Minute {
					delete(l.buckets, k)
				}
			}
		}
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[id] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
