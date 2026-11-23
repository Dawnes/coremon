package master

// Settings → Upstream: the internet connection as a special virtual device.
// When it is switched on, the master pings a few public addresses every
// minute: green when all answer, amber when only some do, red when none do.
// It is the top of the network map and has an Overview tile. It is never
// the cause of "unreachable" for the devices below it.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	upstreamAddrs   = 3
	upstreamTimeout = 3 * time.Second
	defaultUpstream = "Upstream"
	// Ping interval in seconds: default, lowest and highest.
	defaultUpInterval = 60
	minUpInterval     = 30
	maxUpInterval     = 3600
)

// Quad9 (Switzerland), DNS4EU unfiltered (EU), Cloudflare.
var defaultUpstreamAddrs = []string{"9.9.9.9", "86.54.11.100", "1.1.1.1"}

type UpstreamSettings struct {
	Enabled bool     `json:"enabled,omitempty"`
	Name    string   `json:"name,omitempty"`
	Addrs   []string `json:"addrs,omitempty"`
	// IntervalSec: seconds between ping rounds (minUpInterval–maxUpInterval).
	IntervalSec int `json:"interval_sec,omitempty"`
	// HideTile: the Overview tile is not shown (zero value: shown).
	HideTile bool `json:"hide_tile,omitempty"`
}

func (u *UpstreamSettings) normalize() {
	if u.Name == "" {
		u.Name = defaultUpstream
	}
	if u.IntervalSec == 0 {
		u.IntervalSec = defaultUpInterval
	}
	u.IntervalSec = clamp(u.IntervalSec, minUpInterval, maxUpInterval)
	if len(u.Addrs) != upstreamAddrs {
		a := make([]string, upstreamAddrs)
		copy(a, u.Addrs)
		u.Addrs = a
	}
	if u.Addrs[0] == "" && u.Addrs[1] == "" && u.Addrs[2] == "" {
		copy(u.Addrs, defaultUpstreamAddrs)
	}
}

// upAddr: the last ping result of one address.
type upAddr struct {
	Addr  string `json:"addr"`
	OK    bool   `json:"ok"`
	RTTus int64  `json:"rtt_us,omitempty"`
	Err   string `json:"error,omitempty"`
	At    int64  `json:"at,omitempty"`
}

// upstreamState: the results, in memory only.
type upstreamState struct {
	mu    sync.Mutex
	addrs []upAddr
	state string    // wait, ok, partial, down
	since int64     // when the state last changed
	last  time.Time // start of the last ping round
	busy  bool      // a round is running
	kick  chan struct{}
}

// upstreamView: Upstream as the website sees it.
type upstreamView struct {
	Enabled  bool     `json:"enabled"`
	ID       string   `json:"id,omitempty"`
	Name     string   `json:"name"`
	Addrs    []upAddr `json:"addrs"`
	State    string   `json:"state"` // off, wait, ok, partial, down
	Since    int64    `json:"since,omitempty"`
	Answer   int      `json:"answering"`
	Total    int      `json:"total"`
	RTTus    int64    `json:"rtt_us,omitempty"` // fastest answer
	ShowTile bool     `json:"show_tile"`
	Via      *devRef  `json:"via,omitempty"`
	Interval int      `json:"interval_sec"`
}

func upstreamStateOf(addrs []upAddr) string {
	n, ok, wait := 0, 0, 0
	for _, a := range addrs {
		if a.Addr == "" {
			continue
		}
		n++
		if a.At == 0 {
			wait++
		} else if a.OK {
			ok++
		}
	}
	switch {
	case n == 0 || wait == n:
		return "wait"
	case ok == n-wait && wait == 0:
		return "ok"
	case ok > 0:
		return "partial"
	case wait > 0:
		return "wait"
	}
	return "down"
}

// upstreamView: caller holds s.st.mu.
func (s *Server) upstreamView() upstreamView {
	u := s.st.st.Upstream
	v := upstreamView{Enabled: u.Enabled, Name: u.Name, ShowTile: !u.HideTile, State: "off", Addrs: []upAddr{}, Interval: u.IntervalSec}
	if !u.Enabled {
		return v
	}
	v.ID = upstreamID
	if d := s.st.st.Virtuals[upstreamID]; d != nil && d.Via != "" {
		v.Via = s.devRefOf(d.Via)
	}
	s.up.mu.Lock()
	defer s.up.mu.Unlock()
	res := map[string]upAddr{}
	for _, a := range s.up.addrs {
		res[a.Addr] = a
	}
	for _, addr := range u.Addrs {
		if addr == "" {
			continue
		}
		a, ok := res[addr]
		if !ok {
			a = upAddr{Addr: addr}
		}
		v.Addrs = append(v.Addrs, a)
		v.Total++
		if a.OK {
			v.Answer++
			if v.RTTus == 0 || a.RTTus < v.RTTus {
				v.RTTus = a.RTTus
			}
		}
	}
	v.State = upstreamStateOf(v.Addrs)
	if v.State == s.up.state {
		v.Since = s.up.since
	}
	return v
}

func (s *Server) handleUpstreamGet(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	v := s.upstreamView()
	addrs := append([]string(nil), s.st.st.Upstream.Addrs...)
	s.st.mu.Unlock()
	writeJSON(w, map[string]any{"upstream": v, "addrs": addrs, "defaults": defaultUpstreamAddrs, "default_name": defaultUpstream,
		"interval": map[string]int{"default": defaultUpInterval, "min": minUpInterval, "max": maxUpInterval}})
}

func (s *Server) handleUpstreamSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled  bool     `json:"enabled"`
		Name     string   `json:"name"`
		Addrs    []string `json:"addrs"`
		ShowTile bool     `json:"show_tile"`
		Interval int      `json:"interval_sec"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	if req.Interval == 0 {
		req.Interval = defaultUpInterval
	}
	if req.Interval < minUpInterval || req.Interval > maxUpInterval {
		http.Error(w, fmt.Sprintf("ping every %d to %d seconds", minUpInterval, maxUpInterval), http.StatusBadRequest)
		return
	}
	name, err := checkName("name", req.Name, maxSystemName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if name == "" {
		name = defaultUpstream
	}
	if len(req.Addrs) > upstreamAddrs {
		http.Error(w, fmt.Sprintf("at most %d addresses", upstreamAddrs), http.StatusBadRequest)
		return
	}
	addrs := make([]string, upstreamAddrs)
	n := 0
	for i, a := range req.Addrs {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		ip, err := parseDeviceIP(a)
		if err != nil {
			http.Error(w, fmt.Sprintf("address %d: %v", i+1, err), http.StatusBadRequest)
			return
		}
		addrs[i] = ip
		n++
	}
	if req.Enabled && n == 0 {
		http.Error(w, "enter at least one address to ping", http.StatusBadRequest)
		return
	}
	if n == 0 {
		copy(addrs, defaultUpstreamAddrs)
	}

	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	old := s.st.st.Upstream
	nu := UpstreamSettings{Enabled: req.Enabled, Name: name, Addrs: addrs, HideTile: !req.ShowTile, IntervalSec: req.Interval}
	var changes []string
	if nu.Enabled != old.Enabled {
		changes = append(changes, map[bool]string{true: "switched on", false: "switched off"}[nu.Enabled])
	}
	if nu.Name != old.Name {
		changes = append(changes, "name "+old.Name+" → "+nu.Name)
	}
	if strings.Join(nu.Addrs, ",") != strings.Join(old.Addrs, ",") {
		changes = append(changes, "addresses "+strings.Join(nonEmpty(nu.Addrs), ", "))
	}
	if nu.IntervalSec != old.IntervalSec {
		changes = append(changes, fmt.Sprintf("ping every %d s → %d s", old.IntervalSec, nu.IntervalSec))
	}
	if nu.HideTile != old.HideTile {
		changes = append(changes, map[bool]string{true: "Overview tile hidden", false: "Overview tile shown"}[nu.HideTile])
	}
	s.st.st.Upstream = nu
	d := s.st.st.Virtuals[upstreamID]
	switch {
	case nu.Enabled && d == nil:
		s.st.st.Virtuals[upstreamID] = &VirtualDevice{ID: upstreamID, Name: nu.Name, AddedAt: time.Now().Unix(), Special: specialUpstream}
	case nu.Enabled:
		d.Name = nu.Name
	case d != nil:
		delete(s.st.st.Virtuals, upstreamID)
		s.dropVia(upstreamID)
	}
	s.st.markDirty()
	if len(changes) > 0 {
		s.audit.write(actor(r), clientIP(r), "settings.upstream", "", strings.Join(changes, "; "))
	}
	if strings.Join(nu.Addrs, ",") != strings.Join(old.Addrs, ",") || nu.Enabled != old.Enabled {
		s.up.mu.Lock()
		s.up.addrs, s.up.state, s.up.since, s.up.last = nil, "", 0, time.Time{}
		s.up.mu.Unlock()
	}
	if nu.Enabled {
		select {
		case s.up.kick <- struct{}{}:
		default:
		}
	}
	writeJSON(w, map[string]any{"upstream": s.upstreamView()})
}

func nonEmpty(xs []string) []string {
	var out []string
	for _, x := range xs {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

// handleUpstreamVia: "Connected via" of the Upstream device itself (the
// map places it above the master; usually empty).
func (s *Server) handleUpstreamVia(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Via string `json:"via"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	d := s.st.st.Virtuals[upstreamID]
	if d == nil {
		http.Error(w, "Upstream is not switched on", http.StatusConflict)
		return
	}
	via, err := s.checkVia(d.ID, req.Via)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if via != d.Via {
		s.audit.write(actor(r), clientIP(r), "settings.upstream", "", "connected via "+s.viaName(d.Via)+" → "+s.viaName(via))
		d.Via = via
		s.st.markDirty()
	}
	writeJSON(w, map[string]any{"upstream": s.upstreamView()})
}

// upstreamNext: when the next ping round starts (zero: Upstream is off).
// A changed interval counts from the last round. Caller holds s.st.mu.
func (s *Server) upstreamNext(now time.Time) time.Time {
	u := s.st.st.Upstream
	if !u.Enabled || len(nonEmpty(u.Addrs)) == 0 {
		return time.Time{}
	}
	s.up.mu.Lock()
	defer s.up.mu.Unlock()
	if s.up.last.IsZero() {
		return now
	}
	return s.up.last.Add(time.Duration(u.IntervalSec) * time.Second)
}

// upstreamLoop pings the Upstream addresses while it is on, every
// IntervalSec seconds; a new address list or switching it on pings at once.
func (s *Server) upstreamLoop(stop <-chan struct{}) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	start := time.Now().Add(5 * time.Second) // first round shortly after start
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		case <-s.up.kick:
			start = time.Time{}
		}
		now := time.Now()
		if now.Before(start) {
			continue
		}
		s.st.mu.Lock()
		next := s.upstreamNext(now)
		s.st.mu.Unlock()
		if next.IsZero() || now.Before(next) {
			continue
		}
		s.upstreamRound()
	}
}

func (s *Server) upstreamRound() {
	s.st.mu.Lock()
	u := s.st.st.Upstream
	addrs := nonEmpty(u.Addrs)
	s.st.mu.Unlock()
	if !u.Enabled || len(addrs) == 0 {
		return
	}
	s.up.mu.Lock()
	s.up.last, s.up.busy = time.Now(), true
	s.up.mu.Unlock()
	res := make([]upAddr, len(addrs))
	var wg sync.WaitGroup
	for i, a := range addrs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res[i] = s.pingUpstream(a)
		}()
	}
	wg.Wait()
	st := upstreamStateOf(res)
	s.up.mu.Lock()
	s.up.addrs, s.up.busy = res, false
	if st != s.up.state {
		s.up.state, s.up.since = st, time.Now().Unix()
	}
	s.up.mu.Unlock()
}

func (s *Server) pingUpstream(addr string) upAddr {
	out := upAddr{Addr: addr, At: time.Now().Unix()}
	ip := net.ParseIP(addr)
	if ip == nil {
		out.Err = "not an IP address"
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), upstreamTimeout+time.Second)
	defer cancel()
	rtt, err := s.pinger(ctx, ip, upstreamTimeout)
	if err != nil {
		out.Err = shortErr(err)
		return out
	}
	out.OK, out.RTTus = true, max(rtt.Microseconds(), 1)
	return out
}

func shortErr(err error) string {
	m := err.Error()
	if len(m) > 160 {
		m = m[:160] + "…"
	}
	return m
}
