package master

// Maintenance of a device (agent or agentless): for a while nothing is
// checked or started automatically and no failures are shown.
//   - Agent: no automatic jobs (queued ones are cancelled); what it still
//     reports is stored. Manual actions stay possible, but only while the
//     agent is connected (they are not queued for later). An agent that is
//     a remote probe keeps checking its devices.
//   - Agentless: the ping and all services stop; a remote probe drops the
//     device's checks (its list is sent right away). At the end everything
//     is checked straight away and the probe gets its list back.
// It ends by hand or when its time is up; it survives a master restart.

import (
	"fmt"
	"net/http"
	"time"

	"coremon/internal/proto"
)

type Maintenance struct {
	Since int64  `json:"since"`
	Until int64  `json:"until,omitempty"` // 0: until ended by hand
	By    string `json:"by"`
}

// maintChoices: the durations offered, in minutes; 0 = until ended by hand.
var maintChoices = []int{5, 30, 60, 240, 0}

func maintText(minutes int, until int64) string {
	if minutes == 0 {
		return "until ended by hand"
	}
	return fmt.Sprintf("for %s (until %s)", durText(minutes), time.Unix(until, 0).Format("15:04"))
}

func durText(m int) string {
	if m%60 == 0 {
		return plural(m/60, "hour")
	}
	return plural(m, "minute")
}

// maintOffline: during maintenance agent actions need a connected agent.
// Caller holds s.st.mu.
func (s *Server) maintOffline(a *Agent) bool {
	return a.Maint != nil && !s.online(a, time.Now().Unix())
}

const maintOfflineMsg = ": in maintenance and the agent is not connected; actions are not queued for later"

func (s *Server) handleMaintStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Minutes int `json:"minutes"`
	}
	if !readJSON(w, r, &req, 256) {
		return
	}
	if !containsInt(maintChoices, req.Minutes) {
		http.Error(w, "duration must be 5, 30, 60 or 240 minutes, or until ended by hand", http.StatusBadRequest)
		return
	}
	now := time.Now()
	m := &Maintenance{Since: now.Unix(), By: actor(r)}
	if req.Minutes > 0 {
		m.Until = now.Add(time.Duration(req.Minutes) * time.Minute).Unix()
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	id := r.PathValue("id")
	detail := maintText(req.Minutes, m.Until)
	switch {
	case s.st.st.Agents[id] != nil:
		a := s.st.st.Agents[id]
		if a.Status == StatusPending {
			http.Error(w, a.Name+" is waiting for approval", http.StatusConflict)
			return
		}
		a.Maint = m
		// Automatic jobs still waiting are cancelled; the user's own stay.
		for _, j := range s.st.st.Jobs {
			if j.AgentID == a.ID && j.Status == JobQueued && j.Actor == "scheduler" && j.Type == proto.CmdAction {
				j.Status, j.Finished, j.Error = JobCancelled, now.Unix(), "cancelled: device in maintenance"
				s.st.markJobs()
			}
		}
		s.st.markDirty()
		s.audit.writeAgent(actor(r), clientIP(r), "device.maintenance_started", a, detail)
		writeJSON(w, map[string]any{"maint": a.Maint})
	case s.st.st.Locals[id] != nil:
		d := s.st.st.Locals[id]
		d.Maint = m
		s.st.markDirty()
		s.writeLocalAudit(r, "device.maintenance_started", d, detail) // also sends a probe its new list
		writeJSON(w, map[string]any{"maint": d.Maint})
	default:
		http.Error(w, "device not found", http.StatusNotFound)
	}
}

func (s *Server) handleMaintEnd(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	id := r.PathValue("id")
	if s.st.st.Agents[id] == nil && s.st.st.Locals[id] == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	s.endMaint(id, actor(r), clientIP(r), "", time.Now())
	writeJSON(w, map[string]any{"ok": true})
}

// endMaint ends a device's maintenance and restarts its checks right away.
// Caller holds s.st.mu.
func (s *Server) endMaint(id, who, ip, detail string, now time.Time) {
	if a := s.st.st.Agents[id]; a != nil {
		if a.Maint == nil {
			return
		}
		a.Maint = nil
		s.st.markDirty()
		s.audit.writeAgent(who, ip, "device.maintenance_ended", a, detail)
		return
	}
	d := s.st.st.Locals[id]
	if d == nil || d.Maint == nil {
		return
	}
	d.Maint = nil
	s.st.markDirty()
	if d.Probe == "" {
		s.loc.mu.Lock()
		if d.pinging() {
			s.loc.next[d.ID] = now
		}
		for _, sv := range d.Services {
			s.loc.next[sv.ID] = now
		}
		s.loc.mu.Unlock()
	}
	s.audit.add(auditEntry{Actor: who, IP: ip, Action: "device.maintenance_ended", Agent: d.ID, Target: d.Name, Detail: detail})
	s.probeSyncAll(now) // the probe gets the device's checks back
}

// maintExpire ends maintenance whose time is up. Caller holds s.st.mu.
func (s *Server) maintExpire(now time.Time) {
	t := now.Unix()
	for id, a := range s.st.st.Agents {
		if a.Maint != nil && a.Maint.Until > 0 && t >= a.Maint.Until {
			s.endMaint(id, "scheduler", "", "time is up", now)
		}
	}
	for id, d := range s.st.st.Locals {
		if d.Maint != nil && d.Maint.Until > 0 && t >= d.Maint.Until {
			s.endMaint(id, "scheduler", "", "time is up", now)
		}
	}
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
