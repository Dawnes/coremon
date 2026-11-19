package master

// Remote probes ("Checked by"): an agentless device can be checked by an
// agent on another system instead of by the master, for networks the
// master cannot reach. The master stays in control: it sends each probe
// agent its complete check list (with a version: a hash of the list) and
// the agent runs the checks on its own schedule and reports the results
// with every poll, and right away when a status changes. The results are
// stored exactly like the master's own. There is no automatic takeover by
// the master: while the probe is unavailable its devices are "unknown".
//
// An admin can clear an agent's list (the agent drops everything); the
// master then sends nothing until Resend, or until the list changes.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"coremon/internal/proto"
)

const (
	maxProbeChecks = 200 // checks per probe agent
	maxProbeBatch  = 2000
	probeMinAgent  = "0.3.9"
	probeRetry     = 2 * time.Minute // after the agent refused a list
)

func (s *Server) probeName(id string) string {
	if id == "" {
		return "Master"
	}
	if a := s.st.st.Agents[id]; a != nil {
		return a.Name
	}
	return "a removed agent"
}

// probeList: the checks of the devices that agentID checks, in a fixed
// order; the version is a hash of them ("" when there are none). Caller
// holds s.st.mu.
func (s *Server) probeList(agentID string) proto.ProbeList {
	ls := s.st.st.Local
	var devs []*checkDev
	for _, d := range s.checkDevs() { // sorted by ID
		if d.Probe == agentID && d.Maint == nil { // in maintenance: not checked
			devs = append(devs, d)
		}
	}
	l := proto.ProbeList{Checks: []proto.ProbeCheck{}}
	for _, d := range devs {
		if d.pinging() {
			l.Checks = append(l.Checks, proto.ProbeCheck{ID: d.ID, Kind: "ping", Address: d.Address,
				Interval: ls.IntervalSec, Timeout: ls.TimeoutSec, DownAfter: ls.DownAfter})
		}
		for _, sv := range d.Services {
			if !sv.Paused {
				l.Checks = append(l.Checks, proto.ProbeCheck{ID: sv.ID, Kind: sv.kind(), Address: d.Address, Port: sv.Port,
					Greeting: sv.Greeting, Reverse: sv.Reverse, Interval: sv.Interval, Timeout: sv.Timeout, Retries: sv.Retries,
					Host: sv.Host, Path: sv.Path, Codes: sv.Codes, CertValid: sv.CertValid, CertDays: sv.CertDays,
					RType: sv.RType, Expect: sv.Expect, MaxMs: sv.MaxMs})
			}
		}
	}
	if len(l.Checks) > 0 {
		js, _ := json.Marshal(l.Checks)
		sum := sha256.Sum256(js)
		l.Version = hex.EncodeToString(sum[:8])
	}
	return l
}

// probeLoad: the checks that run on agentID. Caller holds s.st.mu.
func (s *Server) probeLoad(agentID string) int {
	n := 0
	for _, d := range s.checkDevs() {
		if d.Probe == agentID {
			n += d.activeChecks()
		}
	}
	return n
}

// probeAllowed: agentID may take extra more checks. Caller holds s.st.mu.
func (s *Server) probeAllowed(agentID string, extra int, local map[string]bool) error {
	a := s.st.st.Agents[agentID]
	switch {
	case a == nil:
		return errors.New("that agent does not exist")
	case a.Status == StatusPending:
		return fmt.Errorf("%s is waiting for approval", a.Name)
	case a.Removing:
		return fmt.Errorf("%s is being uninstalled", a.Name)
	case s.belowMin(a) || !a.ProbeCap:
		return fmt.Errorf("%s cannot check devices: update its agent to %s or later", a.Name, probeMinAgent)
	case onMaster(a, local):
		return fmt.Errorf("%s runs on the master's own system: choose Master instead", a.Name)
	case !a.ProbeOn:
		return fmt.Errorf("%s is not enabled as a remote probe: switch that on on its device page first", a.Name)
	}
	if n := s.probeLoad(agentID) + extra; n > maxProbeChecks {
		return fmt.Errorf("%s would run %d checks: at most %d per agent", a.Name, n, maxProbeChecks)
	}
	return nil
}

// probeRoom: one more check on d (a service added or resumed, the ping
// added or resumed) fits on its probe. Caller holds s.st.mu.
func (s *Server) probeRoom(d *Checks) error {
	if d.Probe == "" {
		return nil
	}
	if n := s.probeLoad(d.Probe) + 1; n > maxProbeChecks {
		return fmt.Errorf("%s would run %d checks: at most %d per agent", s.probeName(d.Probe), n, maxProbeChecks)
	}
	return nil
}

// probeIntake stores what a probe agent reported. Caller holds s.st.mu.
func (s *Server) probeIntake(a *Agent, rep *proto.ProbeReport, now time.Time) {
	if !a.ProbeCap {
		a.ProbeCap = true
		s.st.markAgents()
	}
	if v := clip(rep.Version, 64); v != a.ProbeVer {
		a.ProbeVer = v
		s.st.markAgents()
	}
	if len(rep.Results) == 0 {
		return
	}
	pings := map[string]*checkDev{}
	svcs := map[string]*Service{}
	svcDev := map[string]string{} // service ID -> device name
	for _, d := range s.checkDevs() {
		if d.Probe != a.ID || d.Maint != nil {
			continue // late results for a device in maintenance are dropped
		}
		pings[d.ID] = d
		for _, sv := range d.Services {
			svcs[sv.ID] = sv
			svcDev[sv.ID] = d.Name
		}
	}
	res := rep.Results
	if len(res) > maxProbeBatch {
		res = res[len(res)-maxProbeBatch:]
	}
	var dl *probeDelivery
	if s.st.st.Settings.ProbeResultsShown {
		dl = &probeDelivery{AgentID: a.ID, Agent: a.Name, At: now.Unix()}
		defer s.addDelivery(dl, now.Unix())
	}
	for _, r := range res {
		ago := r.AgoMs
		if ago < 0 {
			ago = 0
		}
		if ago > histSpan*1000 {
			continue
		}
		at := now.Add(-time.Duration(ago) * time.Millisecond)
		t := at.Unix()
		us := r.RTTUs
		if us < 0 || us > 120e6 {
			us = 0
		}
		rtt := time.Duration(us) * time.Microsecond
		var cerr error
		if !r.OK {
			msg := cleanText(r.Error, 200)
			if msg == "" {
				msg = "no answer"
			}
			cerr = errors.New(msg)
		}
		if d := pings[r.ID]; d != nil {
			if !d.pinging() || t <= d.LastCheck {
				continue // paused meanwhile, or already stored
			}
			if r.Setup {
				// The probe's own problem, not the device's: no result.
				d.LastCheck, d.LastError = t, "the probe agent cannot send pings: "+cleanText(r.Error, 200)
				s.markChecks()
				dl.add(d.Name+" · Ping", false, "the probe cannot send pings")
				continue
			}
			s.record(d.Checks, rtt, cerr, at)
			dl.add(d.Name+" · Ping", r.OK, outcome(r.OK, "up", rtt, cerr))
		} else if sv := svcs[r.ID]; sv != nil {
			if sv.Paused || t <= sv.LastCheck {
				continue
			}
			ok := s.storeService(sv, rtt, cleanText(r.Banner, maxBanner), r.OK, cerr, r.Failed, at)
			what := outcome(r.OK, "reached", rtt, cerr)
			if sv.Reverse {
				what += " (expected closed)"
			}
			dl.add(svcDev[sv.ID]+" · "+sv.Name, ok, what)
		}
	}
}

// probeReport: results sent right away (op "probe"), outside the poll.
func (s *Server) probeReport(agentID string, rep *proto.ProbeReport) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	a := s.st.st.Agents[agentID]
	if a == nil || a.Status == StatusPending || s.belowMin(a) {
		return
	}
	s.probeIntake(a, rep, time.Now())
}

// probeSync queues the current list for an agent that runs another one,
// unless an admin cleared it and the list did not change since. Caller
// holds s.st.mu.
func (s *Server) probeSync(a *Agent, now time.Time) {
	if !a.ProbeCap || a.Status == StatusPending || a.Removing || s.belowMin(a) {
		return
	}
	want := s.probeList(a.ID).Version
	if s.probeWant == nil {
		s.probeWant = map[string]string{}
	}
	s.probeWant[a.ID] = want
	if a.ProbeHold != nil {
		if *a.ProbeHold == want {
			return
		}
		a.ProbeHold = nil // the checks changed: back to normal
		s.st.markDirty()
	}
	if a.ProbeVer == want || !s.online(a, now.Unix()) || s.openJob(a.ID, proto.CmdProbeList, "") || s.openJob(a.ID, proto.CmdProbeClear, "") ||
		s.recentFailure(a.ID, proto.CmdProbeList, "", probeRetry) {
		return
	}
	s.enqueue(a, &Job{Type: proto.CmdProbeList, Actor: "scheduler", Timeout: 60})
}

func (s *Server) probeSyncAll(now time.Time) {
	for _, a := range s.st.st.Agents {
		s.probeSync(a, now)
	}
}

// probeWanted: the list version the agent should run (cached by probeSync).
func (s *Server) probeWanted(agentID string) string {
	if v, ok := s.probeWant[agentID]; ok {
		return v
	}
	return s.probeList(agentID).Version
}

// ---- views ----

// localProbeView: how the probe of an agentless device is doing.
type localProbeView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"` // ok, offline, cleared, waiting, unavailable
	Note  string `json:"note,omitempty"`
}

func (s *Server) localProbe(d *Checks, now int64) *localProbeView {
	if d.Probe == "" {
		return nil
	}
	a := s.st.st.Agents[d.Probe]
	v := &localProbeView{ID: d.Probe, Name: s.probeName(d.Probe), State: "ok"}
	switch {
	case a == nil:
		v.State, v.Note = "unavailable", "the agent that checks this device was removed: choose another one"
	case !a.ProbeCap || s.belowMin(a) || a.Removing || a.Status == StatusPending:
		v.State, v.Note = "unavailable", "the agent cannot check devices now"
	case !s.online(a, now):
		v.State, v.Note = "offline", "probe offline"
	case a.ProbeHold != nil:
		v.State, v.Note = "cleared", "waiting for probe: its check list was cleared"
	case a.ProbeVer == "":
		v.State, v.Note = "waiting", "waiting for probe: the check list is not confirmed yet"
	}
	return v
}

// agentProbeView: an agent's work as a probe, for its device page.
type agentProbeView struct {
	On          bool   `json:"on"` // allowed to act as a remote probe
	Devices     int    `json:"devices"`
	Checks      int    `json:"checks"`
	Max         int    `json:"max"`
	Running     string `json:"running"` // version the agent runs
	Want        string `json:"want"`
	Cleared     bool   `json:"cleared"`
	ConfirmedAt int64  `json:"confirmed_at"`
}

func (s *Server) agentProbe(a *Agent, local map[string]bool) *agentProbeView {
	if !a.ProbeCap || s.belowMin(a) || a.Status == StatusPending || onMaster(a, local) {
		return nil
	}
	v := &agentProbeView{On: a.ProbeOn, Max: maxProbeChecks, Running: a.ProbeVer, Want: s.probeWanted(a.ID), Cleared: a.ProbeHold != nil, ConfirmedAt: a.ProbeAt}
	for _, d := range s.checkDevs() {
		if d.Probe == a.ID {
			v.Devices++
			v.Checks += d.activeChecks()
		}
	}
	return v
}

// probeChoice: an agent offered in "Checked by".
type probeChoice struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Online bool   `json:"online"`
	Checks int    `json:"checks"`
	Note   string `json:"note,omitempty"` // why it cannot be chosen
}

func (s *Server) handleProbeChoices(w http.ResponseWriter, r *http.Request) {
	local := s.ownIPs()
	now := time.Now().Unix()
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	out := []probeChoice{}
	for _, a := range s.orderedAgents() {
		if a.Status == StatusPending || onMaster(a, local) || !a.ProbeOn {
			continue
		}
		c := probeChoice{ID: a.ID, Name: a.Name, Online: s.online(a, now), Checks: s.probeLoad(a.ID)}
		if !a.ProbeCap || s.belowMin(a) {
			c.Note = "agent " + probeMinAgent + " or later needed"
		} else if a.Removing {
			c.Note = "being uninstalled"
		}
		out = append(out, c)
	}
	writeJSON(w, map[string]any{"agents": out, "max": maxProbeChecks})
}

// ---- the switch and the two admin buttons on an agent's page ----

// handleProbeEnabled switches the agent's remote probe role on or off.
// Off only when no agentless device is checked by it any more.
func (s *Server) handleProbeEnabled(w http.ResponseWriter, r *http.Request) {
	var req struct {
		On bool `json:"on"`
	}
	if !readJSON(w, r, &req, 256) {
		return
	}
	local := s.ownIPs()
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	a := s.agentOr404(w, r)
	if a == nil {
		return
	}
	if onMaster(a, local) {
		http.Error(w, a.Name+" runs on the master's own system: it cannot be a remote probe", http.StatusConflict)
		return
	}
	if a.ProbeOn == req.On {
		writeJSON(w, map[string]any{"on": a.ProbeOn})
		return
	}
	if !req.On {
		n := 0
		for _, d := range s.checkDevs() {
			if d.Probe == a.ID {
				n++
			}
		}
		if n > 0 {
			http.Error(w, fmt.Sprintf("%s still checks %s: choose another checker for them first", a.Name, plural(n, "device")), http.StatusConflict)
			return
		}
	}
	a.ProbeOn = req.On
	s.st.markDirty()
	act := "agent.probe_enabled"
	if !req.On {
		act = "agent.probe_disabled"
	}
	s.audit.writeAgent(actor(r), clientIP(r), act, a, "")
	writeJSON(w, map[string]any{"on": a.ProbeOn})
}

func (s *Server) cancelProbeLists(a *Agent, reason string) {
	now := time.Now().Unix()
	for _, j := range s.st.st.Jobs {
		if j.AgentID == a.ID && j.Status == JobQueued && j.Type == proto.CmdProbeList {
			j.Status, j.Finished, j.Error = JobCancelled, now, "cancelled: "+reason
			s.st.markJobs()
		}
	}
}

func (s *Server) probeButton(w http.ResponseWriter, r *http.Request) *Agent {
	a := s.agentOr404(w, r)
	if a == nil {
		return nil
	}
	if !a.ProbeCap || s.belowMin(a) || a.Status == StatusPending {
		http.Error(w, a.Name+" cannot check devices: update its agent to "+probeMinAgent+" or later", http.StatusConflict)
		return nil
	}
	return a
}

// handleProbeClear: the agent drops its whole list and waits; the master
// sends nothing until Resend or a change of the checks.
func (s *Server) handleProbeClear(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	a := s.probeButton(w, r)
	if a == nil {
		return
	}
	s.cancelProbeLists(a, "check list cleared")
	want := s.probeList(a.ID).Version
	a.ProbeHold = &want
	j := s.enqueue(a, &Job{Type: proto.CmdProbeClear, Actor: actor(r), Timeout: 60})
	s.st.markDirty()
	s.audit.writeJob(actor(r), clientIP(r), "agent.probe_cleared", a, "", j)
	writeJSON(w, map[string]any{"job": j.ID})
}

// handleProbeResend sends the current list, also when the agent has it.
func (s *Server) handleProbeResend(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	a := s.probeButton(w, r)
	if a == nil {
		return
	}
	s.cancelProbeLists(a, "sent again")
	a.ProbeHold = nil
	j := s.enqueue(a, &Job{Type: proto.CmdProbeList, Actor: actor(r), Timeout: 60})
	s.st.markDirty()
	s.audit.writeJob(actor(r), clientIP(r), "agent.probe_resent", a, "", j)
	writeJSON(w, map[string]any{"job": j.ID})
}
