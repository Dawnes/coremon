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

	"coremon/internal/probe"
)

type LocalDevice struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	GroupID string `json:"group_id"`
	Order   int    `json:"order"`
	AddedAt int64  `json:"added_at"`
	// The address, ping, services and "Checked by" (same JSON fields as before).
	Checks
	// Maint: in maintenance; nothing is checked.
	Maint *Maintenance `json:"maint,omitempty"`
	// Via: the device it is connected via ("Connected via"); "" = none.
	Via string `json:"via,omitempty"`
}

const (
	pingPaused  = "paused"
	pingRemoved = "removed"
)

// rttSlot: the round-trip times of the pings in five minutes, for the graph.
// Times in microseconds; F counts the unanswered pings.
type rttSlot struct {
	T   int64 `json:"t"`
	N   int   `json:"n,omitempty"`
	Sum int64 `json:"sum,omitempty"`
	Min int64 `json:"min,omitempty"`
	Max int64 `json:"max,omitempty"`
	F   int   `json:"f,omitempty"`
}

// rttPoint: one five-minute slot as the website shows it.
type rttPoint struct {
	T    int64 `json:"t"`
	Avg  int64 `json:"avg"`
	Min  int64 `json:"min"`
	Max  int64 `json:"max"`
	N    int   `json:"n"`
	Fail int   `json:"fail"`
}

const rttSlotSec = 300

// addRTT counts one ping in its five minutes and drops what is older than 24 hours.
func addRTT(rs []rttSlot, ok bool, us int64, t int64) []rttSlot {
	slot := t - t%rttSlotSec
	if n := len(rs); n == 0 || rs[n-1].T != slot {
		rs = append(rs, rttSlot{T: slot})
	}
	r := &rs[len(rs)-1]
	if ok {
		if r.N == 0 || us < r.Min {
			r.Min = us
		}
		if us > r.Max {
			r.Max = us
		}
		r.N++
		r.Sum += us
	} else {
		r.F++
	}
	cut := 0
	for cut < len(rs) && rs[cut].T <= t-histSpan {
		cut++
	}
	if cut > 0 {
		rs = append([]rttSlot(nil), rs[cut:]...)
	}
	return rs
}

// recentRTT: the slots of the last 24 hours with their average.
func recentRTT(rs []rttSlot, now int64) []rttPoint {
	out := []rttPoint{}
	for _, r := range rs {
		if r.T <= now-histSpan {
			continue
		}
		p := rttPoint{T: r.T, Min: r.Min, Max: r.Max, N: r.N, Fail: r.F}
		if r.N > 0 {
			p.Avg = r.Sum / int64(r.N)
		}
		out = append(out, p)
	}
	return out
}

// histSlot: the pings of one quarter of an hour, for the 24-hour bar.
type histSlot struct {
	T  int64  `json:"t"`
	OK int    `json:"ok"`
	N  int    `json:"n"`
	F  string `json:"f,omitempty"` // services: the sub-probe of the slot's latest failure
	U  int    `json:"u,omitempty"` // failures while a device above it was down or in maintenance (unreachable)
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

// icmpPing: the master's own ping, with a hint for the admin when the
// system does not allow it.
func icmpPing(ctx context.Context, ip net.IP, timeout time.Duration) (time.Duration, error) {
	rtt, err := probe.Ping(ctx, ip, timeout)
	var se *probe.SetupError
	if errors.As(err, &se) && se.Denied {
		return 0, &probe.SetupError{Denied: true, Err: errors.New("the master may not send pings: the system refuses ping sockets for its user " +
			"(sysctl net.ipv4.ping_group_range) and the service lacks CAP_NET_RAW. Run the master install command once as root on the master: it updates the service")}
	}
	return rtt, err
}

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

// ipOwner: the device (other than except) that already uses ip in the
// network seen by probe ("" = the master). The same address may be used
// by devices that different probes check: separate networks. Agent
// devices count in the master's network. Caller holds s.st.mu.
func (s *Server) ipOwner(ip, except, probe string) (id, name string) {
	if probe == "" {
		for _, a := range s.st.st.Agents {
			for _, x := range a.Facts.IPs {
				if a.ID != except && canonIP(x) == ip {
					return a.ID, a.Name
				}
			}
		}
	}
	for _, d := range s.st.st.Locals {
		if d.ID != except && d.Address == ip && d.Probe == probe {
			return d.ID, d.Name
		}
	}
	// The address an agent device's services are checked on.
	for _, a := range s.st.st.Agents {
		if a.ID != except && agentHasChecks(a) && a.Checks.Address == ip && a.Checks.Probe == probe {
			return a.ID, a.Name
		}
	}
	return "", ""
}

// agentConflicts: local devices checked by the master using one of this
// agent device's addresses.
func (s *Server) agentConflicts(a *Agent) []ipConflict {
	var out []ipConflict
	for _, x := range a.Facts.IPs {
		ip := canonIP(x)
		if ip == "" {
			continue
		}
		for _, d := range s.st.st.Locals {
			if d.Address == ip && d.Probe == "" {
				out = append(out, ipConflict{IP: ip, ID: d.ID, Name: d.Name})
			}
		}
	}
	return out
}

func (s *Server) localConflicts(d *LocalDevice) []ipConflict {
	var out []ipConflict
	if d.Probe != "" {
		return out
	}
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

	// Ping not running: the status above comes from the services; the ping fields are empty.
	Ping      bool          `json:"ping"`      // the ping runs
	PingMode  string        `json:"ping_mode"` // on, paused or removed
	PingInfo  *pingView     `json:"ping_info,omitempty"`
	SvcTotal  int           `json:"svc_total"` // services that run (not paused)
	SvcDown   int           `json:"svc_down"`
	SvcOK     int           `json:"svc_ok"` // running services that are up without failed retries
	SvcPaused int           `json:"svc_paused"`
	SvcFail   *svcFailView  `json:"svc_fail,omitempty"` // the first running service that is down: what failed
	Services  []serviceView `json:"services,omitempty"` // device page only

	Probe *localProbeView `json:"probe,omitempty"` // checked by an agent instead of the master
}

// svcFailView: a service that is down, the sub-probe that failed and why.
type svcFailView struct {
	Name   string `json:"name"`
	Step   string `json:"step,omitempty"`
	Method string `json:"method"`
	Error  string `json:"error,omitempty"`
}

// pingView: the ping as one of the device's checks, also while paused.
type pingView struct {
	Mode      string     `json:"mode"`
	Checked   bool       `json:"checked"`
	Up        bool       `json:"up"`
	Since     int64      `json:"since"`
	LastCheck int64      `json:"last_check"`
	LastUp    int64      `json:"last_up"`
	RTTUs     int64      `json:"rtt_us"`
	Fails     int        `json:"fails"`
	LastError string     `json:"last_error,omitempty"`
	Uptime    float64    `json:"uptime"`
	Hist      []histSlot `json:"hist,omitempty"`
	RTT       []rttPoint `json:"rtt,omitempty"`
	Interval  int        `json:"interval"`
	Timeout   int        `json:"timeout"`
	DownAfter int        `json:"down_after"`
}

func (s *Server) localDeviceView(d *LocalDevice, full bool) agentView {
	lv := s.checksView(&d.Checks, full, false)
	v := agentView{
		ID: d.ID, Name: d.Name, GroupID: d.GroupID, Order: d.Order, Status: StatusActive, Monitor: "local",
		Online: lv.Checked && lv.Up, LastSeen: lv.LastUp, EnrolledAt: d.AddedAt, Features: []string{}, Maint: d.Maint,
	}
	v.Facts.IPs = []string{d.Address}
	v.Local = lv
	v.Conflicts = s.localConflicts(d)
	s.setVia(&v, d.Via, d.Maint == nil && lv.Checked && !lv.Up, full)
	return v
}

// checksView: the ping and services of a device as the website shows them.
// noReadings: the device's agent is offline: disk, memory and load are
// unknown (not down), as no readings come in.
func (s *Server) checksView(d *Checks, full, noReadings bool) *localView {
	now := time.Now().Unix()
	mode := d.PingMode
	if mode == "" {
		mode = "on"
	}
	lv := &localView{Address: d.Address, Interval: s.st.st.Local.IntervalSec, DownAfter: s.st.st.Local.DownAfter, Ping: d.pinging(), PingMode: mode, Uptime: -1}
	if d.PingMode != pingRemoved {
		h, up := recentHist(d.Hist, now)
		pv := &pingView{Mode: mode, Checked: d.Checked, Up: d.Up, Since: d.Since, LastCheck: d.LastCheck, LastUp: d.LastUp, RTTUs: d.RTTUs,
			Fails: d.Fails, LastError: d.LastError, Uptime: up, Interval: s.st.st.Local.IntervalSec, Timeout: s.st.st.Local.TimeoutSec, DownAfter: s.st.st.Local.DownAfter}
		if full {
			pv.Hist = h
			pv.RTT = recentRTT(d.RTT, now)
		}
		lv.PingInfo = pv
		if d.pinging() {
			lv.Checked, lv.Up, lv.Since, lv.LastCheck, lv.LastUp, lv.RTTUs, lv.Fails, lv.LastError, lv.Uptime =
				pv.Checked, pv.Up, pv.Since, pv.LastCheck, pv.LastUp, pv.RTTUs, pv.Fails, pv.LastError, pv.Uptime
			if full {
				lv.Hist = h
			}
		}
	}
	svcs := []serviceView{}
	running := []serviceView{}
	for _, sv := range d.Services {
		v := s.serviceView(sv, now, full)
		if noReadings && sv.isMetric() {
			v.Checked, v.Stale = false, true
		}
		svcs = append(svcs, v)
		if sv.Paused {
			lv.SvcPaused++
			continue
		}
		running = append(running, v)
		switch {
		case v.Checked && !v.Up:
			lv.SvcDown++
			if lv.SvcFail == nil {
				lv.SvcFail = &svcFailView{Name: v.Name, Step: v.Failed, Method: v.Method, Error: v.LastError}
			}
		case v.Checked && v.Fails == 0:
			lv.SvcOK++
		}
	}
	lv.SvcTotal = len(running)
	if !d.pinging() {
		servicesStatus(lv, running)
	}
	if lv.Probe = s.localProbe(d, now); lv.Probe != nil && lv.Probe.State != "ok" {
		// No results come in: unknown, not down (no takeover by the master).
		lv.Checked, lv.Up, lv.SvcOK, lv.SvcDown, lv.SvcFail = false, false, 0, 0, nil
	}
	if full {
		lv.Services = svcs
	}
	return lv
}

// servicesStatus: with ping off, the device is up while at least one
// service is up and down when all are down. Reversed services count only
// when there are no others.
func servicesStatus(lv *localView, svcs []serviceView) {
	use := []serviceView{}
	for _, sv := range svcs {
		if !sv.Reverse {
			use = append(use, sv)
		}
	}
	if len(use) == 0 {
		use = svcs
	}
	up, down := 0, 0
	var upSince, downSince int64
	sum, n := 0.0, 0
	for _, sv := range use {
		if sv.LastCheck > lv.LastCheck {
			lv.LastCheck = sv.LastCheck
		}
		if sv.LastOK > lv.LastUp {
			lv.LastUp = sv.LastOK
		}
		if sv.Uptime >= 0 {
			sum, n = sum+sv.Uptime, n+1
		}
		switch {
		case sv.Checked && sv.Up:
			up++
			if upSince == 0 || sv.Since < upSince {
				upSince = sv.Since
			}
		case sv.Checked:
			down++
			if sv.Since > downSince {
				downSince = sv.Since
			}
		}
	}
	if n > 0 {
		lv.Uptime = sum / float64(n)
	}
	switch {
	case up > 0:
		lv.Checked, lv.Up, lv.Since = true, true, upSince
	case down > 0 && down == len(use):
		lv.Checked, lv.Up, lv.Since = true, false, downSince
	}
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
func (s *Server) record(d *Checks, rtt time.Duration, err error, now time.Time) {
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
	d.Hist = addHist(d.Hist, ok, "", t)
	d.RTT = addRTT(d.RTT, ok, rtt.Microseconds(), t)
	s.markChecks()
}

// addHist counts one result in its quarter of an hour and drops what is
// older than 24 hours. failed: the sub-probe that failed (services), kept
// as the slot's latest failure.
func addHist(hist []histSlot, ok bool, failed string, t int64) []histSlot {
	slot := t - t%histSlotSec
	if n := len(hist); n > 0 && hist[n-1].T == slot {
		hist[n-1].N++
		if ok {
			hist[n-1].OK++
		} else if failed != "" {
			hist[n-1].F = failed
		}
	} else {
		h := histSlot{T: slot, N: 1}
		if ok {
			h.OK = 1
		} else {
			h.F = failed
		}
		hist = append(hist, h)
	}
	cut := 0
	for cut < len(hist) && hist[cut].T <= t-histSpan {
		cut++
	}
	if cut > 0 {
		hist = append([]histSlot(nil), hist[cut:]...)
	}
	return hist
}

// recentHist: the slots of the last 24 hours, and the share that was ok (-1: none).
func recentHist(hist []histSlot, now int64) ([]histSlot, float64) {
	out := []histSlot{}
	ok, n := 0, 0
	for _, h := range hist {
		if h.T > now-histSpan {
			out = append(out, h)
			ok, n = ok+h.OK, n+h.N
		}
	}
	if n == 0 {
		return out, -1
	}
	return out, float64(ok) / float64(n)
}

// pingOne pings a device and records the result. Without the lock held.
func (s *Server) pingOne(id string) {
	s.st.mu.Lock()
	d := s.checkDevByID(id)
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
	var se *probe.SetupError
	s.loc.mu.Lock()
	if errors.As(err, &se) {
		if s.loc.setup == "" {
			log.Printf("agentless devices: cannot send pings: %v", err)
		}
		s.loc.setup, s.loc.setupAt = err.Error(), time.Now().Unix()
	} else {
		s.loc.setup = ""
		if m := probe.Method(); m != "" {
			s.loc.method = m
		}
	}
	s.loc.mu.Unlock()
	s.st.mu.Lock()
	if d := s.checkDevByID(id); d != nil && d.Address == addr {
		if se != nil {
			// The master's own problem, not the device's: no result.
			d.LastCheck, d.LastError = time.Now().Unix(), se.Error()
			s.markChecks()
		} else {
			now := time.Now()
			s.record(d.Checks, rtt, err, now)
			s.noteBehind(d.ID, err != nil, d.Hist, now)
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
		devs := s.checkDevs()
		dueSvc := s.dueServices(devs, now)
		pinged := map[string]bool{}
		for _, d := range devs {
			if !d.pinging() || d.Probe != "" || d.Maint != nil {
				continue
			}
			id := d.ID
			pinged[id] = true
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
			if !strings.HasPrefix(id, "s_") && !pinged[id] {
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
		for _, k := range dueSvc {
			go func(k svcKey) {
				sem <- struct{}{}
				defer func() { <-sem }()
				s.checkService(k.dev, k.svc)
				s.loc.mu.Lock()
				delete(s.loc.busy, k.svc)
				s.loc.mu.Unlock()
			}(k)
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

// writeLocalAudit records a change of an agentless device. Every such
// change may change a probe agent's check list: it is sent right away.
// Caller holds s.st.mu.
func (s *Server) writeLocalAudit(r *http.Request, action string, d *LocalDevice, detail string) {
	s.writeDevAudit(r, action, d.ID, d.Name, detail)
}

// writeDevAudit: the same for any device with checks. Caller holds s.st.mu.
func (s *Server) writeDevAudit(r *http.Request, action, id, name, detail string) {
	s.audit.add(auditEntry{Actor: actor(r), IP: clientIP(r), Action: action, Agent: id, Target: name, Detail: detail})
	s.probeSyncAll(time.Now())
}

// checkAddress: a valid IP that no other device uses. Caller holds s.st.mu.
func (s *Server) checkAddress(addr, except, probe string) (string, int, error) {
	ip, err := parseDeviceIP(addr)
	if err != nil {
		return "", http.StatusBadRequest, err
	}
	if _, name := s.ipOwner(ip, except, probe); name != "" {
		return "", http.StatusConflict, fmt.Errorf("IP address %s is already used by %s: a device can only be added once, with one type of monitoring", ip, name)
	}
	return ip, 0, nil
}

func (s *Server) handleLocalCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Address string `json:"address"`
		GroupID string `json:"group_id"`
		Probe   string `json:"probe"`
		Via     string `json:"via"`
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
	ip, code, err := s.checkAddress(req.Address, "", req.Probe)
	if err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	if req.GroupID != "" && s.st.group(req.GroupID) == nil {
		http.Error(w, "group not found", http.StatusBadRequest)
		return
	}
	d := &LocalDevice{ID: "l_" + randHex(8), Name: name, Checks: Checks{Address: ip}, GroupID: req.GroupID, AddedAt: time.Now().Unix()}
	if d.Via, err = s.checkVia(d.ID, req.Via); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Probe != "" {
		if err := s.probeAllowed(req.Probe, 1, s.ownIPs()); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		d.Probe = req.Probe
	}
	d.Order = s.st.nextOrder(d.GroupID)
	s.st.st.Locals[d.ID] = d
	s.st.markDirty()
	detail := "address " + ip + ", checked by " + s.probeName(d.Probe)
	if d.Via != "" {
		detail += ", connected via " + s.viaName(d.Via)
	}
	s.writeLocalAudit(r, "device.local_added", d, detail)
	s.probeSyncAll(time.Now())
	writeJSON(w, s.localDeviceView(d, false))
}

func (s *Server) handleLocalUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    *string `json:"name"`
		Address *string `json:"address"`
		GroupID *string `json:"group_id"`
		Probe   *string `json:"probe"`
		Via     *string `json:"via"`
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
	via := d.Via
	if req.Via != nil {
		var err error
		if via, err = s.checkVia(d.ID, *req.Via); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	// Everything is checked before anything changes.
	name := ""
	if req.Name != nil {
		n, err := checkName("device name", *req.Name, maxSystemName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		name = n
	}
	if req.GroupID != nil && *req.GroupID != "" && s.st.group(*req.GroupID) == nil {
		http.Error(w, "group not found", http.StatusBadRequest)
		return
	}
	probe := d.Probe
	if req.Probe != nil && *req.Probe != d.Probe {
		probe = *req.Probe
		if probe != "" {
			if err := s.probeAllowed(probe, d.activeChecks(), s.ownIPs()); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
		}
	}
	ip := d.Address
	if req.Address != nil || probe != d.Probe {
		addr := d.Address
		if req.Address != nil {
			addr = *req.Address
		}
		var code int
		var err error
		if ip, code, err = s.checkAddress(addr, d.ID, probe); err != nil {
			http.Error(w, err.Error(), code)
			return
		}
	}

	var changes []string
	if probe != d.Probe {
		// The history stays: it is the same device, checked from elsewhere.
		changes = append(changes, "checked by "+s.probeName(d.Probe)+" → "+s.probeName(probe))
		d.Probe = probe
		s.loc.mu.Lock()
		delete(s.loc.next, d.ID)
		for _, sv := range d.Services {
			delete(s.loc.next, sv.ID)
		}
		s.loc.mu.Unlock()
	}
	if name != "" && name != d.Name {
		changes = append(changes, "name "+d.Name+" → "+name)
		d.Name = name
	}
	if via != d.Via {
		changes = append(changes, "connected via "+s.viaName(d.Via)+" → "+s.viaName(via))
		d.Via = via
	}
	if ip != d.Address {
		changes = append(changes, "address "+d.Address+" → "+ip)
		d.Address = ip
		// A new address starts over.
		d.resetPing()
		s.loc.mu.Lock()
		delete(s.loc.next, d.ID)
		s.loc.mu.Unlock()
		for _, sv := range d.Services {
			s.resetService(sv)
		}
	}
	if req.GroupID != nil && *req.GroupID != d.GroupID {
		d.Order = s.st.nextOrder(*req.GroupID)
		d.GroupID = *req.GroupID
	}
	s.st.markDirty()
	s.markChecks()
	if len(changes) > 0 {
		s.writeLocalAudit(r, "device.local_changed", d, strings.Join(changes, "; "))
	}
	s.probeSyncAll(time.Now())
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
	s.dropVia(d.ID)
	s.st.markDirty()
	s.markChecks()
	s.writeLocalAudit(r, "device.local_removed", d, "address "+d.Address)
	writeJSON(w, map[string]bool{"removed": true})
}

func (s *Server) handleLocalPing(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	d := s.checkDevOr404(w, r, false)
	if d == nil {
		s.st.mu.Unlock()
		return
	}
	id, pinging, probe := d.ID, d.pinging(), d.Probe
	s.st.mu.Unlock()
	if probe != "" {
		http.Error(w, remoteCheckMsg, http.StatusConflict)
		return
	}
	if !pinging {
		http.Error(w, "ping is paused or removed for this device", http.StatusConflict)
		return
	}
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
	d = s.checkDevByID(id)
	if d == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	writeJSON(w, s.devView(d, true))
}

// handlePingState pauses, resumes, removes or adds the ping of a device.
func (s *Server) handlePingState(w http.ResponseWriter, r *http.Request) {
	var req struct {
		State string `json:"state"` // on, paused or removed
	}
	if !readJSON(w, r, &req, 1024) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	want := map[string]string{"on": "", "paused": pingPaused, "removed": pingRemoved}
	mode, ok := want[req.State]
	if !ok {
		http.Error(w, "state must be on, paused or removed", http.StatusBadRequest)
		return
	}
	d := s.checkDevOr404(w, r, mode == "")
	if d == nil {
		return
	}
	if mode == d.PingMode {
		writeJSON(w, s.devView(d, true))
		return
	}
	if mode != "" && d.pinging() && d.activeChecks() <= d.minChecks() {
		http.Error(w, noCheckMsg, http.StatusConflict)
		return
	}
	if mode == "" {
		if err := s.probeRoom(d.Checks); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if code, err := s.attach(d); err != nil {
			http.Error(w, err.Error(), code)
			return
		}
	}
	action := map[string]string{"": "device.ping_resumed", pingPaused: "device.ping_paused", pingRemoved: "device.ping_removed"}[mode]
	switch {
	case mode == pingRemoved:
		d.resetPing()
	case mode == "" && d.PingMode == pingRemoved:
		d.resetPing()
		action = "device.ping_added"
	case mode == "":
		d.Fails = 0 // resumed: decide afresh from the next pings
	}
	d.PingMode = mode
	s.loc.mu.Lock()
	delete(s.loc.next, d.ID)
	s.loc.mu.Unlock()
	s.st.markDirty()
	s.markChecks()
	s.writeDevAudit(r, action, d.ID, d.Name, "")
	writeJSON(w, s.devView(d, true))
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

// replanPings applies a new ping interval right away instead of after the
// old one: a ping waiting longer than the new interval since the last one is
// moved forward. Services keep their own schedule. Caller holds s.st.mu.
func (s *Server) replanPings(intervalSec int, now time.Time) {
	iv := time.Duration(intervalSec) * time.Second
	s.loc.mu.Lock()
	defer s.loc.mu.Unlock()
	for _, d := range s.checkDevs() {
		id := d.ID
		next, ok := s.loc.next[id]
		if !ok || !d.pinging() {
			continue
		}
		want := now.Add(iv)
		if d.LastCheck > 0 {
			want = time.Unix(d.LastCheck, 0).Add(iv)
		}
		if want.Before(now) {
			want = now
		}
		if next.After(want) {
			s.loc.next[id] = want
		}
	}
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
	if old.IntervalSec != req.IntervalSec {
		s.replanPings(req.IntervalSec, time.Now())
	}
	s.probeSyncAll(time.Now()) // the ping settings are part of the check lists
	s.st.mu.Unlock()
	s.audit.write(actor(r), clientIP(r), "settings.local", "", fmt.Sprintf("ping every %ds, timeout %ds, down after %d failed", req.IntervalSec, req.TimeoutSec, req.DownAfter))
	writeJSON(w, map[string]any{"settings": req})
}
