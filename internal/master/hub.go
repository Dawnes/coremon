package master

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"coremon/internal/proto"
)

const (
	onlineWindow = 90 // seconds since last poll
	pollHold     = 25 * time.Second
	maxJobLog    = 20 << 20
)

func (a *Agent) online(now int64) bool { return now-a.LastSeen < onlineWindow }

// authAgent validates "Authorization: Bearer <id>:<secret>". Caller must
// hold s.st.mu.
func (s *Server) authAgent(r *http.Request) *Agent {
	h := r.Header.Get("Authorization")
	tok, ok := strings.CutPrefix(h, "Bearer ")
	if !ok {
		return nil
	}
	id, secret, ok := strings.Cut(tok, ":")
	if !ok {
		return nil
	}
	a := s.st.st.Agents[id]
	if a == nil {
		return nil
	}
	if subtle.ConstantTimeCompare([]byte(a.SecretHash), []byte(sha256hex(secret))) != 1 {
		return nil
	}
	return a
}

func readJSON(w http.ResponseWriter, r *http.Request, v any, max int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

// wake interrupts a waiting long-poll for the agent. Caller holds s.st.mu.
func (s *Server) wake(agentID string) {
	ch := s.wakes[agentID]
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

// enqueue adds a job for an agent. Caller holds s.st.mu.
func (s *Server) enqueue(a *Agent, j *Job) *Job {
	j.ID = "j_" + randHex(8)
	j.AgentID = a.ID
	j.Status = JobQueued
	j.Created = time.Now().Unix()
	if j.Timeout <= 0 {
		j.Timeout = 600
	}
	s.st.addJob(j)
	s.wake(a.ID)
	return j
}

// openJob reports whether a queued or running job matches.
func (s *Server) openJob(agentID, typ, module string) bool {
	for i := len(s.st.st.Jobs) - 1; i >= 0; i-- {
		j := s.st.st.Jobs[i]
		if j.AgentID == agentID && j.Type == typ && j.Module == module && !j.done() {
			return true
		}
	}
	return false
}

// recentFailure reports whether a matching job failed within d.
func (s *Server) recentFailure(agentID, typ, module string, d time.Duration) bool {
	cut := time.Now().Add(-d).Unix()
	for i := len(s.st.st.Jobs) - 1; i >= 0; i-- {
		j := s.st.st.Jobs[i]
		if j.Created < cut {
			break
		}
		if j.AgentID == agentID && j.Type == typ && j.Module == module && j.done() && j.Status != JobSuccess {
			return true
		}
	}
	return false
}

// recentJob reports whether any matching job was created within d. Guards
// against acting on a module list that predates a just-finished job.
func (s *Server) recentJob(agentID, typ, module string, d time.Duration) bool {
	cut := time.Now().Add(-d).Unix()
	for i := len(s.st.st.Jobs) - 1; i >= 0; i-- {
		j := s.st.st.Jobs[i]
		if j.Created < cut {
			break
		}
		if j.AgentID == agentID && j.Type == typ && j.Module == module {
			return true
		}
	}
	return false
}

func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	var req proto.EnrollRequest
	if !readJSON(w, r, &req, 64<<10) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	h := sha256hex(strings.TrimSpace(req.Token))
	tok := s.st.st.Tokens[h]
	if tok == nil || time.Now().Unix() > tok.Expires {
		s.audit.write("agent", clientIP(r), "enroll.rejected", req.Facts.Hostname, "invalid or expired token")
		http.Error(w, "invalid or expired enrollment token", http.StatusForbidden)
		return
	}
	delete(s.st.st.Tokens, h)
	secret := randHex(32)
	a := &Agent{
		ID:             "a_" + randHex(8),
		Name:           req.Facts.Hostname,
		Tags:           []string{},
		Status:         "pending",
		SecretHash:     sha256hex(secret),
		EnrolledAt:     time.Now().Unix(),
		LastSeen:       time.Now().Unix(),
		CoreVersion:    req.CoreVersion,
		Facts:          req.Facts,
		Modules:        []proto.ModuleInfo{},
		DesiredModules: map[string]string{},
		State:          map[string]json.RawMessage{},
		StateAt:        map[string]int64{},
	}
	if tok.Note != "" && a.Name == "" {
		a.Name = tok.Note
	}
	s.st.st.Agents[a.ID] = a
	s.st.markDirty()
	s.audit.write("agent", clientIP(r), "agent.enrolled", a.ID, fmt.Sprintf("%s (token %s…)", a.Name, tok.Prefix))
	writeJSON(w, proto.EnrollResponse{AgentID: a.ID, Secret: secret, SigningKey: s.signingKeyB64})
}

func (s *Server) handlePoll(w http.ResponseWriter, r *http.Request) {
	var req proto.PollRequest
	if !readJSON(w, r, &req, 8<<20) {
		return
	}
	s.st.mu.Lock()
	a := s.authAgent(r)
	if a == nil {
		s.st.mu.Unlock()
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	now := time.Now().Unix()
	a.LastSeen = now
	a.Facts = req.Facts
	a.CoreVersion = req.CoreVersion
	if req.Modules != nil {
		a.Modules = req.Modules
	}
	// Commands the agent no longer runs but never reported: lost (e.g. the
	// agent restarted mid-command).
	running := map[string]bool{}
	for _, id := range req.Running {
		running[id] = true
	}
	for _, j := range s.st.st.Jobs {
		if j.AgentID == a.ID && j.Status == JobRunning && !running[j.ID] && now-j.Started > 45 {
			j.Status, j.Finished, j.Error = JobFailed, now, "agent lost this command (restarted?)"
		}
	}
	s.st.markDirty()
	ch := s.wakes[a.ID]
	if ch == nil {
		ch = make(chan struct{}, 1)
		s.wakes[a.ID] = ch
	}
	agentID := a.ID
	s.st.mu.Unlock()

	deadline := time.NewTimer(pollHold)
	defer deadline.Stop()
	for {
		s.st.mu.Lock()
		cmds := s.takeCommands(agentID)
		s.st.mu.Unlock()
		if len(cmds) > 0 {
			writeJSON(w, proto.PollResponse{Commands: cmds})
			return
		}
		select {
		case <-ch:
		case <-deadline.C:
			writeJSON(w, proto.PollResponse{Commands: []proto.Command{}})
			return
		case <-r.Context().Done():
			return
		}
	}
}

// takeCommands moves queued jobs to running and returns them as commands.
// Caller holds s.st.mu.
func (s *Server) takeCommands(agentID string) []proto.Command {
	a := s.st.st.Agents[agentID]
	if a == nil {
		return nil
	}
	var out []proto.Command
	now := time.Now().Unix()
	for _, j := range s.st.st.Jobs {
		if j.AgentID != agentID || j.Status != JobQueued {
			continue
		}
		// Paused/pending agents only receive an uninstall.
		if a.Status != "active" && j.Type != proto.CmdUninstall {
			continue
		}
		j.Status, j.Started = JobRunning, now
		out = append(out, proto.Command{
			ID: j.ID, Type: j.Type, Module: j.Module, Action: j.Action,
			Params: j.Params, Artifact: j.Artifact, Timeout: j.Timeout,
		})
	}
	if len(out) > 0 {
		s.st.markDirty()
	}
	return out
}

func (s *Server) handleOutput(w http.ResponseWriter, r *http.Request) {
	var req proto.OutputBatch
	if !readJSON(w, r, &req, 8<<20) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	a := s.authAgent(r)
	if a == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	for _, it := range req.Items {
		j := s.st.jobByID(it.CommandID)
		if j == nil || j.AgentID != a.ID || j.Status != JobRunning || j.LogSize >= maxJobLog {
			continue
		}
		data := it.Data
		if j.LogSize+int64(len(data)) > maxJobLog {
			data = data[:maxJobLog-j.LogSize] + "\n[log truncated]\n"
		}
		f, err := os.OpenFile(s.st.jobLogPath(j.ID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			continue
		}
		n, _ := f.WriteString(data)
		f.Close()
		j.LogSize += int64(n)
	}
	s.st.markDirty()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResult(w http.ResponseWriter, r *http.Request) {
	var res proto.Result
	if !readJSON(w, r, &res, 8<<20) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	a := s.authAgent(r)
	if a == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	j := s.st.jobByID(res.CommandID)
	if j == nil || j.AgentID != a.ID {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if j.done() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	now := time.Now().Unix()
	j.Finished = now
	if res.OK {
		j.Status = JobSuccess
	} else {
		j.Status, j.Error = JobFailed, res.Error
	}
	if j.Type == proto.CmdModuleCall && len(res.Data) > 0 {
		var d struct {
			State json.RawMessage `json:"state"`
		}
		if json.Unmarshal(res.Data, &d) == nil && len(d.State) > 0 && string(d.State) != "null" {
			a.State[j.Module] = d.State
			a.StateAt[j.Module] = now
		}
	}
	if j.Type == proto.CmdModuleInstall && res.OK && len(res.Data) > 0 {
		var mi proto.ModuleInfo
		if json.Unmarshal(res.Data, &mi) == nil && mi.Name == j.Module {
			if cur := a.module(mi.Name); cur != nil {
				*cur = mi
			} else {
				a.Modules = append(a.Modules, mi)
			}
			go s.schedulerTick() // e.g. run the first check immediately
		}
	}
	if j.Type == proto.CmdModuleRemove && res.OK {
		for i, m := range a.Modules {
			if m.Name == j.Module {
				a.Modules = append(a.Modules[:i], a.Modules[i+1:]...)
				break
			}
		}
	}
	if j.Type == proto.CmdUninstall && res.OK {
		s.audit.write("agent", clientIP(r), "agent.uninstalled", a.ID, a.Name)
		delete(s.st.st.Agents, a.ID)
		delete(s.wakes, a.ID)
	}
	s.st.markDirty()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serveArtifact(w http.ResponseWriter, r *http.Request, e *artifactEntry) {
	if e == nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(e.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprint(e.Size))
	w.Header().Set("X-Checksum-Sha256", e.SHA256)
	io.Copy(w, f)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	a := s.authAgent(r)
	s.st.mu.Unlock()
	if a == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	s.serveArtifact(w, r, s.repo.find(r.PathValue("kind"), r.PathValue("name"), r.PathValue("version"), r.PathValue("os"), r.PathValue("arch")))
}

// handlePublicAgent serves the latest agent-core binary to the install
// script. Binaries are not secret; integrity is enforced by the checksum
// embedded in the (pinned-TLS) install script.
func (s *Server) handlePublicAgent(w http.ResponseWriter, r *http.Request) {
	goos, arch, ok := strings.Cut(r.PathValue("plat"), "-")
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.serveArtifact(w, r, s.repo.find("core", "coremon-agent", s.repo.coreLatest(), goos, arch))
}

// scheduler reconciles desired modules/core, triggers periodic checks and
// expires stale jobs.
func (s *Server) schedulerLoop(stop <-chan struct{}) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		s.schedulerTick()
		select {
		case <-stop:
			return
		case <-t.C:
		}
	}
}

func (s *Server) schedulerTick() {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	now := time.Now()
	nowU := now.Unix()
	interval := int64(s.st.st.Settings.CheckIntervalHours) * 3600

	for _, a := range s.st.st.Agents {
		if a.Status != "active" || a.Removing || !a.online(nowU) {
			continue
		}
		goos, arch := a.Facts.OS, a.Facts.Arch
		if goos == "" {
			goos = "linux"
		}
		// Desired modules.
		for name, want := range a.DesiredModules {
			if want == "" {
				want = s.repo.moduleLatest(name)
			}
			have := a.module(name)
			if have != nil && have.Version == want {
				continue
			}
			if s.openJob(a.ID, proto.CmdModuleInstall, name) || s.recentFailure(a.ID, proto.CmdModuleInstall, name, 10*time.Minute) ||
				s.recentJob(a.ID, proto.CmdModuleInstall, name, 2*time.Minute) {
				continue
			}
			e := s.repo.find("module", name, want, goos, arch)
			if e == nil {
				continue
			}
			art := e.Artifact
			s.enqueue(a, &Job{Type: proto.CmdModuleInstall, Module: name, Artifact: &art, Actor: "scheduler", Timeout: 900})
		}
		for _, m := range a.Modules {
			if _, ok := a.DesiredModules[m.Name]; !ok && !s.openJob(a.ID, proto.CmdModuleRemove, m.Name) && !s.recentFailure(a.ID, proto.CmdModuleRemove, m.Name, 10*time.Minute) &&
				!s.recentJob(a.ID, proto.CmdModuleRemove, m.Name, 2*time.Minute) {
				s.enqueue(a, &Job{Type: proto.CmdModuleRemove, Module: m.Name, Actor: "scheduler", Timeout: 120})
			}
		}
		// Desired core version.
		if a.DesiredCore != "" && a.CoreVersion != a.DesiredCore &&
			!s.openJob(a.ID, proto.CmdCoreUpdate, "") && !s.recentFailure(a.ID, proto.CmdCoreUpdate, "", 10*time.Minute) {
			if e := s.repo.find("core", "coremon-agent", a.DesiredCore, goos, arch); e != nil {
				art := e.Artifact
				s.enqueue(a, &Job{Type: proto.CmdCoreUpdate, Artifact: &art, Actor: "scheduler", Timeout: 900})
			}
		}
		// Periodic checks: any module offering a "check" action.
		for _, m := range a.Modules {
			spec := m.Manifest.Action("check")
			if spec == nil || nowU-a.StateAt[m.Name] < interval {
				continue
			}
			if s.openJob(a.ID, proto.CmdModuleCall, m.Name) || s.recentFailure(a.ID, proto.CmdModuleCall, m.Name, 15*time.Minute) {
				continue
			}
			s.enqueue(a, &Job{Type: proto.CmdModuleCall, Module: m.Name, Action: "check", Params: json.RawMessage("{}"), Actor: "scheduler", Timeout: spec.Timeout})
		}
	}
	// Expire jobs.
	for _, j := range s.st.st.Jobs {
		switch {
		case j.Status == JobRunning && nowU > j.Started+int64(j.Timeout)+120:
			j.Status, j.Finished, j.Error = JobTimeout, nowU, "no result from agent within timeout"
			s.st.markDirty()
		case j.Status == JobQueued && nowU > j.Created+7*24*3600:
			j.Status, j.Finished, j.Error = JobFailed, nowU, "expired: agent never picked it up"
			s.st.markDirty()
		}
	}
	// Expire enrollment tokens.
	for h, t := range s.st.st.Tokens {
		if nowU > t.Expires {
			delete(s.st.st.Tokens, h)
			s.st.markDirty()
		}
	}
}
