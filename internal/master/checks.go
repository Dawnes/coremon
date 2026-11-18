package master

// Checks: what is checked on a device from outside, by the master or by a
// remote probe agent: the ping and the services. An agentless device always
// has them; an agent device once a service is added on its Services tab
// (its own status still comes from its agent's contact). Both kinds share
// the scheduling, the remote probes, the history and the website views.

import (
	"errors"
	"net/http"
	"sort"
	"strings"
)

type Checks struct {
	Address string `json:"address"`
	// The ping's results: kept in checks.dat, not in state.dat.
	Checked   bool       `json:"-"` // up or down has been decided
	Up        bool       `json:"-"`
	Since     int64      `json:"-"` // when it went up or down
	LastCheck int64      `json:"-"`
	LastUp    int64      `json:"-"` // last answered ping
	RTTUs     int64      `json:"-"`
	Fails     int        `json:"-"` // unanswered pings in a row
	LastError string     `json:"-"`
	Hist      []histSlot `json:"-"`
	RTT       []rttSlot  `json:"-"`
	PingMode  string     `json:"ping_mode,omitempty"` // "" pinged, "paused" or "removed"
	Services  []*Service `json:"services,omitempty"`
	// Probe: the agent that checks this device ("Checked by"); "" = the master.
	Probe string `json:"probe,omitempty"`
}

func (c *Checks) pinging() bool { return c.PingMode == "" }

// activeChecks: the checks that run (the ping and the services not paused).
func (c *Checks) activeChecks() int {
	n := 0
	if c.pinging() {
		n++
	}
	for _, sv := range c.Services {
		if !sv.Paused {
			n++
		}
	}
	return n
}

func (c *Checks) resetPing() {
	c.Checked, c.Up, c.Since, c.LastCheck, c.LastUp, c.RTTUs, c.Fails, c.LastError, c.Hist, c.RTT = false, false, 0, 0, 0, 0, 0, "", nil, nil
}

func (c *Checks) service(id string) *Service {
	for _, sv := range c.Services {
		if sv.ID == id {
			return sv
		}
	}
	return nil
}

// checkDev: a device with checks, agentless or agent.
type checkDev struct {
	*Checks
	ID    string
	Name  string
	Maint *Maintenance
	Local *LocalDevice // the agentless device, or nil
	Agent *Agent       // the agent device, or nil
}

// agentHasChecks: the agent device has its own checks (approved, with a
// checks block). Pending agents get none.
func agentHasChecks(a *Agent) bool { return a.Checks != nil && a.Status != StatusPending }

// checkDevs: every device with checks, in a fixed order. Caller holds s.st.mu.
func (s *Server) checkDevs() []*checkDev {
	out := make([]*checkDev, 0, len(s.st.st.Locals))
	for _, d := range s.st.st.Locals {
		out = append(out, &checkDev{Checks: &d.Checks, ID: d.ID, Name: d.Name, Maint: d.Maint, Local: d})
	}
	for _, a := range s.st.st.Agents {
		if agentHasChecks(a) {
			out = append(out, &checkDev{Checks: a.Checks, ID: a.ID, Name: a.Name, Maint: a.Maint, Agent: a})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// checkDevByID: the device with checks, or nil. Caller holds s.st.mu.
func (s *Server) checkDevByID(id string) *checkDev {
	if d := s.st.st.Locals[id]; d != nil {
		return &checkDev{Checks: &d.Checks, ID: d.ID, Name: d.Name, Maint: d.Maint, Local: d}
	}
	if a := s.st.st.Agents[id]; a != nil && agentHasChecks(a) {
		return &checkDev{Checks: a.Checks, ID: a.ID, Name: a.Name, Maint: a.Maint, Agent: a}
	}
	return nil
}

// checkDevOr404: the device of a check request. An approved agent device
// without checks yet is returned with an empty checks block only when
// create is set (its first service). Caller holds s.st.mu.
func (s *Server) checkDevOr404(w http.ResponseWriter, r *http.Request, create bool) *checkDev {
	id := r.PathValue("id")
	if d := s.checkDevByID(id); d != nil {
		return d
	}
	if a := s.st.st.Agents[id]; a != nil && create {
		if a.Status == StatusPending {
			http.Error(w, a.Name+" is waiting for approval", http.StatusConflict)
			return nil
		}
		return &checkDev{Checks: &Checks{PingMode: pingRemoved}, ID: a.ID, Name: a.Name, Maint: a.Maint, Agent: a}
	}
	if s.st.st.Agents[id] != nil {
		http.Error(w, "this device has no services", http.StatusNotFound)
		return nil
	}
	http.Error(w, "device not found", http.StatusNotFound)
	return nil
}

// minChecks: an agentless device must keep at least one running check (its
// status comes from them); an agent device may have none.
func (d *checkDev) minChecks() int {
	if d.Local != nil {
		return 1
	}
	return 0
}

// tidy drops the checks block of an agent device that checks nothing any
// more (no services, no ping). Caller holds s.st.mu.
func (s *Server) tidy(d *checkDev) {
	if d.Agent != nil && len(d.Services) == 0 && d.PingMode == pingRemoved {
		d.Agent.Checks = nil
	}
}

// devView: the device's own view (after a change), as the website needs it.
func (s *Server) devView(d *checkDev, full bool) agentView {
	if d.Local != nil {
		return s.localDeviceView(d.Local, full)
	}
	return s.view(d.Agent, full)
}

// agentAddresses: the addresses of an agent device that its checks can use
// (no loopback, link-local or unusable ones), IPv4 first.
func agentAddresses(a *Agent) []string {
	var v4, v6 []string
	seen := map[string]bool{}
	for _, x := range a.Facts.IPs {
		ip := canonIP(x)
		if ip == "" || seen[ip] {
			continue
		}
		if _, err := parseDeviceIP(ip); err != nil {
			continue
		}
		seen[ip] = true
		if strings.Contains(ip, ":") {
			v6 = append(v6, ip)
		} else {
			v4 = append(v4, ip)
		}
	}
	return append(v4, v6...)
}

// handleAgentChecks sets the address and "Checked by" of an agent device's
// checks (created if needed).
func (s *Server) handleAgentChecks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address *string `json:"address"`
		Probe   *string `json:"probe"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	a := s.agentOr404(w, r)
	if a == nil {
		return
	}
	if a.Status == StatusPending {
		http.Error(w, a.Name+" is waiting for approval", http.StatusConflict)
		return
	}
	c := a.Checks
	if c == nil {
		http.Error(w, "add a service first", http.StatusConflict)
		return
	}
	probe := c.Probe
	if req.Probe != nil && *req.Probe != c.Probe {
		probe = *req.Probe
		if probe == a.ID {
			http.Error(w, "a device cannot be checked by its own agent: choose the master or another agent", http.StatusConflict)
			return
		}
		if probe != "" {
			if err := s.probeAllowed(probe, c.activeChecks(), s.ownIPs()); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
		}
	}
	addr := c.Address
	if req.Address != nil {
		addr = *req.Address
	}
	ip, code, err := s.checkAddress(addr, a.ID, probe)
	if err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	var changes []string
	if probe != c.Probe {
		changes = append(changes, "checked by "+s.probeName(c.Probe)+" → "+s.probeName(probe))
		c.Probe = probe
		s.loc.mu.Lock()
		delete(s.loc.next, a.ID)
		for _, sv := range c.Services {
			delete(s.loc.next, sv.ID)
		}
		s.loc.mu.Unlock()
	}
	if ip != c.Address {
		if c.Address != "" {
			changes = append(changes, "address "+c.Address+" → "+ip)
		} else {
			changes = append(changes, "address "+ip)
		}
		c.Address = ip
		c.resetPing()
		s.loc.mu.Lock()
		delete(s.loc.next, a.ID)
		s.loc.mu.Unlock()
		for _, sv := range c.Services {
			s.resetService(sv)
		}
	}
	s.st.markDirty()
	s.markChecks()
	if len(changes) > 0 {
		s.writeDevAudit(r, "device.checks_changed", a.ID, a.Name, strings.Join(changes, "; "))
	}
	writeJSON(w, s.view(a, true))
}

// attach gives an agent device that has no checks yet its checks block,
// with its first usable address, checked by the master. Caller holds s.st.mu.
func (s *Server) attach(d *checkDev) (int, error) {
	if d.Agent == nil || d.Agent.Checks != nil {
		return 0, nil
	}
	l := agentAddresses(d.Agent)
	if len(l) == 0 {
		return http.StatusConflict, errors.New(d.Name + " reports no usable IP address yet: wait for its agent to report one")
	}
	ip, code, err := s.checkAddress(l[0], d.ID, "")
	if err != nil {
		return code, err
	}
	d.Address = ip
	d.Agent.Checks = d.Checks
	return 0, nil
}
