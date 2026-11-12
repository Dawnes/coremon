package master

// Registration attempts per address. Anyone can send a correctly sealed
// "register" request (the master's public key is in the install script), so
// attempts with a wrong token are limited like failed logins and summarised
// in the audit log instead of logged one by one. With a valid token, an
// address can have one system waiting for approval at a time.

import (
	"sync"
	"time"
)

const (
	enrollFails   = 5 // attempts with a wrong token per address ...
	enrollWindow  = 15 * time.Minute
	enrollTracked = 10000 // addresses remembered at most
)

type enrollWin struct {
	start time.Time
	n     int // attempts in this window (the first one is logged on its own)
}

type enrollGuard struct {
	mu      sync.Mutex
	fails   map[string]*enrollWin
	waiting map[string]bool // addresses already told (and logged) to wait
}

// rejected records an attempt with a wrong token. first: log it on its own;
// blocked: the address is over its limit (answer "too many").
func (g *enrollGuard) rejected(ip string, now time.Time) (first, blocked bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.fails == nil {
		g.fails = map[string]*enrollWin{}
	}
	w := g.fails[ip]
	if w != nil && now.Sub(w.start) >= enrollWindow {
		w = nil // an expired window is summarised by flush; start anew
	}
	if w == nil {
		if len(g.fails) >= enrollTracked {
			return false, true // under attack from many addresses: no bookkeeping, no log
		}
		g.fails[ip] = &enrollWin{start: now, n: 1}
		return true, false
	}
	w.n++
	return false, w.n > enrollFails
}

// over reports whether the address is over its limit right now (checked
// before the token is looked at).
func (g *enrollGuard) over(ip string, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	w := g.fails[ip]
	return w != nil && now.Sub(w.start) < enrollWindow && w.n >= enrollFails
}

// flush returns, per address, how many further attempts followed the logged
// first one in windows that have ended, and forgets those windows.
func (g *enrollGuard) flush(now time.Time) map[string]int {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := map[string]int{}
	for ip, w := range g.fails {
		if now.Sub(w.start) >= enrollWindow {
			if w.n > 1 {
				out[ip] = w.n - 1
			}
			delete(g.fails, ip)
		}
	}
	return out
}

// noteWaiting reports whether this is the first refusal for the address
// since it had a system waiting (so it is logged once, not per retry).
func (g *enrollGuard) noteWaiting(ip string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.waiting == nil {
		g.waiting = map[string]bool{}
	}
	if g.waiting[ip] || len(g.waiting) >= enrollTracked {
		return false
	}
	g.waiting[ip] = true
	return true
}

func (g *enrollGuard) clearWaiting(ip string) {
	g.mu.Lock()
	delete(g.waiting, ip)
	g.mu.Unlock()
}
