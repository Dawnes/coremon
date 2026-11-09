package master

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"coremon/internal/feature/updates"
	"coremon/internal/proto"
)

const (
	minPasswordLen = 5
	maxPasswordLen = 40 // characters; checked when a password is set
	maxTokenHours  = 3  // validity of an install command
)

// ---- views ----

type updatesSummary struct {
	Total          int   `json:"total"`
	Security       int   `json:"security"`
	RebootRequired bool  `json:"reboot_required"`
	CheckedAt      int64 `json:"checked_at"`
}

type agentView struct {
	ID           string                     `json:"id"`
	Name         string                     `json:"name"`
	GroupID      string                     `json:"group_id"`
	Order        int                        `json:"order"`
	Status       string                     `json:"status"`
	Online       bool                       `json:"online"`
	LastSeen     int64                      `json:"last_seen"`
	EnrolledAt   int64                      `json:"enrolled_at"`
	CoreVersion  string                     `json:"core_version"`
	CoreLatest   string                     `json:"core_latest"`
	CoreIsLatest bool                       `json:"core_is_latest"`
	DesiredCore  string                     `json:"desired_core"`
	Legacy       bool                       `json:"legacy"`            // agent before 0.3.0: old connection method, cannot connect
	IsMaster     bool                       `json:"is_master"`         // the system the master itself runs on
	ModeSupport  bool                       `json:"mode_support"`      // agent enforces report-only itself (protocol 3+)
	AgentReport  bool                       `json:"agent_report_only"` // mode the agent itself confirmed
	Sealed       bool                       `json:"sealed"`            // uses the payload key (0.3.0+)
	OSSupport    *osSupport                 `json:"os_support,omitempty"`
	Facts        proto.Facts                `json:"facts"`
	Features     []string                   `json:"features"`
	Updates      *updatesSummary            `json:"updates,omitempty"`
	Busy         int                        `json:"busy"` // open jobs
	Removing     bool                       `json:"removing"`
	State        map[string]json.RawMessage `json:"state,omitempty"`
	StateAt      map[string]int64           `json:"state_at,omitempty"`
}

// view builds the UI representation. Caller holds s.st.mu.
func (s *Server) view(a *Agent, full bool) agentView {
	now := time.Now().Unix()
	v := agentView{
		ID: a.ID, Name: a.Name, GroupID: a.GroupID, Order: a.Order, Status: a.Status, Online: s.online(a, now),
		LastSeen: a.LastSeen, EnrolledAt: a.EnrolledAt, CoreVersion: a.CoreVersion,
		Facts: a.Facts, Removing: a.Removing, DesiredCore: a.DesiredCore, Features: []string{},
	}
	v.CoreLatest = s.repo.coreLatest()
	v.CoreIsLatest = v.CoreLatest != "" && a.CoreVersion == v.CoreLatest
	v.Legacy = a.CoreVersion != "" && versionLess(a.CoreVersion, minAgentVersion)
	v.ModeSupport, v.AgentReport = a.Protocol >= 3, a.ReportOnly
	v.Sealed = a.Protocol >= 5 || a.AgentKey != ""
	v.OSSupport = s.supportFor(a.Facts.OSID, a.Facts.OSVersion, time.Now())
	for _, f := range a.Features {
		v.Features = append(v.Features, f.Name)
	}
	own := s.ownIPs()
	for _, ip := range a.Facts.IPs {
		if own[ip] {
			v.IsMaster = true
		}
	}
	if raw, ok := a.State["updates"]; ok {
		var u updatesSummary
		if json.Unmarshal(raw, &u) == nil {
			v.Updates = &u
		}
	}
	for _, j := range s.st.st.Jobs {
		if j.AgentID == a.ID && !j.done() {
			v.Busy++
		}
	}
	if full {
		v.State, v.StateAt = a.State, a.StateAt
	}
	return v
}

// orderedAgents returns agents in display order: by group (in group order,
// ungrouped last), then by position. Caller holds s.st.mu.
func (s *Server) orderedAgents() []*Agent {
	gpos := map[string]int{"": len(s.st.st.Groups)}
	for i, g := range s.st.st.Groups {
		gpos[g.ID] = i
	}
	out := make([]*Agent, 0, len(s.st.st.Agents))
	for _, a := range s.st.st.Agents {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if gpos[a.GroupID] != gpos[b.GroupID] {
			return gpos[a.GroupID] < gpos[b.GroupID]
		}
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return out
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	agents := []agentView{}
	for _, a := range s.orderedAgents() {
		agents = append(agents, s.view(a, false))
	}
	active := 0
	for _, j := range s.st.st.Jobs {
		if !j.done() {
			active++
		}
	}
	writeJSON(w, map[string]any{
		"agents": agents, "groups": s.st.st.Groups, "active_jobs": active, "server_time": time.Now().Unix(),
		"settings": s.st.st.Settings, "repo": s.repo.summary(),
	})
}

func (s *Server) agentOr404(w http.ResponseWriter, r *http.Request) *Agent {
	a := s.st.st.Agents[r.PathValue("id")]
	if a == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
	}
	return a
}

func (s *Server) jobsFor(agentID string, limit int, activeOnly bool) []*Job {
	return s.jobsFiltered(agentID, "", limit, activeOnly)
}

// jobsFiltered returns the newest matching jobs; filters are applied before
// the limit. actor is a user name or "scheduler".
func (s *Server) jobsFiltered(agentID, actor string, limit int, activeOnly bool) []*Job {
	out := []*Job{}
	for i := len(s.st.st.Jobs) - 1; i >= 0 && len(out) < limit; i-- {
		j := s.st.st.Jobs[i]
		if (agentID == "" || j.AgentID == agentID) && (actor == "" || strings.EqualFold(j.Actor, actor)) && (!activeOnly || !j.done()) {
			out = append(out, j)
		}
	}
	return out
}

func (s *Server) handleAgentGet(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	a := s.agentOr404(w, r)
	if a == nil {
		return
	}
	writeJSON(w, map[string]any{"agent": s.view(a, true), "jobs": s.jobsFor(a.ID, 50, false),
		"repo": s.repo.summary(), "groups": s.st.st.Groups})
}

var actionNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

const maxActionPackages = 5000

// checkActionParams validates an action request before it goes to agents:
// plain names, a JSON object as parameters and, for package installs, only
// names matching the agent's own package pattern.
func checkActionParams(feature, action string, params json.RawMessage) error {
	if !actionNameRe.MatchString(feature) || !actionNameRe.MatchString(action) {
		return errors.New("invalid function or action name")
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(params, &obj) != nil || obj == nil {
		return errors.New("parameters must be an object")
	}
	if raw, ok := obj["packages"]; ok {
		var pkgs []string
		if json.Unmarshal(raw, &pkgs) != nil {
			return errors.New("packages must be a list of names")
		}
		if len(pkgs) > maxActionPackages {
			return fmt.Errorf("at most %d packages at once", maxActionPackages)
		}
		for _, p := range pkgs {
			if !updates.PackageNameRe.MatchString(p) {
				return fmt.Errorf("invalid package name %q", clip(p, 64))
			}
		}
	}
	return nil
}

// setGroup moves an agent to the end of a group. Caller holds s.st.mu.
func (s *Server) setGroup(a *Agent, groupID string) error {
	if groupID != "" && s.st.group(groupID) == nil {
		return fmt.Errorf("group not found")
	}
	if a.GroupID != groupID {
		a.Order = s.st.nextOrder(groupID)
		a.GroupID = groupID
	}
	return nil
}

func (s *Server) handleAgentUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       *string `json:"name"`
		GroupID    *string `json:"group_id"`
		ReportOnly *bool   `json:"report_only"`
		Paused     *bool   `json:"paused"` // before 0.2.8
	}
	if !readJSON(w, r, &req, 16<<10) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	a := s.agentOr404(w, r)
	if a == nil {
		return
	}
	if req.Name != nil {
		n, err := checkName("system name", *req.Name, maxSystemName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if n != "" {
			a.Name = n
		}
	}
	if req.GroupID != nil {
		if err := s.setGroup(a, *req.GroupID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if req.ReportOnly == nil {
		req.ReportOnly = req.Paused
	}
	if req.ReportOnly != nil && a.Status != StatusPending {
		var j *Job
		switch {
		case *req.ReportOnly && a.Status != StatusReportOnly:
			a.Status = StatusReportOnly
			// Nothing queued earlier runs later; agent version changes stay.
			n := s.cancelQueued(a, "system set to report only")
			j = s.syncMode(a, actor(r))
			s.audit.writeJob(actor(r), clientIP(r), "agent.report_only", a, fmt.Sprintf("%d queued job(s) cancelled", n), j)
		case !*req.ReportOnly && a.Status != StatusActive:
			a.Status = StatusActive
			j = s.syncMode(a, actor(r))
			s.audit.writeJob(actor(r), clientIP(r), "agent.allow_changes", a, "", j)
		}
		s.wake(a.ID)
	}
	s.st.markDirty()
	writeJSON(w, s.view(a, false))
}

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		GroupID string `json:"group_id"`
	}
	if !readJSON(w, r, &req, 16<<10) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	a := s.agentOr404(w, r)
	if a == nil {
		return
	}
	if a.Status != "pending" {
		http.Error(w, "agent is not pending", http.StatusConflict)
		return
	}
	n, err := checkName("system name", req.Name, maxSystemName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if n != "" {
		a.Name = n
	}
	if err := s.setGroup(a, req.GroupID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.Status = StatusActive
	s.st.markDirty()
	s.wake(a.ID)
	s.audit.writeAgent(actor(r), clientIP(r), "agent.approved", a, "")
	go s.schedulerTick() // run the first check right away
	writeJSON(w, s.view(a, false))
}

func (s *Server) handleRemove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Uninstall bool `json:"uninstall"`
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
	if req.Uninstall && s.online(a, time.Now().Unix()) {
		a.Removing = true
		j := s.enqueue(a, &Job{Type: proto.CmdUninstall, Actor: actor(r), Timeout: 120})
		s.audit.writeJob(actor(r), clientIP(r), "agent.uninstall", a, "", j)
		writeJSON(w, map[string]any{"job": j.ID})
		return
	}
	delete(s.st.st.Agents, a.ID)
	delete(s.wakes, a.ID)
	s.st.markDirty()
	s.audit.writeAgent(actor(r), clientIP(r), "agent.removed", a, "")
	writeJSON(w, map[string]any{"removed": true})
}

// handleCoreUpdate up- or downgrades one agent to a version from the
// release store.
func (s *Server) handleCoreUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version string `json:"version"`
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
	if req.Version == "" {
		req.Version = s.repo.coreLatest()
	}
	goos, arch := a.Facts.OS, a.Facts.Arch
	if goos == "" {
		goos = "linux"
	}
	if s.repo.find("core", req.Version, goos, arch) == nil {
		http.Error(w, fmt.Sprintf("agent %s for %s-%s is not available on the master", req.Version, goos, arch), http.StatusBadRequest)
		return
	}
	if (a.Protocol >= 5 || a.AgentKey != "") && versionLess(req.Version, sealedAgentVersion) {
		http.Error(w, fmt.Sprintf("agent %s uses the old connection method; a system on %s or later cannot go back to it", req.Version, sealedAgentVersion), http.StatusBadRequest)
		return
	}
	if req.Version == a.CoreVersion {
		a.DesiredCore = ""
		s.st.markDirty()
		writeJSON(w, map[string]any{"agent": s.view(a, false)})
		return
	}
	a.DesiredCore = req.Version
	s.st.markDirty()
	from := a.CoreVersion
	verb := "upgrade"
	if versionLess(req.Version, from) {
		verb = "downgrade"
	}
	res := map[string]any{"agent": s.view(a, false)}
	j := s.requestCore(a, req.Version, actor(r))
	if j != nil {
		res["job"] = j.ID
	}
	s.audit.writeJob(actor(r), clientIP(r), "agent."+verb, a, from+" → "+req.Version, j)
	writeJSON(w, res)
}

// handleActions runs a feature action on one or more agents.
func (s *Server) handleActions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentIDs []string        `json:"agent_ids"`
		Feature  string          `json:"feature"`
		Action   string          `json:"action"`
		Params   json.RawMessage `json:"params"`
	}
	if !readJSON(w, r, &req, 256<<10) {
		return
	}
	if len(req.Params) == 0 || string(req.Params) == "null" {
		req.Params = json.RawMessage("{}")
	}
	if err := checkActionParams(req.Feature, req.Action, req.Params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.AgentIDs) > 10000 {
		http.Error(w, "too many systems", http.StatusBadRequest)
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	jobs := []string{}
	errs := []string{}
	for _, id := range req.AgentIDs {
		a := s.st.st.Agents[id]
		if a == nil {
			errs = append(errs, cleanText(id, 32)+": not found")
			continue
		}
		if a.Status == StatusReportOnly {
			errs = append(errs, a.Name+": report only — allow changes first")
			continue
		}
		if a.Status != StatusActive {
			errs = append(errs, a.Name+": agent is "+a.Status)
			continue
		}
		f := a.feature(req.Feature)
		if f == nil {
			errs = append(errs, a.Name+": agent "+a.CoreVersion+" has no "+req.Feature+" function (update the agent)")
			continue
		}
		spec := f.Action(req.Action)
		if spec == nil {
			errs = append(errs, a.Name+": unknown action "+req.Action)
			continue
		}
		j := s.enqueue(a, &Job{Type: proto.CmdAction, Feature: req.Feature, Action: req.Action, Params: req.Params, Actor: actor(r), Timeout: spec.Timeout})
		jobs = append(jobs, j.ID)
		if spec.Mutating {
			s.audit.writeJob(actor(r), clientIP(r), req.Feature+"."+req.Action, a, string(req.Params), j)
		}
	}
	writeJSON(w, map[string]any{"jobs": jobs, "errors": errs})
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := r.URL.Query()
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	jobs := s.jobsFiltered(q.Get("agent"), q.Get("user"), limit, q.Get("active") == "1")
	names := map[string]string{}
	for _, a := range s.st.st.Agents {
		names[a.ID] = a.Name
	}
	users := []string{}
	for _, u := range s.st.st.Users {
		users = append(users, u.Name)
	}
	sort.Strings(users)
	writeJSON(w, map[string]any{"jobs": jobs, "names": names, "users": users})
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	off, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	s.st.mu.Lock()
	j := s.st.jobByID(r.PathValue("id"))
	var jc Job
	name := ""
	if j != nil {
		jc = *j
		if a := s.st.st.Agents[j.AgentID]; a != nil {
			name = a.Name
		}
	}
	s.st.mu.Unlock()
	if j == nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	logData := ""
	if f, err := os.Open(s.st.jobLogPath(jc.ID)); err == nil {
		if off > 0 {
			f.Seek(off, io.SeekStart)
		}
		b, _ := io.ReadAll(io.LimitReader(f, 2<<20))
		f.Close()
		logData = string(b)
		off += int64(len(b))
	}
	writeJSON(w, map[string]any{"job": jc, "agent_name": name, "log": logData, "offset": off})
}

func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	out := []*EnrollToken{}
	for _, t := range s.st.st.Tokens {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	writeJSON(w, out)
}

func (s *Server) handleTokenCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Note     string `json:"note"`
		TTLHours int    `json:"ttl_hours"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	// Install commands are short-lived: 1 hour by default, at most 3.
	if req.TTLHours <= 0 {
		req.TTLHours = 1
	}
	if req.TTLHours > maxTokenHours {
		http.Error(w, fmt.Sprintf("an install command is valid for at most %d hours", maxTokenHours), http.StatusBadRequest)
		return
	}
	note, err := checkName("note", req.Note, maxNoteLen)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.baseURL(r) == "" {
		http.Error(w, badHostMsg, http.StatusBadRequest)
		return
	}
	tok := randHex(16)
	t := &EnrollToken{Hash: sha256hex(tok), Prefix: tok[:6], Note: note,
		Created: time.Now().Unix(), Expires: time.Now().Add(time.Duration(req.TTLHours) * time.Hour).Unix()}
	s.st.mu.Lock()
	s.st.st.Tokens[t.Hash] = t
	s.st.markDirty()
	s.st.mu.Unlock()
	s.audit.write(actor(r), clientIP(r), "token.created", t.Prefix, t.Note)
	writeJSON(w, map[string]any{"token": tok, "command": s.installCommand(r, tok), "expires": t.Expires, "has_core": s.repo.coreLatest() != ""})
}

func (s *Server) handleTokenDelete(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	if t := s.st.st.Tokens[r.PathValue("hash")]; t != nil {
		delete(s.st.st.Tokens, t.Hash)
		s.st.markDirty()
		s.audit.write(actor(r), clientIP(r), "token.revoked", t.Prefix, t.Note)
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	url := s.indexURL()
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	writeJSON(w, map[string]any{"settings": s.st.st.Settings, "pin": s.currentPin(), "suggested_url": s.derivedURL(r), "index_url": url,
		"os_info": s.osInfoStatus(s.st.st.Settings.OSInfoDays), "os_info_choices": osInfoChoices,
		"next_purge": s.nextPurgeAt().Unix(), "agent_port": s.agentPort(), "ui_port_error": s.uiPortError(),
		"limits": map[string]int{"heartbeat_min": minHeartbeat, "heartbeat_max": maxHeartbeat, "retention_min": minRetention, "retention_max": maxRetention}})
}

// handleRetentionPreview tells what a retention of ?days=N would remove,
// and when, if it were saved now.
func (s *Server) handleRetentionPreview(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	days = clamp(days, minRetention, maxRetention)
	s.st.mu.Lock()
	cur := s.st.st.Settings.RetentionDays
	s.st.mu.Unlock()
	at := s.nextPurgeAt()
	if days != cur {
		at = purgeAfterChange(time.Now())
	}
	cutoff, audit, jobs := s.purgePreview(days, at)
	writeJSON(w, map[string]any{"days": days, "at": at.Unix(), "cutoff": cutoff, "audit": audit, "jobs": jobs})
}

var masterURLRe = regexp.MustCompile(`^https://[A-Za-z0-9.\-\[\]:]+$`)

func (s *Server) handleSettingsSave(w http.ResponseWriter, r *http.Request) {
	var req Settings
	if !readJSON(w, r, &req, 4096) {
		return
	}
	bad := func(msg string) { http.Error(w, msg, http.StatusBadRequest) }
	if req.CheckIntervalHours < 1 || req.CheckIntervalHours > 168 {
		bad("check interval must be 1-168 hours")
		return
	}
	if req.HeartbeatSec < minHeartbeat || req.HeartbeatSec > maxHeartbeat {
		bad(fmt.Sprintf("heartbeat must be %d-%d seconds", minHeartbeat, maxHeartbeat))
		return
	}
	if req.RetentionDays < minRetention || req.RetentionDays > maxRetention {
		bad(fmt.Sprintf("retention must be %d-%d days", minRetention, maxRetention))
		return
	}
	if req.UIPort != 8443 && req.UIPort != 443 {
		bad("website port must be 8443 or 443")
		return
	}
	req.MasterURL = strings.TrimRight(strings.TrimSpace(req.MasterURL), "/")
	if req.MasterURL != "" && !masterURLRe.MatchString(req.MasterURL) {
		bad("master URL must look like https://host:port")
		return
	}
	req.GitHubRepo = strings.Trim(strings.TrimSpace(req.GitHubRepo), "/")
	req.GitHubRepo = strings.TrimPrefix(req.GitHubRepo, "https://github.com/")
	if req.GitHubRepo != "" && !ghRepoRe.MatchString(req.GitHubRepo) {
		bad("GitHub repository must look like owner/name")
		return
	}
	req.GitHubSeeded = true
	s.st.mu.Lock()
	old := s.st.st.Settings
	if req.OSInfoDays == 0 {
		req.OSInfoDays = old.OSInfoDays // a page from before this setting
	}
	if !validOSInfoDays(req.OSInfoDays) {
		s.st.mu.Unlock()
		bad("OS support lookup must be every 3, 14, 30 or 90 days")
		return
	}
	s.st.st.Settings = req
	s.st.markDirty()
	s.st.mu.Unlock()
	s.audit.write(actor(r), clientIP(r), "settings.saved", "", fmt.Sprintf("check=%dh heartbeat=%ds retention=%dd port=%d url=%q repo=%q os-info=%dd",
		req.CheckIntervalHours, req.HeartbeatSec, req.RetentionDays, req.UIPort, req.MasterURL, req.GitHubRepo, req.OSInfoDays))
	if old.OSInfoDays != req.OSInfoDays {
		s.kickOSInfo()
	}
	if old.GitHubRepo != req.GitHubRepo {
		s.kickIndex()
	}
	if old.RetentionDays != req.RetentionDays {
		s.setNextPurge(purgeAfterChange(time.Now()))
	}
	if old.UIPort != req.UIPort && s.handler != nil {
		s.applyUIPort(s.handler)
	}
	writeJSON(w, map[string]any{"settings": req, "next_purge": s.nextPurgeAt().Unix(), "ui_port_error": s.uiPortError()})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rows := s.audit.list(q.Get("agent"), q.Get("user"), 1000)
	s.st.mu.Lock()
	names := map[string]string{}
	for _, a := range s.st.st.Agents {
		names[a.ID] = a.Name
	}
	users := []string{}
	for _, u := range s.st.st.Users {
		users = append(users, u.Name)
	}
	s.st.mu.Unlock()
	sort.Strings(users)
	writeJSON(w, map[string]any{"entries": rows, "names": names, "users": users})
}

// ---- groups and ordering ----

func (s *Server) handleGroupCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	name, err := checkName("group name", req.Name, maxGroupName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if name == "" {
		http.Error(w, "group name is required", http.StatusBadRequest)
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	for _, g := range s.st.st.Groups {
		if strings.EqualFold(g.Name, name) {
			http.Error(w, "a group with this name exists", http.StatusConflict)
			return
		}
	}
	g := &Group{ID: "g_" + randHex(6), Name: name}
	s.st.st.Groups = append(s.st.st.Groups, g)
	s.st.markDirty()
	s.audit.write(actor(r), clientIP(r), "group.created", name, "")
	writeJSON(w, g)
}

func (s *Server) handleGroupUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	name, err := checkName("group name", req.Name, maxGroupName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if name == "" {
		http.Error(w, "group name is required", http.StatusBadRequest)
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	g := s.st.group(r.PathValue("id"))
	if g == nil {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}
	old := g.Name
	g.Name = name
	s.st.markDirty()
	s.audit.write(actor(r), clientIP(r), "group.renamed", name, "was "+old)
	writeJSON(w, g)
}

func (s *Server) handleGroupDelete(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	id := r.PathValue("id")
	g := s.st.group(id)
	if g == nil {
		http.Error(w, "group not found", http.StatusNotFound)
		return
	}
	// Its systems move to the end of "Ungrouped", in their current order.
	members := []*Agent{}
	for _, a := range s.orderedAgents() {
		if a.GroupID == id {
			members = append(members, a)
		}
	}
	next := s.st.nextOrder("")
	for _, a := range members {
		a.GroupID, a.Order = "", next
		next++
	}
	groups := s.st.st.Groups[:0]
	for _, x := range s.st.st.Groups {
		if x.ID != id {
			groups = append(groups, x)
		}
	}
	s.st.st.Groups = groups
	s.st.markDirty()
	s.audit.write(actor(r), clientIP(r), "group.deleted", g.Name, fmt.Sprintf("%d system(s) moved to Ungrouped", len(members)))
	writeJSON(w, map[string]bool{"ok": true})
}

// handleLayout stores the complete arrangement in one go: the order of the
// groups and, per group, which systems it holds in which order.
func (s *Server) handleLayout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Groups []struct {
			ID     string   `json:"id"` // "" = ungrouped
			Agents []string `json:"agents"`
		} `json:"groups"`
	}
	if !readJSON(w, r, &req, 256<<10) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	byID := map[string]*Group{}
	for _, g := range s.st.st.Groups {
		byID[g.ID] = g
	}
	order := []*Group{} // never nil: stored and sent as [] even without groups
	seenG := map[string]bool{}
	seenA := map[string]bool{}
	for _, gl := range req.Groups {
		if gl.ID != "" {
			g := byID[gl.ID]
			if g == nil || seenG[gl.ID] {
				http.Error(w, "unknown or duplicate group "+gl.ID, http.StatusBadRequest)
				return
			}
			seenG[gl.ID] = true
			order = append(order, g)
		}
		for _, id := range gl.Agents {
			if s.st.st.Agents[id] == nil || seenA[id] {
				http.Error(w, "unknown or duplicate system "+id, http.StatusBadRequest)
				return
			}
			seenA[id] = true
		}
	}
	// Groups not mentioned keep their relative order at the end.
	for _, g := range s.st.st.Groups {
		if !seenG[g.ID] {
			order = append(order, g)
		}
	}
	s.st.st.Groups = order
	for _, gl := range req.Groups {
		for i, id := range gl.Agents {
			a := s.st.st.Agents[id]
			a.GroupID, a.Order = gl.ID, i
		}
	}
	// Systems not mentioned (e.g. enrolled meanwhile) go after the others.
	for _, a := range s.st.st.Agents {
		if !seenA[a.ID] {
			a.Order += 100000
		}
	}
	s.st.markDirty()
	writeJSON(w, map[string]bool{"ok": true})
}
