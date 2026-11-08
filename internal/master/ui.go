package master

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"coremon/internal/proto"
)

const actorAdmin = "admin"

// requireUI checks the session cookie and the anti-CSRF header.
func (s *Server) requireUI(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Coremon") != "1" {
			http.Error(w, "missing X-Coremon header", http.StatusForbidden)
			return
		}
		c, err := r.Cookie(cookieName)
		if err != nil || !s.sess.valid(c.Value) {
			http.Error(w, "not logged in", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(cookieName)
	authed := err == nil && s.sess.valid(c.Value)
	_, statErr := os.Stat(s.initialPwPath())
	writeJSON(w, map[string]any{"authenticated": authed, "version": s.cfg.Version, "initial_password": statErr == nil})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Coremon") != "1" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	ip := clientIP(r)
	if !s.sess.allowLogin(ip) {
		http.Error(w, "too many failed attempts; try again later", http.StatusTooManyRequests)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	s.st.mu.Lock()
	hash := s.st.st.PasswordHash
	s.st.mu.Unlock()
	if !checkPassword(hash, req.Password) {
		s.sess.recordFail(ip)
		s.audit.write("?", ip, "login.failed", "", "")
		time.Sleep(400 * time.Millisecond)
		http.Error(w, "wrong password", http.StatusUnauthorized)
		return
	}
	tok := s.sess.create()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: tok, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	s.audit.write(actorAdmin, ip, "login", "", "")
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.sess.destroy(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	if len(req.New) < 10 {
		http.Error(w, "new password must be at least 10 characters", http.StatusBadRequest)
		return
	}
	s.st.mu.Lock()
	ok := checkPassword(s.st.st.PasswordHash, req.Current)
	s.st.mu.Unlock()
	if !ok {
		http.Error(w, "current password is wrong", http.StatusForbidden)
		return
	}
	h := hashPassword(req.New)
	s.st.mu.Lock()
	s.st.st.PasswordHash = h
	s.st.markDirty()
	s.st.mu.Unlock()
	os.Remove(s.initialPwPath())
	s.sess.destroyAll()
	tok := s.sess.create()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: tok, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	s.audit.write(actorAdmin, clientIP(r), "password.changed", "", "")
	writeJSON(w, map[string]bool{"ok": true})
}

// ---- views ----

type updatesSummary struct {
	Total          int   `json:"total"`
	Security       int   `json:"security"`
	RebootRequired bool  `json:"reboot_required"`
	CheckedAt      int64 `json:"checked_at"`
}

type moduleView struct {
	proto.ModuleInfo
	Desired  string `json:"desired"`
	Latest   string `json:"latest"`
	Upgrade  bool   `json:"upgrade"`
	Deployed bool   `json:"deployed"` // reported by agent
}

type agentView struct {
	ID          string                     `json:"id"`
	Name        string                     `json:"name"`
	Tags        []string                   `json:"tags"`
	Status      string                     `json:"status"`
	Online      bool                       `json:"online"`
	LastSeen    int64                      `json:"last_seen"`
	EnrolledAt  int64                      `json:"enrolled_at"`
	CoreVersion string                     `json:"core_version"`
	CoreLatest  string                     `json:"core_latest"`
	CoreUpgrade bool                       `json:"core_upgrade"`
	DesiredCore string                     `json:"desired_core"`
	Facts       proto.Facts                `json:"facts"`
	Modules     []moduleView               `json:"modules"`
	Updates     *updatesSummary            `json:"updates,omitempty"`
	Busy        int                        `json:"busy"` // open jobs
	Removing    bool                       `json:"removing"`
	State       map[string]json.RawMessage `json:"state,omitempty"`
	StateAt     map[string]int64           `json:"state_at,omitempty"`
}

// view builds the UI representation. Caller holds s.st.mu.
func (s *Server) view(a *Agent, full bool) agentView {
	now := time.Now().Unix()
	v := agentView{
		ID: a.ID, Name: a.Name, Tags: a.Tags, Status: a.Status, Online: a.online(now),
		LastSeen: a.LastSeen, EnrolledAt: a.EnrolledAt, CoreVersion: a.CoreVersion,
		Facts: a.Facts, Removing: a.Removing, DesiredCore: a.DesiredCore,
	}
	v.CoreLatest = s.repo.coreLatest()
	v.CoreUpgrade = v.CoreLatest != "" && a.CoreVersion != "" && versionLess(a.CoreVersion, v.CoreLatest)
	if v.Tags == nil {
		v.Tags = []string{}
	}
	seen := map[string]bool{}
	for _, m := range a.Modules {
		mv := moduleView{ModuleInfo: m, Deployed: true, Latest: s.repo.moduleLatest(m.Name)}
		mv.Desired = a.DesiredModules[m.Name]
		mv.Upgrade = mv.Latest != "" && m.Version != "" && versionLess(m.Version, mv.Latest)
		v.Modules = append(v.Modules, mv)
		seen[m.Name] = true
	}
	for name, want := range a.DesiredModules {
		if !seen[name] {
			v.Modules = append(v.Modules, moduleView{ModuleInfo: proto.ModuleInfo{Name: name}, Desired: want, Latest: s.repo.moduleLatest(name)})
		}
	}
	sort.Slice(v.Modules, func(i, j int) bool { return v.Modules[i].Name < v.Modules[j].Name })
	if v.Modules == nil {
		v.Modules = []moduleView{}
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

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	agents := []agentView{}
	for _, a := range s.st.st.Agents {
		agents = append(agents, s.view(a, false))
	}
	sort.Slice(agents, func(i, j int) bool { return strings.ToLower(agents[i].Name) < strings.ToLower(agents[j].Name) })
	active := 0
	for _, j := range s.st.st.Jobs {
		if !j.done() {
			active++
		}
	}
	writeJSON(w, map[string]any{
		"agents": agents, "active_jobs": active, "server_time": time.Now().Unix(),
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
	out := []*Job{}
	for i := len(s.st.st.Jobs) - 1; i >= 0 && len(out) < limit; i-- {
		j := s.st.st.Jobs[i]
		if (agentID == "" || j.AgentID == agentID) && (!activeOnly || !j.done()) {
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
	writeJSON(w, map[string]any{"agent": s.view(a, true), "jobs": s.jobsFor(a.ID, 50, false), "repo": s.repo.summary()})
}

var tagRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)

func cleanTags(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		if !tagRe.MatchString(t) {
			return nil, fmt.Errorf("invalid tag %q (letters, digits, . _ - only)", t)
		}
		seen[t] = true
		out = append(out, t)
	}
	return out, nil
}

func cleanName(n string) string {
	n = strings.TrimSpace(n)
	if len(n) > 64 {
		n = n[:64]
	}
	return n
}

func (s *Server) handleAgentUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   *string   `json:"name"`
		Tags   *[]string `json:"tags"`
		Paused *bool     `json:"paused"`
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
		if n := cleanName(*req.Name); n != "" {
			a.Name = n
		}
	}
	if req.Tags != nil {
		t, err := cleanTags(*req.Tags)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		a.Tags = t
	}
	if req.Paused != nil && a.Status != "pending" {
		if *req.Paused {
			a.Status = "paused"
		} else {
			a.Status = "active"
			s.wake(a.ID)
		}
		s.audit.write(actorAdmin, clientIP(r), "agent."+a.Status, a.ID, a.Name)
	}
	s.st.markDirty()
	writeJSON(w, s.view(a, false))
}

var moduleNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

func (s *Server) handleApprove(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string   `json:"name"`
		Tags    []string `json:"tags"`
		Modules []string `json:"modules"`
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
	tags, err := cleanTags(req.Tags)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if n := cleanName(req.Name); n != "" {
		a.Name = n
	}
	a.Tags = tags
	for _, m := range req.Modules {
		if !moduleNameRe.MatchString(m) {
			continue
		}
		if l := s.repo.moduleLatest(m); l != "" {
			a.DesiredModules[m] = l
		}
	}
	a.Status = "active"
	s.st.markDirty()
	s.wake(a.ID)
	s.audit.write(actorAdmin, clientIP(r), "agent.approved", a.ID, fmt.Sprintf("%s modules=%v", a.Name, req.Modules))
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
	if req.Uninstall && a.online(time.Now().Unix()) {
		a.Removing = true
		j := s.enqueue(a, &Job{Type: proto.CmdUninstall, Actor: actorAdmin, Timeout: 120})
		s.audit.write(actorAdmin, clientIP(r), "agent.uninstall", a.ID, a.Name)
		writeJSON(w, map[string]any{"job": j.ID})
		return
	}
	delete(s.st.st.Agents, a.ID)
	delete(s.wakes, a.ID)
	s.st.markDirty()
	s.audit.write(actorAdmin, clientIP(r), "agent.removed", a.ID, a.Name)
	writeJSON(w, map[string]any{"removed": true})
}

func (s *Server) handleModuleOp(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Op   string `json:"op"` // deploy | upgrade | remove
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	if !moduleNameRe.MatchString(req.Name) {
		http.Error(w, "invalid module name", http.StatusBadRequest)
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	a := s.agentOr404(w, r)
	if a == nil {
		return
	}
	switch req.Op {
	case "deploy", "upgrade":
		l := s.repo.moduleLatest(req.Name)
		if l == "" {
			http.Error(w, "module not in repository", http.StatusBadRequest)
			return
		}
		a.DesiredModules[req.Name] = l
	case "remove":
		delete(a.DesiredModules, req.Name)
	default:
		http.Error(w, "invalid op", http.StatusBadRequest)
		return
	}
	s.st.markDirty()
	s.audit.write(actorAdmin, clientIP(r), "module."+req.Op, a.ID, a.Name+": "+req.Name)
	go s.schedulerTick()
	writeJSON(w, s.view(a, false))
}

func (s *Server) handleCoreUpdate(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	a := s.agentOr404(w, r)
	if a == nil {
		return
	}
	l := s.repo.coreLatest()
	if l == "" {
		http.Error(w, "no agent-core in repository", http.StatusBadRequest)
		return
	}
	a.DesiredCore = l
	s.st.markDirty()
	s.audit.write(actorAdmin, clientIP(r), "core.update", a.ID, a.Name+" -> "+l)
	go s.schedulerTick()
	writeJSON(w, s.view(a, false))
}

// handleActions runs a module action on one or more agents.
func (s *Server) handleActions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentIDs []string        `json:"agent_ids"`
		Module   string          `json:"module"`
		Action   string          `json:"action"`
		Params   json.RawMessage `json:"params"`
	}
	if !readJSON(w, r, &req, 256<<10) {
		return
	}
	if len(req.Params) == 0 || string(req.Params) == "null" {
		req.Params = json.RawMessage("{}")
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	jobs := []string{}
	errs := []string{}
	for _, id := range req.AgentIDs {
		a := s.st.st.Agents[id]
		if a == nil {
			errs = append(errs, id+": not found")
			continue
		}
		if a.Status != "active" {
			errs = append(errs, a.Name+": agent is "+a.Status)
			continue
		}
		m := a.module(req.Module)
		if m == nil || m.Manifest == nil {
			errs = append(errs, a.Name+": module "+req.Module+" not installed")
			continue
		}
		spec := m.Manifest.Action(req.Action)
		if spec == nil {
			errs = append(errs, a.Name+": unknown action "+req.Action)
			continue
		}
		j := s.enqueue(a, &Job{Type: proto.CmdModuleCall, Module: req.Module, Action: req.Action, Params: req.Params, Actor: actorAdmin, Timeout: spec.Timeout})
		jobs = append(jobs, j.ID)
		if spec.Mutating {
			s.audit.write(actorAdmin, clientIP(r), req.Module+"."+req.Action, a.ID, a.Name+" "+string(req.Params))
		}
	}
	writeJSON(w, map[string]any{"jobs": jobs, "errors": errs})
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	jobs := s.jobsFor(r.URL.Query().Get("agent"), limit, r.URL.Query().Get("active") == "1")
	names := map[string]string{}
	for _, j := range jobs {
		if a := s.st.st.Agents[j.AgentID]; a != nil {
			names[j.AgentID] = a.Name
		}
	}
	writeJSON(w, map[string]any{"jobs": jobs, "names": names})
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
	if req.TTLHours <= 0 || req.TTLHours > 24*30 {
		req.TTLHours = 24
	}
	tok := randHex(16)
	t := &EnrollToken{Hash: sha256hex(tok), Prefix: tok[:6], Note: cleanName(req.Note),
		Created: time.Now().Unix(), Expires: time.Now().Add(time.Duration(req.TTLHours) * time.Hour).Unix()}
	s.st.mu.Lock()
	s.st.st.Tokens[t.Hash] = t
	s.st.markDirty()
	s.st.mu.Unlock()
	s.audit.write(actorAdmin, clientIP(r), "token.created", t.Prefix, t.Note)
	writeJSON(w, map[string]any{"token": tok, "command": s.installCommand(r, tok), "expires": t.Expires, "has_core": s.repo.coreLatest() != ""})
}

func (s *Server) handleTokenDelete(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	if t := s.st.st.Tokens[r.PathValue("hash")]; t != nil {
		delete(s.st.st.Tokens, t.Hash)
		s.st.markDirty()
		s.audit.write(actorAdmin, clientIP(r), "token.revoked", t.Prefix, t.Note)
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleRepo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.repo.summary())
}

func (s *Server) handleRepoRescan(w http.ResponseWriter, r *http.Request) {
	s.repo.scan()
	s.audit.write(actorAdmin, clientIP(r), "repo.rescan", "", "")
	writeJSON(w, s.repo.summary())
}

func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	writeJSON(w, map[string]any{"settings": s.st.st.Settings, "pin": s.pin, "suggested_url": s.derivedURL(r)})
}

func (s *Server) handleSettingsSave(w http.ResponseWriter, r *http.Request) {
	var req Settings
	if !readJSON(w, r, &req, 4096) {
		return
	}
	if req.CheckIntervalHours < 1 || req.CheckIntervalHours > 168 {
		http.Error(w, "check interval must be 1-168 hours", http.StatusBadRequest)
		return
	}
	req.MasterURL = strings.TrimRight(strings.TrimSpace(req.MasterURL), "/")
	if req.MasterURL != "" && !regexp.MustCompile(`^https://[A-Za-z0-9.\-\[\]:]+$`).MatchString(req.MasterURL) {
		http.Error(w, "master URL must look like https://host:port", http.StatusBadRequest)
		return
	}
	req.GitHubRepo = strings.Trim(strings.TrimSpace(req.GitHubRepo), "/")
	req.GitHubRepo = strings.TrimPrefix(req.GitHubRepo, "https://github.com/")
	if req.GitHubRepo != "" && !ghRepoRe.MatchString(req.GitHubRepo) {
		http.Error(w, "GitHub repository must look like owner/name", http.StatusBadRequest)
		return
	}
	req.GitHubSeeded = true
	s.st.mu.Lock()
	s.st.st.Settings = req
	s.st.markDirty()
	s.st.mu.Unlock()
	s.audit.write(actorAdmin, clientIP(r), "settings.saved", "", fmt.Sprintf("%+v", req))
	select {
	case s.gh.kick <- struct{}{}:
	default:
	}
	writeJSON(w, req)
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.audit.tail(300))
}
