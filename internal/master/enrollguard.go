package master

// Limits on registration attempts per address.

import (
	"sync"
	"time"
)

const (
	enrollFails   = 5
	enrollWindow  = 15 * time.Minute
	enrollTracked = 10000
)

type enrollWin struct {
	start time.Time
	n     int
}

type enrollGuard struct {
	mu      sync.Mutex
	fails   map[string]*enrollWin
	waiting map[string]bool
}

func (g *enrollGuard) rejected(ip string, now time.Time) (first, blocked bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.fails == nil {
		g.fails = map[string]*enrollWin{}
	}
	w := g.fails[ip]
	if w != nil && now.Sub(w.start) >= enrollWindow {
		w = nil
	}
	if w == nil {
		if len(g.fails) >= enrollTracked {
			return false, true
		}
		g.fails[ip] = &enrollWin{start: now, n: 1}
		return true, false
	}
	w.n++
	return false, w.n > enrollFails
}

func (g *enrollGuard) over(ip string, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	w := g.fails[ip]
	return w != nil && now.Sub(w.start) < enrollWindow && w.n >= enrollFails
}

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
