package master

import (
	"context"
	"encoding/json"
	"errors"
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

// jobAllowed: which jobs a system may receive in its status.
//   - pending: only an uninstall
//   - report only: no actions (checks, installs, reboots) except the
//     application scan, which only reads; agent version changes, mode
//     changes and uninstall are fine
//   - active: everything
func jobAllowed(a *Agent, j *Job) bool {
	if j.Type == proto.CmdSetPins || j.Type == proto.CmdPing {
		return true // key slots and ping: every status, even pending
	}
	switch a.Status {
	case StatusActive:
		return true
	case StatusReportOnly:
		return j.Type != proto.CmdAction || readOnlyAction(j.Feature)
	}
	return j.Type == proto.CmdUninstall
}

// readOnlyAction: a function that only reads, so it may run on report-only
// systems (the application scan's "Refresh now").
func readOnlyAction(feature string) bool { return feature == "apps" }

// syncMode sends the master's decision (active or report only) to an agent
// that supports report-only mode and has not confirmed it yet. actor is the
// user who switched, or "scheduler" for an automatic resync. Caller holds
// s.st.mu.
func (s *Server) syncMode(a *Agent, actor string) *Job {
	if a.Status == StatusPending || a.Removing {
		return nil
	}
	want := a.Status == StatusReportOnly
	if a.ReportOnly == want || s.openJob(a.ID, proto.CmdSetMode, "") {
		return nil
	}
	p, _ := json.Marshal(proto.ModeParams{ReportOnly: want})
	return s.enqueue(a, &Job{Type: proto.CmdSetMode, Params: p, Actor: actor, Timeout: 60})
}

// cancelQueued cancels an agent's queued actions (checks, installs,
// reboots) that have not started yet; running jobs finish normally and
// agent version changes stay. Caller holds s.st.mu.
func (s *Server) cancelQueued(a *Agent, reason string) int {
	n := 0
	now := time.Now().Unix()
	for _, j := range s.st.st.Jobs {
		if j.AgentID == a.ID && j.Status == JobQueued && j.Type == proto.CmdAction {
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

// ---- agent API logic (carried by the sealed method, sealed.go) ----

type apiError struct {
	code int
	msg  string
}

func (e *apiError) Error() string { return e.msg }

// enrollCore registers a new system with its own key. Caller must not hold
// s.st.mu.
func (s *Server) enrollCore(req proto.EnrollRequest, ip, agentKey string) (*proto.EnrollResponse, error) {
	if agentKey == "" {
		return nil, &apiError{http.StatusBadRequest, "agent key missing"}
	}
	now := time.Now()
	// An address over its limit of wrong tokens gets no further look.
	if s.enrolls.over(ip, now) {
		s.enrolls.rejected(ip, now) // counted for the summary
		return nil, &apiError{http.StatusTooManyRequests, "too many registration attempts; try again later"}
	}
	req.Facts = sanitizeFacts(req.Facts)
	req.CoreVersion = cleanText(req.CoreVersion, 32)
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	h := sha256hex(strings.TrimSpace(req.Token))
	tok := s.st.st.Tokens[h]
	if tok == nil || now.Unix() > tok.Expires {
		// Only the first attempt of an address is logged on its own; the
		// rest is summarised (see flushEnrolls).
		if first, _ := s.enrolls.rejected(ip, now); first {
			s.audit.write("agent", ip, "enroll.rejected", cleanText(req.Facts.Hostname, 64), "invalid or expired token")
		}
		return nil, &apiError{http.StatusForbidden, "invalid or expired enrollment token"}
	}
	// One system per address may wait for approval. The token is kept, so
	// the waiting agent registers by itself once the other one is approved
	// or removed.
	if ip != "" {
		for _, p := range s.st.st.Agents {
			if p.Status == StatusPending && p.EnrollIP == ip {
				if s.enrolls.noteWaiting(ip) {
					s.audit.write("agent", ip, "enroll.waiting", cleanText(req.Facts.Hostname, 64),
						"refused for now: "+p.Name+" from the same address is waiting for approval (approve or remove it first)")
				}
				return nil, &apiError{http.StatusTooManyRequests, "another system from this address is waiting for approval"}
			}
		}
		s.enrolls.clearWaiting(ip)
	}
	delete(s.st.st.Tokens, h)
	a := &Agent{
		EnrollIP:    ip,
		ID:          "a_" + randHex(8),
		Name:        cleanText(req.Facts.Hostname, maxSystemName),
		Status:      StatusPending,
		EnrolledAt:  time.Now().Unix(),
		LastSeen:    time.Now().Unix(),
		CoreVersion: req.CoreVersion,
		Facts:       req.Facts,
		Features:    []proto.FeatureInfo{},
		State:       map[string]json.RawMessage{},
		StateAt:     map[string]int64{},
		AgentKey:    agentKey,
	}
	resp := &proto.EnrollResponse{AgentID: a.ID, SigningKey: s.signingKeyB64}
	if tok.Note != "" && a.Name == "" {
		a.Name = tok.Note
	}
	a.Order = s.st.nextOrder("")
	s.st.st.Agents[a.ID] = a
	s.st.markDirty()
	s.audit.writeAgent("agent", ip, "agent.enrolled", a, fmt.Sprintf("token %s…", tok.Prefix))
	return resp, nil
}

// flushEnrolls writes one summary line per address for the registration
// attempts with a wrong token that followed the first (logged) one.
func (s *Server) flushEnrolls(now time.Time) {
	for ip, n := range s.enrolls.flush(now) {
		s.audit.write("agent", ip, "enroll.rejected", "", fmt.Sprintf("%d more rejected registration attempt(s) from this address in %d minutes", n, int(enrollWindow/time.Minute)))
	}
}

// pollCore records a heartbeat and waits up to one heartbeat for commands.
func (s *Server) pollCore(ctx context.Context, agentID string, req proto.PollRequest) (*proto.PollResponse, error) {
	s.st.mu.Lock()
	a := s.st.st.Agents[agentID]
	if a == nil {
		s.st.mu.Unlock()
		return nil, &apiError{http.StatusUnauthorized, "unauthorized"}
	}
	now := time.Now().Unix()
	a.LastSeen = now
	a.CoreVersion = cleanText(req.CoreVersion, 32)
	a.Protocol = req.Protocol
	if len(req.Running) > 256 {
		req.Running = req.Running[:256]
	}
	if a.DesiredCore != "" && a.DesiredCore == a.CoreVersion {
		a.DesiredCore = ""
	}
	// Key slots also below the minimum: a key rotation must not cut it off.
	a.Pins = [2]string{clip(req.Pins[0], 256), clip(req.Pins[1], 256)}
	a.PinsAt = now
	if s.belowMin(a) {
		// Upgrade only: just the platform (for the right update); none of its
		// data is stored. What was stored before stays, hidden, until updated.
		f := sanitizeFacts(proto.Facts{OS: req.Facts.OS, Arch: req.Facts.Arch})
		a.Facts.OS, a.Facts.Arch = f.OS, f.Arch
	} else {
		a.Facts = sanitizeFacts(req.Facts)
		// Agents before protocol 2 have no built-in features.
		a.Features = sanitizeFeatures(req.Features)
		if a.Features == nil {
			a.Features = []proto.FeatureInfo{}
		}
		a.ReportOnly = req.ReportOnly
		s.syncMode(a, "scheduler") // the master's decision wins; sent in this same reply
	}
	s.syncPins(a, "scheduler")
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
	hb := s.heartbeat()
	// At most one open heartbeat per agent: a new one ends the previous one,
	// as a reconnect would.
	if s.polls == nil {
		s.polls = map[string]chan struct{}{}
	}
	if old := s.polls[agentID]; old != nil {
		close(old)
	}
	mine := make(chan struct{})
	s.polls[agentID] = mine
	s.st.mu.Unlock()
	defer func() {
		s.st.mu.Lock()
		if s.polls[agentID] == mine {
			delete(s.polls, agentID)
		}
		s.st.mu.Unlock()
	}()

	deadline := time.NewTimer(time.Duration(hb) * time.Second)
	defer deadline.Stop()
	for {
		s.st.mu.Lock()
		cmds := s.takeCommands(agentID)
		s.st.mu.Unlock()
		if len(cmds) > 0 {
			return &proto.PollResponse{Commands: cmds, Heartbeat: hb}, nil
		}
		select {
		case <-ch:
		case <-mine: // replaced by a newer heartbeat
			return &proto.PollResponse{Commands: []proto.Command{}, Heartbeat: hb}, nil
		case <-deadline.C:
			return &proto.PollResponse{Commands: []proto.Command{}, Heartbeat: hb}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
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
		if !jobAllowed(a, j) || (s.belowMin(a) && !upgradeOnlyJob(j.Type)) {
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

// outputCore appends streamed job output.
func (s *Server) outputCore(agentID string, req proto.OutputBatch) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	a := s.st.st.Agents[agentID]
	if a == nil {
		return
	}
	upgradeOnly := s.belowMin(a)
	for _, it := range req.Items {
		j := s.st.jobByID(it.CommandID)
		if j == nil || j.AgentID != agentID || j.Status != JobRunning || j.LogSize >= maxJobLog {
			continue
		}
		if upgradeOnly && !upgradeOnlyJob(j.Type) {
			continue // below the minimum: only an agent update's output is kept
		}
		data := cleanLog(it.Data)
		if j.LogSize+int64(len(data)) > maxJobLog {
			data = clip(data, int(maxJobLog-j.LogSize)) + "\n[log truncated]\n"
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
}

// resultCore records a job's outcome.
func (s *Server) resultCore(agentID string, res proto.Result, ip string) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	a := s.st.st.Agents[agentID]
	j := s.st.jobByID(res.CommandID)
	if a == nil || j == nil || j.AgentID != a.ID || j.done() {
		return
	}
	now := time.Now().Unix()
	j.Finished = now
	if s.belowMin(a) && !upgradeOnlyJob(j.Type) {
		// E.g. a check that was running when the minimum was raised.
		j.Status, j.Error = JobFailed, "result discarded: the agent is older than the lowest allowed version"
		s.st.markDirty()
		return
	}
	if res.OK {
		j.Status = JobSuccess
	} else {
		j.Status, j.Error = JobFailed, clip(cleanLog(res.Error), maxResultErrLen)
	}
	switch {
	case j.Type == proto.CmdAction && len(res.Data) > 0:
		var d struct {
			State  json.RawMessage            `json:"state"`
			States map[string]json.RawMessage `json:"states"` // extra states, e.g. the application scan with a check
		}
		if json.Unmarshal(res.Data, &d) == nil && len(d.State) > 0 && string(d.State) != "null" {
			state, err := checkState(j.Feature, d.State)
			if err != nil {
				j.Status, j.Error = JobFailed, "result refused by the master: "+err.Error()
				break
			}
			a.State[j.Feature] = state
			a.StateAt[j.Feature] = now
		}
		for feat, raw := range d.States {
			if !extraStates[feat] || feat == j.Feature || len(raw) == 0 || string(raw) == "null" {
				continue
			}
			if state, err := checkState(feat, raw); err == nil {
				a.State[feat] = state
				a.StateAt[feat] = now
			}
		}
		if _, ok := a.StateAt["apps"]; ok && a.StateAt["apps"] == now {
			s.kickVulns() // new versions are looked up soon (not under this lock)
		}
	case j.Type == proto.CmdPing:
		s.pingAnswered(j, res)
	case j.Type == proto.CmdCoreUpdate && !res.OK:
		// Do not retry a failed up/downgrade on our own.
		a.DesiredCore = ""
	case j.Type == proto.CmdUninstall && res.OK:
		s.audit.writeAgent("agent", ip, "agent.uninstalled", a, "")
		delete(s.st.st.Agents, a.ID)
		delete(s.wakes, a.ID)
	}
	s.st.markDirty()
}

func writeAPIError(w http.ResponseWriter, err error) {
	var ae *apiError
	if errors.As(err, &ae) {
		http.Error(w, ae.msg, ae.code)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
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

// handlePublicAgent serves the latest agent binary to the install script.
// Binaries are not secret; integrity is enforced by the checksum embedded
// in the install script (whose own hash is in the install command).
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
	// Agent versions may change in report-only mode too.
	if (a.Status != StatusActive && a.Status != StatusReportOnly) || a.Removing || !s.online(a, time.Now().Unix()) || s.openJob(a.ID, proto.CmdCoreUpdate, "") {
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
		s.flushEnrolls(time.Now())
		s.sess.sweep()
		s.renewCertIfDue(time.Now())
		s.renewTicketKeys(time.Now())
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
		if a.Removing || !s.online(a, nowU) {
			continue
		}
		if !s.recentFailure(a.ID, proto.CmdSetPins, "", 2*time.Minute) {
			s.syncPins(a, "scheduler")
		}
		if a.Status == StatusPending {
			continue
		}
		// Requested agent version (set while the agent was offline).
		if a.DesiredCore != "" && a.CoreVersion != a.DesiredCore &&
			!s.recentFailure(a.ID, proto.CmdCoreUpdate, "", 10*time.Minute) {
			s.requestCore(a, a.DesiredCore, "scheduler")
		}
		if s.belowMin(a) {
			continue // upgrade only: no mode sync, no checks
		}
		if !s.recentFailure(a.ID, proto.CmdSetMode, "", 2*time.Minute) {
			s.syncMode(a, "scheduler")
		}
		if a.Status != StatusActive {
			continue // report only: no checks
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
