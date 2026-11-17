package master

// Remote Probes (Activity > Remote Probes): what the master is waiting
// for from agents on other systems. Requests the master handles itself
// (its own agentless checks) and the agent on the master's own system are
// not shown. Listed are the agents' open jobs (including a check list or
// a clear not confirmed yet) and the checks of remote probes whose result
// is overdue. A request leaves the list when it is answered; one that
// gave up stays 10 minutes.

import (
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"coremon/internal/proto"
)

const probeGaveUpKeep = 10 * 60 // seconds a request that gave up stays listed

type probeRequest struct {
	*Job
	Kind     string `json:"kind"` // "job" or "check"
	AgentID  string `json:"agent_id"`
	Agent    string `json:"agent"`
	Title    string `json:"title,omitempty"` // checks
	DeviceID string `json:"device_id,omitempty"`
	Since    int64  `json:"since"` // requested (or expected) at
	State    string `json:"state"` // waiting, running, overdue, offline, gave_up
	Note     string `json:"note,omitempty"`
}

// onMaster: the agent runs on the master's own system.
func onMaster(a *Agent, local map[string]bool) bool {
	for _, ip := range a.Facts.IPs {
		if p := net.ParseIP(strings.TrimSpace(ip)); p != nil && local[p.String()] {
			return true
		}
	}
	return false
}

// gaveUp: the job ended without an answer from its agent.
func gaveUp(j *Job) bool {
	switch j.Status {
	case JobTimeout:
		return true
	case JobFailed, JobCancelled:
		return strings.HasPrefix(j.Error, "expired") || strings.HasPrefix(j.Error, "agent lost") || strings.HasPrefix(j.Error, "no answer")
	}
	return false
}

// probeRequests lists the open and recently abandoned requests. Caller holds s.st.mu.
func (s *Server) probeRequests(now int64, local map[string]bool) []probeRequest {
	out := []probeRequest{}
	hb := int64(s.heartbeat())
	for _, j := range s.st.st.Jobs {
		a := s.st.st.Agents[j.AgentID]
		if a == nil || onMaster(a, local) {
			continue
		}
		p := probeRequest{Job: j, Kind: "job", AgentID: a.ID, Agent: a.Name, Since: j.Created}
		switch {
		case j.Status == JobQueued && !s.online(a, now):
			p.State, p.Note = "offline", "the agent is offline; it runs when the agent is back"
		case j.Status == JobQueued && now-j.Created > 2*hb:
			p.State, p.Note = "overdue", "not picked up by the agent"
		case j.Status == JobQueued:
			p.State = "waiting"
		case j.Status == JobRunning && j.Started > 0 && now-j.Started > int64(j.Timeout):
			p.State, p.Note = "overdue", "no result within its timeout"
		case j.Status == JobRunning:
			p.State = "running"
		case gaveUp(j) && now-j.Finished <= probeGaveUpKeep:
			p.State, p.Note = "gave_up", j.Error
		default:
			continue
		}
		if !j.done() && p.Note == "" {
			switch j.Type {
			case proto.CmdProbeList:
				p.Note = "check list sent, not yet confirmed"
			case proto.CmdProbeClear:
				p.Note = "clear sent, not yet confirmed"
			}
		}
		out = append(out, p)
	}
	out = append(out, s.overdueChecks(now, local)...)
	sort.SliceStable(out, func(i, j int) bool {
		gi, gj := out[i].State == "gave_up", out[j].State == "gave_up"
		if gi != gj {
			return !gi // open requests first
		}
		return out[i].Since < out[j].Since // oldest first
	})
	return out
}

// overdueChecks: checks of remote probes that should have reported by now
// (interval + timeout + one heartbeat after the last result), for probes
// that are online and run the current list. Caller holds s.st.mu.
func (s *Server) overdueChecks(now int64, local map[string]bool) []probeRequest {
	var out []probeRequest
	hb := int64(s.heartbeat())
	ls := s.st.st.Local
	for _, d := range s.st.st.Locals {
		a := s.st.st.Agents[d.Probe]
		if d.Probe == "" || a == nil || onMaster(a, local) || !s.online(a, now) || a.ProbeHold != nil ||
			a.ProbeVer == "" || a.ProbeVer != s.probeWanted(a.ID) {
			continue
		}
		add := func(title string, last int64, every, timeout int) {
			if a.ProbeAt > last {
				last = a.ProbeAt // the list was (re)confirmed later
			}
			due := last + int64(every)
			if now <= due+int64(timeout)+hb {
				return
			}
			out = append(out, probeRequest{Kind: "check", AgentID: a.ID, Agent: a.Name, Title: title, DeviceID: d.ID,
				Since: due, State: "overdue", Note: "no result in time"})
		}
		if d.pinging() {
			add(d.Name+" · Ping", d.LastCheck, ls.IntervalSec, ls.TimeoutSec)
		}
		for _, sv := range d.Services {
			if sv.Paused {
				continue
			}
			every := sv.Interval
			if sv.Fails > 0 {
				every = int(svcRetryEvery / time.Second)
			}
			add(d.Name+" · "+sv.Name, sv.LastCheck, every, sv.Timeout)
		}
	}
	return out
}

func (s *Server) handleProbes(w http.ResponseWriter, r *http.Request) {
	local := s.ownIPs()
	now := time.Now().Unix()
	s.st.mu.Lock()
	reqs := s.probeRequests(now, local)
	s.st.mu.Unlock()
	writeJSON(w, map[string]any{"now": now, "requests": reqs})
}
