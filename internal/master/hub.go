package master

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"coremon/internal/proto"
)

const maxJobLog = 20 << 20

// heartbeat returns the configured heartbeat in seconds. Caller holds s.st.mu.
func (s *Server) heartbeat() int { return s.st.st.Settings.HeartbeatSec }

// online: an agent is online while it was heard from within three
// heartbeats. Caller holds s.st.mu.
func (s *Server) online(a *Agent, now int64) bool {
	return now-a.LastSeen < int64(3*s.heartbeat())
}

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

// cancelQueued cancels an agent's jobs that have not started yet; running
// jobs finish normally. Caller holds s.st.mu.
func (s *Server) cancelQueued(a *Agent, reason string) int {
	n := 0
	now := time.Now().Unix()
	for _, j := range s.st.st.Jobs {
		if j.AgentID == a.ID && j.Status == JobQueued {
			j.Status, j.Finished, j.Error = JobCancelled, now, "cancelled: "+reason
			n++
		}
	}
	if n > 0 {
		s.st.markDirty()
	}
	return n
}

// openJob reports whether a queued or running job matches.
func (s *Server) openJob(agentID, typ, feature string) bool {
	for i := len(s.st.st.Jobs) - 1; i >= 0; i-- {
		j := s.st.st.Jobs[i]
		if j.AgentID == agentID && j.Type == typ && j.Feature == feature && !j.done() {
			return true
		}
	}
	return false
}

// recentFailure reports whether a matching job failed within d.
func (s *Server) recentFailure(agentID, typ, feature string, d time.Duration) bool {
	cut := time.Now().Add(-d).Unix()
	for i := len(s.st.st.Jobs) - 1; i >= 0; i-- {
		j := s.st.st.Jobs[i]
		if j.Created < cut {
			break
		}
		if j.AgentID == agentID && j.Type == typ && j.Feature == feature && j.done() && j.Status != JobSuccess {
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
		ID:          "a_" + randHex(8),
		Name:        req.Facts.Hostname,
		Status:      "pending",
		SecretHash:  sha256hex(secret),
		EnrolledAt:  time.Now().Unix(),
		LastSeen:    time.Now().Unix(),
		CoreVersion: req.CoreVersion,
		Facts:       req.Facts,
		Features:    []proto.FeatureInfo{},
		State:       map[string]json.RawMessage{},
		StateAt:     map[string]int64{},
	}
	if tok.Note != "" && a.Name == "" {
		a.Name = tok.Note
	}
	a.Order = s.st.nextOrder("")
	s.st.st.Agents[a.ID] = a
	s.st.markDirty()
	s.audit.writeAgent("agent", clientIP(r), "agent.enrolled", a, fmt.Sprintf("token %s…", tok.Prefix))
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
	a.Protocol = req.Protocol
	// Agents before protocol 2 have no built-in features.
	a.Features = req.Features
	if a.Features == nil {
		a.Features = []proto.FeatureInfo{}
	}
	if a.DesiredCore != "" && a.DesiredCore == a.CoreVersion {
		a.DesiredCore = ""
	}
	// Commands the agent no longer runs but never reported: lost (e.g. the
	// agent restarted mid-command).
	running := map[string]bool{}
	for _, id := range req.Running {
		running[id] = true
	}
	for _, j := range s.st.st.Jobs {
		if j.AgentID == a.ID && j.Status == JobRunning && !running[j.ID] && now-j.Started > 45 {
			if j.Type == proto.CmdCoreUpdate && j.Artifact != nil && a.CoreVersion == j.Artifact.Version {
				// The agent restarted into the new version before its result
				// reached us.
				j.Status, j.Finished = JobSuccess, now
				continue
			}
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
	hb := s.heartbeat()
	s.st.mu.Unlock()

	deadline := time.NewTimer(time.Duration(hb) * time.Second)
	defer deadline.Stop()
	for {
		s.st.mu.Lock()
		cmds := s.takeCommands(agentID)
		s.st.mu.Unlock()
		if len(cmds) > 0 {
			writeJSON(w, proto.PollResponse{Commands: cmds, Heartbeat: hb})
			return
		}
		select {
		case <-ch:
		case <-deadline.C:
			writeJSON(w, proto.PollResponse{Commands: []proto.Command{}, Heartbeat: hb})
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
			ID: j.ID, Type: j.Type, Feature: j.Feature, Action: j.Action,
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
	if j == nil || j.AgentID != a.ID || j.done() {
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
	switch {
	case j.Type == proto.CmdAction && len(res.Data) > 0:
		var d struct {
			State json.RawMessage `json:"state"`
		}
		if json.Unmarshal(res.Data, &d) == nil && len(d.State) > 0 && string(d.State) != "null" {
			a.State[j.Feature] = d.State
			a.StateAt[j.Feature] = now
		}
	case j.Type == proto.CmdCoreUpdate && !res.OK:
		// Do not retry a failed up/downgrade on our own.
		a.DesiredCore = ""
	case j.Type == proto.CmdUninstall && res.OK:
		s.audit.writeAgent("agent", clientIP(r), "agent.uninstalled", a, "")
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
	if r.PathValue("kind") != "core" {
		http.NotFound(w, r)
		return
	}
	s.serveArtifact(w, r, s.repo.find("core", r.PathValue("version"), r.PathValue("os"), r.PathValue("arch")))
}

// handlePublicAgent serves the latest agent binary to the install script.
// Binaries are not secret; integrity is enforced by the checksum embedded
// in the (pinned-TLS) install script.
func (s *Server) handlePublicAgent(w http.ResponseWriter, r *http.Request) {
	goos, arch, ok := strings.Cut(r.PathValue("plat"), "-")
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.serveArtifact(w, r, s.repo.find("core", s.repo.coreLatest(), goos, arch))
}

// requestCore queues an agent up/downgrade to version if possible. Caller
// holds s.st.mu.
func (s *Server) requestCore(a *Agent, version, actor string) *Job {
	if a.Status != "active" || a.Removing || !s.online(a, time.Now().Unix()) || s.openJob(a.ID, proto.CmdCoreUpdate, "") {
		return nil
	}
	goos, arch := a.Facts.OS, a.Facts.Arch
	if goos == "" {
		goos = "linux"
	}
	e := s.repo.find("core", version, goos, arch)
	if e == nil {
		return nil
	}
	art := e.Artifact
	return s.enqueue(a, &Job{Type: proto.CmdCoreUpdate, Artifact: &art, Actor: actor, Timeout: 900})
}

// scheduler applies requested agent versions, triggers periodic checks,
// expires stale jobs and purges old history.
func (s *Server) schedulerLoop(stop <-chan struct{}) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for {
		s.schedulerTick()
		if due := s.nextPurgeAt(); !time.Now().Before(due) {
			s.purge()
			s.setNextPurge(nextFullHour(time.Now()))
		}
		select {
		case <-stop:
			return
		case <-t.C:
		}
	}
}

// The history cleanup runs on every full hour.
const purgeMinNotice = 3 * time.Minute

func nextFullHour(t time.Time) time.Time { return t.Truncate(time.Hour).Add(time.Hour) }

// purgeAfterChange is when a changed retention period takes effect: the
// next full hour, unless that is less than three minutes away.
func purgeAfterChange(now time.Time) time.Time {
	n := nextFullHour(now)
	if n.Sub(now) < purgeMinNotice {
		n = n.Add(time.Hour)
	}
	return n
}

func (s *Server) nextPurgeAt() time.Time {
	s.purgeMu.Lock()
	defer s.purgeMu.Unlock()
	if s.nextPurge.IsZero() {
		s.nextPurge = nextFullHour(time.Now())
	}
	return s.nextPurge
}

func (s *Server) setNextPurge(t time.Time) {
	s.purgeMu.Lock()
	s.nextPurge = t
	s.purgeMu.Unlock()
}

func (s *Server) schedulerTick() {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	nowU := time.Now().Unix()
	interval := int64(s.st.st.Settings.CheckIntervalHours) * 3600

	for _, a := range s.st.st.Agents {
		if a.Status != "active" || a.Removing || !s.online(a, nowU) {
			continue
		}
		// Requested agent version (set while the agent was offline).
		if a.DesiredCore != "" && a.CoreVersion != a.DesiredCore &&
			!s.recentFailure(a.ID, proto.CmdCoreUpdate, "", 10*time.Minute) {
			s.requestCore(a, a.DesiredCore, "scheduler")
		}
		// Periodic checks: any feature offering a "check" action.
		for i := range a.Features {
			f := &a.Features[i]
			spec := f.Action("check")
			if spec == nil || nowU-a.StateAt[f.Name] < interval {
				continue
			}
			if s.openJob(a.ID, proto.CmdAction, f.Name) || s.recentFailure(a.ID, proto.CmdAction, f.Name, 15*time.Minute) {
				continue
			}
			s.enqueue(a, &Job{Type: proto.CmdAction, Feature: f.Name, Action: "check", Params: json.RawMessage("{}"), Actor: "scheduler", Timeout: spec.Timeout})
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

// retentionCutoff returns the oldest time kept at moment t.
func retentionCutoff(t time.Time, days int) int64 {
	return t.Add(-time.Duration(days) * 24 * time.Hour).Unix()
}

// purgePreview counts what a cleanup at time at with the given retention
// would remove.
func (s *Server) purgePreview(days int, at time.Time) (cutoff int64, audit, jobs int) {
	cutoff = retentionCutoff(at, days)
	s.st.mu.Lock()
	for _, j := range s.st.st.Jobs {
		end := j.Finished
		if end == 0 {
			end = j.Created
		}
		if j.done() && end < cutoff {
			jobs++
		}
	}
	s.st.mu.Unlock()
	return cutoff, s.audit.countOlder(cutoff), jobs
}

// purge removes audit entries, jobs and job output older than the
// retention period.
func (s *Server) purge() {
	s.st.mu.Lock()
	days := s.st.st.Settings.RetentionDays
	cutoff := retentionCutoff(time.Now(), days)
	jobs := s.st.purgeJobs(cutoff)
	s.st.mu.Unlock()
	entries, err := s.audit.purge(cutoff)
	if err != nil {
		log.Printf("audit purge: %v", err)
	}
	if jobs > 0 || entries > 0 {
		log.Printf("retention (%d days): removed %d job(s) and %d audit entries", days, jobs, entries)
	}
}
