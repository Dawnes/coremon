package master

// Local devices: monitored by the master itself (for now with a ping), no
// agent. They live apart from the agent devices, so no agent logic (keys,
// jobs, updates, CVEs) ever touches them.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type LocalDevice struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	GroupID   string     `json:"group_id"`
	Order     int        `json:"order"`
	Address   string     `json:"address"`
	AddedAt   int64      `json:"added_at"`
	Checked   bool       `json:"checked,omitempty"` // up or down has been decided
	Up        bool       `json:"up,omitempty"`
	Since     int64      `json:"since,omitempty"` // when it went up or down
	LastCheck int64      `json:"last_check,omitempty"`
	LastUp    int64      `json:"last_up,omitempty"` // last answered ping
	RTTUs     int64      `json:"rtt_us,omitempty"`
	Fails     int        `json:"fails,omitempty"` // unanswered pings in a row
	LastError string     `json:"last_error,omitempty"`
	Hist      []histSlot `json:"hist,omitempty"`
}

// histSlot: the pings of one quarter of an hour, for the 24-hour bar.
type histSlot struct {
	T  int64 `json:"t"`
	OK int   `json:"ok"`
	N  int   `json:"n"`
}

const (
	histSlotSec = 900
	histSpan    = 24 * 3600
)

type LocalSettings struct {
	IntervalSec int `json:"interval_sec"`
	TimeoutSec  int `json:"timeout_sec"`
	DownAfter   int `json:"down_after"`
}

const (
	defaultPingInterval = 60
	minPingInterval     = 10
	maxPingInterval     = 3600
	defaultPingTimeout  = 2
	minPingTimeout      = 1
	maxPingTimeout      = 10
	defaultDownAfter    = 3
	minDownAfter        = 1
	maxDownAfter        = 10
	maxLocalDevices     = 1000
	pingsAtOnce         = 32
)

func (l *LocalSettings) normalize() {
	if l.IntervalSec == 0 {
		l.IntervalSec = defaultPingInterval
	}
	if l.TimeoutSec == 0 {
		l.TimeoutSec = defaultPingTimeout
	}
	if l.DownAfter == 0 {
		l.DownAfter = defaultDownAfter
	}
	l.IntervalSec = clamp(l.IntervalSec, minPingInterval, maxPingInterval)
	l.TimeoutSec = clamp(l.TimeoutSec, minPingTimeout, maxPingTimeout)
	l.DownAfter = clamp(l.DownAfter, minDownAfter, maxDownAfter)
}

// pingFunc sends one echo request and waits for the answer.
type pingFunc func(ctx context.Context, ip net.IP, timeout time.Duration) (time.Duration, error)

// pingSetupError: the master could not send a ping at all (no permission).
type pingSetupError struct{ err error }

func (e *pingSetupError) Error() string { return e.err.Error() }

type localState struct {
	mu      sync.Mutex
	next    map[string]time.Time
	busy    map[string]bool
	method  string // how pings are sent: "ping socket" or "raw socket"
	setup   string // why pings cannot be sent, if so
	setupAt int64
}

// ---- addresses ----

// parseDeviceIP accepts an IPv4 or IPv6 address only (no host names: DNS
// is not relied on) and returns its canonical form.
func parseDeviceIP(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("an IP address is required")
	}
	if strings.ContainsAny(s, "%/") {
		return "", errors.New("enter a plain IP address, without a prefix length or zone")
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return "", fmt.Errorf("%q is not an IP address (host names are not allowed)", clip(s, 64))
	}
	switch {
	case ip.IsUnspecified():
		return "", errors.New("this address cannot be pinged")
	case ip.IsLoopback():
		return "", errors.New("a loopback address points at the master itself")
	case ip.IsMulticast(), ip.Equal(net.IPv4bcast):
		return "", errors.New("multicast and broadcast addresses are not allowed")
	case ip.IsLinkLocalUnicast():
		return "", errors.New("link-local addresses are not allowed")
	}
	return ip.String(), nil
}

// canonIP: the comparable form of an address, or "" for addresses that
// every system has (loopback, link-local) and are never compared.
func canonIP(s string) string {
	if i := strings.IndexAny(s, "%/"); i >= 0 {
		s = s[:i]
	}
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return ""
	}
	return ip.String()
}

type ipConflict struct {
	IP   string `json:"ip"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ipOwner: the device (other than except) that already uses ip. Caller holds s.st.mu.
func (s *Server) ipOwner(ip, except string) (id, name string) {
	for _, a := range s.st.st.Agents {
		for _, x := range a.Facts.IPs {
			if a.ID != except && canonIP(x) == ip {
				return a.ID, a.Name
			}
		}
	}
	for _, d := range s.st.st.Locals {
		if d.ID != except && d.Address == ip {
			return d.ID, d.Name
		}
	}
	return "", ""
}

// agentConflicts: local devices using one of this agent device's addresses.
func (s *Server) agentConflicts(a *Agent) []ipConflict {
	var out []ipConflict
	for _, x := range a.Facts.IPs {
		ip := canonIP(x)
		if ip == "" {
			continue
		}
		for _, d := range s.st.st.Locals {
			if d.Address == ip {
				out = append(out, ipConflict{IP: ip, ID: d.ID, Name: d.Name})
			}
		}
	}
	return out
}

func (s *Server) localConflicts(d *LocalDevice) []ipConflict {
	var out []ipConflict
	for _, a := range s.st.st.Agents {
		for _, x := range a.Facts.IPs {
			if canonIP(x) == d.Address {
				out = append(out, ipConflict{IP: d.Address, ID: a.ID, Name: a.Name})
				break
			}
		}
	}
	return out
}

// ---- the website view ----

type localView struct {
	Address   string     `json:"address"`
	Checked   bool       `json:"checked"`
	Up        bool       `json:"up"`
	Since     int64      `json:"since"`
	LastCheck int64      `json:"last_check"`
	LastUp    int64      `json:"last_up"`
	RTTUs     int64      `json:"rtt_us"`
	Fails     int        `json:"fails"`
	LastError string     `json:"last_error,omitempty"`
	Uptime    float64    `json:"uptime"` // share of answered pings in the last 24 hours; -1 none
	Hist      []histSlot `json:"hist,omitempty"`
	Interval  int        `json:"interval"`
	DownAfter int        `json:"down_after"`
}

func (s *Server) localDeviceView(d *LocalDevice, full bool) agentView {
	now := time.Now().Unix()
	v := agentView{
		ID: d.ID, Name: d.Name, GroupID: d.GroupID, Order: d.Order, Status: StatusActive, Monitor: "local",
		Online: d.Checked && d.Up, LastSeen: d.LastUp, EnrolledAt: d.AddedAt, Features: []string{},
	}
	v.Facts.IPs = []string{d.Address}
	lv := &localView{Address: d.Address, Checked: d.Checked, Up: d.Up, Since: d.Since, LastCheck: d.LastCheck,
		LastUp: d.LastUp, RTTUs: d.RTTUs, Fails: d.Fails, LastError: d.LastError, Uptime: -1, Interval: s.st.st.Local.IntervalSec, DownAfter: s.st.st.Local.DownAfter}
	ok, n := 0, 0
	for _, h := range d.Hist {
		if h.T > now-histSpan {
			ok, n = ok+h.OK, n+h.N
		}
	}
	if n > 0 {
		lv.Uptime = float64(ok) / float64(n)
	}
	if full {
		lv.Hist = []histSlot{}
		for _, h := range d.Hist {
			if h.T > now-histSpan {
				lv.Hist = append(lv.Hist, h)
			}
		}
	}
	v.Local = lv
	v.Conflicts = s.localConflicts(d)
	return v
}

// sortViews: by group (in group order, ungrouped last), then position, then name.
func (s *Server) sortViews(out []agentView) {
	gpos := map[string]int{"": len(s.st.st.Groups)}
	for i, g := range s.st.st.Groups {
		gpos[g.ID] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if gpos[a.GroupID] != gpos[b.GroupID] {
			return gpos[a.GroupID] < gpos[b.GroupID]
		}
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}

// ---- results ----

// record stores one ping result. Caller holds s.st.mu.
func (s *Server) record(d *LocalDevice, rtt time.Duration, err error, now time.Time) {
	ls := s.st.st.Local
	t := now.Unix()
	d.LastCheck = t
	ok := err == nil
	if ok {
		d.RTTUs, d.LastUp, d.Fails, d.LastError = rtt.Microseconds(), t, 0, ""
		if !d.Checked || !d.Up {
			d.Checked, d.Up, d.Since = true, true, t
		}
	} else {
		d.Fails++
		d.LastError = clip(err.Error(), 200)
		if d.Fails >= ls.DownAfter && (!d.Checked || d.Up) {
			d.Checked, d.Up, d.Since = true, false, t
		}
	}
	slot := t - t%histSlotSec
	if n := len(d.Hist); n > 0 && d.Hist[n-1].T == slot {
		d.Hist[n-1].N++
		if ok {
			d.Hist[n-1].OK++
		}
	} else {
		h := histSlot{T: slot, N: 1}
		if ok {
			h.OK = 1
		}
		d.Hist = append(d.Hist, h)
	}
	cut := 0
	for cut < len(d.Hist) && d.Hist[cut].T <= t-histSpan {
		cut++
	}
	if cut > 0 {
		d.Hist = append([]histSlot(nil), d.Hist[cut:]...)
	}
	s.st.markDirty()
}

// pingOne pings a device and records the result. Without the lock held.
func (s *Server) pingOne(id string) {
	s.st.mu.Lock()
	d := s.st.st.Locals[id]
	if d == nil {
		s.st.mu.Unlock()
		return
	}
	addr, timeout := d.Address, time.Duration(s.st.st.Local.TimeoutSec)*time.Second
	s.st.mu.Unlock()
	ip := net.ParseIP(addr)
	ctx, cancel := context.WithTimeout(context.Background(), timeout+time.Second)
	rtt, err := s.pinger(ctx, ip, timeout)
	cancel()
	var se *pingSetupError
	s.loc.mu.Lock()
	if errors.As(err, &se) {
		if s.loc.setup == "" {
			log.Printf("agentless devices: cannot send pings: %v", err)
		}
		s.loc.setup, s.loc.setupAt = err.Error(), time.Now().Unix()
	} else {
		s.loc.setup = ""
		if m := pingMethod(); m != "" {
			s.loc.method = m
		}
	}
	s.loc.mu.Unlock()
	s.st.mu.Lock()
	if d := s.st.st.Locals[id]; d != nil && d.Address == addr {
		if se != nil {
			// The master's own problem, not the device's: no result.
			d.LastCheck, d.LastError = time.Now().Unix(), se.Error()
			s.st.markDirty()
		} else {
			s.record(d, rtt, err, time.Now())
		}
	}
	s.st.mu.Unlock()
}

// localLoop pings every local device once per interval.
func (s *Server) localLoop(stop <-chan struct{}) {
	sem := make(chan struct{}, pingsAtOnce)
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		now := time.Now()
		s.st.mu.Lock()
		interval := time.Duration(s.st.st.Local.IntervalSec) * time.Second
		var due []string
		s.loc.mu.Lock()
		for id := range s.st.st.Locals {
			next, seen := s.loc.next[id]
			if !seen {
				// First check soon, spread a little so not all start at once.
				next = now.Add(time.Duration(len(s.loc.next)%10) * time.Second)
				s.loc.next[id] = next
			}
			if !s.loc.busy[id] && !now.Before(next) {
				s.loc.busy[id] = true
				s.loc.next[id] = now.Add(interval)
				due = append(due, id)
			}
		}
		for id := range s.loc.next {
			if s.st.st.Locals[id] == nil {
				delete(s.loc.next, id)
			}
		}
		s.loc.mu.Unlock()
		s.st.mu.Unlock()
		for _, id := range due {
			go func(id string) {
				sem <- struct{}{}
				defer func() { <-sem }()
				s.pingOne(id)
				s.loc.mu.Lock()
				delete(s.loc.busy, id)
				s.loc.mu.Unlock()
			}(id)
		}
		select {
		case <-stop:
			return
		case <-t.C:
		}
	}
}

// ---- handlers ----

func (s *Server) localOr404(w http.ResponseWriter, r *http.Request) *LocalDevice {
	d := s.st.st.Locals[r.PathValue("id")]
	if d == nil {
		http.Error(w, "device not found", http.StatusNotFound)
	}
	return d
}

func (s *Server) writeLocalAudit(r *http.Request, action string, d *LocalDevice, detail string) {
	s.audit.add(auditEntry{Actor: actor(r), IP: clientIP(r), Action: action, Agent: d.ID, Target: d.Name, Detail: detail})
}

// checkAddress: a valid IP that no other device uses. Caller holds s.st.mu.
func (s *Server) checkAddress(addr, except string) (string, int, error) {
	ip, err := parseDeviceIP(addr)
	if err != nil {
		return "", http.StatusBadRequest, err
	}
	if _, name := s.ipOwner(ip, except); name != "" {
		return "", http.StatusConflict, fmt.Errorf("IP address %s is already used by %s: a device can only be added once, with one type of monitoring", ip, name)
	}
	return ip, 0, nil
}

func (s *Server) handleLocalCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		GroupID string `json:"group_id"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	name, err := checkName("device name", req.Name, maxSystemName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if name == "" {
		http.Error(w, "a name is required", http.StatusBadRequest)
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	if len(s.st.st.Locals) >= maxLocalDevices {
		http.Error(w, fmt.Sprintf("at most %d agentless devices", maxLocalDevices), http.StatusBadRequest)
		return
	}
	ip, code, err := s.checkAddress(req.Address, "")
	if err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	if req.GroupID != "" && s.st.group(req.GroupID) == nil {
		http.Error(w, "group not found", http.StatusBadRequest)
		return
	}
	d := &LocalDevice{ID: "l_" + randHex(8), Name: name, Address: ip, GroupID: req.GroupID, AddedAt: time.Now().Unix()}
	d.Order = s.st.nextOrder(d.GroupID)
	s.st.st.Locals[d.ID] = d
	s.st.markDirty()
	s.writeLocalAudit(r, "device.local_added", d, "address "+ip)
	writeJSON(w, s.localDeviceView(d, false))
}

func (s *Server) handleLocalUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    *string `json:"name"`
		Address *string `json:"address"`
		GroupID *string `json:"group_id"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	d := s.localOr404(w, r)
	if d == nil {
		return
	}
	var changes []string
	if req.Name != nil {
		n, err := checkName("device name", *req.Name, maxSystemName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if n != "" && n != d.Name {
			changes = append(changes, "name "+d.Name+" → "+n)
			d.Name = n
		}
	}
	if req.Address != nil {
		ip, code, err := s.checkAddress(*req.Address, d.ID)
		if err != nil {
			http.Error(w, err.Error(), code)
			return
		}
		if ip != d.Address {
			changes = append(changes, "address "+d.Address+" → "+ip)
			d.Address = ip
			// A new address starts over.
			d.Checked, d.Up, d.Since, d.LastCheck, d.LastUp, d.RTTUs, d.Fails, d.LastError, d.Hist = false, false, 0, 0, 0, 0, 0, "", nil
			s.loc.mu.Lock()
			delete(s.loc.next, d.ID)
			s.loc.mu.Unlock()
		}
	}
	if req.GroupID != nil && *req.GroupID != d.GroupID {
		if *req.GroupID != "" && s.st.group(*req.GroupID) == nil {
			http.Error(w, "group not found", http.StatusBadRequest)
			return
		}
		d.Order = s.st.nextOrder(*req.GroupID)
		d.GroupID = *req.GroupID
	}
	s.st.markDirty()
	if len(changes) > 0 {
		s.writeLocalAudit(r, "device.local_changed", d, strings.Join(changes, "; "))
	}
	writeJSON(w, s.localDeviceView(d, false))
}

func (s *Server) handleLocalDelete(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	d := s.localOr404(w, r)
	if d == nil {
		return
	}
	delete(s.st.st.Locals, d.ID)
	s.st.markDirty()
	s.writeLocalAudit(r, "device.local_removed", d, "address "+d.Address)
	writeJSON(w, map[string]bool{"removed": true})
}

func (s *Server) handleLocalPing(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	d := s.localOr404(w, r)
	if d == nil {
		s.st.mu.Unlock()
		return
	}
	id := d.ID
	s.st.mu.Unlock()
	s.loc.mu.Lock()
	if s.loc.busy[id] {
		s.loc.mu.Unlock()
		http.Error(w, "a ping to this device is already running", http.StatusConflict)
		return
	}
	s.loc.busy[id] = true
	s.loc.mu.Unlock()
	s.pingOne(id)
	s.loc.mu.Lock()
	delete(s.loc.busy, id)
	s.loc.mu.Unlock()
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	d = s.st.st.Locals[id]
	if d == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	writeJSON(w, s.localDeviceView(d, true))
}

func (s *Server) localStatus() map[string]any {
	s.loc.mu.Lock()
	defer s.loc.mu.Unlock()
	return map[string]any{"method": s.loc.method, "error": s.loc.setup, "error_at": s.loc.setupAt}
}

func (s *Server) handleLocalSettingsGet(w http.ResponseWriter, r *http.Request) {
	st := s.localStatus()
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	writeJSON(w, map[string]any{"settings": s.st.st.Local, "devices": len(s.st.st.Locals), "ping": st,
		"limits": map[string]int{"interval_min": minPingInterval, "interval_max": maxPingInterval,
			"timeout_min": minPingTimeout, "timeout_max": maxPingTimeout, "down_min": minDownAfter, "down_max": maxDownAfter}})
}

func (s *Server) handleLocalSettingsSave(w http.ResponseWriter, r *http.Request) {
	var req LocalSettings
	if !readJSON(w, r, &req, 4096) {
		return
	}
	bad := func(msg string) { http.Error(w, msg, http.StatusBadRequest) }
	switch {
	case req.IntervalSec < minPingInterval || req.IntervalSec > maxPingInterval:
		bad(fmt.Sprintf("ping interval must be %d-%d seconds", minPingInterval, maxPingInterval))
		return
	case req.TimeoutSec < minPingTimeout || req.TimeoutSec > maxPingTimeout:
		bad(fmt.Sprintf("timeout must be %d-%d seconds", minPingTimeout, maxPingTimeout))
		return
	case req.DownAfter < minDownAfter || req.DownAfter > maxDownAfter:
		bad(fmt.Sprintf("failed pings before down must be %d-%d", minDownAfter, maxDownAfter))
		return
	}
	s.st.mu.Lock()
	old := s.st.st.Local
	s.st.st.Local = req
	s.st.markDirty()
	s.st.mu.Unlock()
	if old.IntervalSec != req.IntervalSec {
		// Apply the new interval right away instead of after the old one.
		s.loc.mu.Lock()
		now := time.Now()
		for id, next := range s.loc.next {
			if lim := now.Add(time.Duration(req.IntervalSec) * time.Second); next.After(lim) {
				s.loc.next[id] = lim
			}
		}
		s.loc.mu.Unlock()
	}
	s.audit.write(actor(r), clientIP(r), "settings.local", "", fmt.Sprintf("ping every %ds, timeout %ds, down after %d failed", req.IntervalSec, req.TimeoutSec, req.DownAfter))
	writeJSON(w, map[string]any{"settings": req})
}
