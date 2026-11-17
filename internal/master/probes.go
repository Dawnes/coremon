package master

// Remote Probes (Activity > Remote Probes): what the master is waiting
// for from agents on other systems. Requests the master handles itself
// (its agentless checks) and the agent on the master's own system are not
// shown. For now these are the agents' jobs; agentless checks done by
// remote agents will join later. A request leaves the list when it is
// answered; one that gave up stays 10 minutes.

import (
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

const probeGaveUpKeep = 10 * 60 // seconds a request that gave up stays listed

type probeRequest struct {
	*Job
	Agent string `json:"agent"`
	Since int64  `json:"since"` // requested at
	State string `json:"state"` // waiting, running, overdue, offline, gave_up
	Note  string `json:"note,omitempty"`
}

// localIPs: the master system's own addresses (not loopback).
func localIPs() map[string]bool {
	out := map[string]bool{}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() {
			out[n.IP.String()] = true
		}
	}
	return out
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
		p := probeRequest{Job: j, Agent: a.Name, Since: j.Created}
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
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		gi, gj := out[i].State == "gave_up", out[j].State == "gave_up"
		if gi != gj {
			return !gi // open requests first
		}
		return out[i].Since < out[j].Since // oldest first
	})
	return out
}

func (s *Server) handleProbes(w http.ResponseWriter, r *http.Request) {
	local := localIPs()
	now := time.Now().Unix()
	s.st.mu.Lock()
	reqs := s.probeRequests(now, local)
	s.st.mu.Unlock()
	writeJSON(w, map[string]any{"now": now, "requests": reqs})
}
