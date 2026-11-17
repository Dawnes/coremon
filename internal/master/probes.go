package master

// Remote Probes (Activity > Remote Probes): what the master is waiting
// for from agents on other systems. Requests the master handles itself
// (its own agentless checks) and the agent on the master's own system are
// not shown. Listed are the agents' open jobs (including a check list or
// a clear not confirmed yet) and the checks of remote probes whose result
// is overdue. An answered request stays 20 seconds; one that gave up
// stays 10 minutes. With the general setting on, each delivery of probe
// results is listed too, for 20 seconds (kept in memory only).

import (
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"coremon/internal/proto"
)

const (
	probeGaveUpKeep   = 10 * 60 // seconds a request that gave up stays listed
	probeAnsweredKeep = 20      // seconds an answered request (or a result delivery) stays listed
	probeRecvMax      = 200     // deliveries kept at most
	probeRecvItems    = 50      // checks named per delivery at most
)

type probeRequest struct {
	*Job
	Kind     string   `json:"kind"` // "job", "check" or "results"
	AgentID  string   `json:"agent_id"`
	Agent    string   `json:"agent"`
	Title    string   `json:"title,omitempty"` // checks
	DeviceID string   `json:"device_id,omitempty"`
	Since    int64    `json:"since"` // requested (or expected, or received) at
	State    string   `json:"state"` // waiting, running, overdue, offline, gave_up, answered, failed, received
	Note     string   `json:"note,omitempty"`
	Checks   int      `json:"checks,omitempty"` // results: checks in the delivery
	Failed   int      `json:"failed,omitempty"` // results: of which failed
	Items    []string `json:"items,omitempty"`  // results: "<check>: <outcome>"
	at       int64    // sort key of the done rows: answered or received at
}

// probeDelivery: one batch of results a probe agent sent (memory only).
type probeDelivery struct {
	AgentID, Agent string
	At             int64
	Checks, Failed int
	Items          []string
}

// add counts one check result; a nil delivery (setting off) ignores it.
func (dl *probeDelivery) add(title string, ok bool, what string) {
	if dl == nil {
		return
	}
	dl.Checks++
	if !ok {
		dl.Failed++
		what = "FAILED – " + what
	}
	if len(dl.Items) < probeRecvItems {
		dl.Items = append(dl.Items, title+": "+what)
	}
}

// outcome: short text of one result.
func outcome(reached bool, good string, rtt time.Duration, err error) string {
	if !reached {
		if err != nil {
			return err.Error()
		}
		return "no answer"
	}
	ms := float64(rtt.Microseconds()) / 1000
	if ms < 10 {
		return fmt.Sprintf("%s %.1f ms", good, ms)
	}
	return fmt.Sprintf("%s %.0f ms", good, ms)
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// addDelivery stores a delivery with results and drops expired ones. Caller holds s.st.mu.
func (s *Server) addDelivery(dl *probeDelivery, now int64) {
	keep := s.probeRecv[:0]
	for _, d := range s.probeRecv {
		if now-d.At <= probeAnsweredKeep {
			keep = append(keep, d)
		}
	}
	if dl.Checks > 0 {
		keep = append(keep, *dl)
	}
	if len(keep) > probeRecvMax {
		keep = keep[len(keep)-probeRecvMax:]
	}
	s.probeRecv = keep
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
		case answered(j) && now-j.Finished <= probeAnsweredKeep:
			p.State = "answered"
			if j.Status != JobSuccess {
				p.State, p.Note = "failed", j.Error
			}
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
		p.at = j.Finished
		out = append(out, p)
	}
	out = append(out, s.overdueChecks(now, local)...)
	if s.st.st.Settings.ProbeResultsShown {
		for _, d := range s.probeRecv {
			a := s.st.st.Agents[d.AgentID]
			if now-d.At > probeAnsweredKeep || a == nil || onMaster(a, local) {
				continue
			}
			out = append(out, probeRequest{Kind: "results", AgentID: a.ID, Agent: a.Name, Since: d.At, State: "received",
				Checks: d.Checks, Failed: d.Failed, Items: d.Items, at: d.At})
		}
	}
	rank := func(p probeRequest) int {
		switch p.State {
		case "answered", "failed", "received":
			return 1
		case "gave_up":
			return 2
		}
		return 0
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank(out[i]), rank(out[j])
		if ri != rj {
			return ri < rj // open requests first, then the answered ones, then those that gave up
		}
		if ri == 1 {
			return out[i].at > out[j].at // latest answer first
		}
		return out[i].Since < out[j].Since // oldest first
	})
	return out
}

// answered: the agent sent a result, good or bad (not a failure the master
// decided itself, such as a lost or discarded command).
func answered(j *Job) bool {
	switch j.Status {
	case JobSuccess:
		return true
	case JobFailed:
		return j.Started > 0 && !gaveUp(j) && !strings.HasPrefix(j.Error, "master restarted") &&
			!strings.HasPrefix(j.Error, "result discarded") && !strings.HasPrefix(j.Error, "result refused")
	}
	return false
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
	shown := s.st.st.Settings.ProbeResultsShown
	s.st.mu.Unlock()
	writeJSON(w, map[string]any{"now": now, "requests": reqs, "results_shown": shown})
}
