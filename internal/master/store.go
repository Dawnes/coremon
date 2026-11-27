package master

// Persistent state: settings, users, systems and jobs.

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"coremon/internal/proto"
)

type Settings struct {
	CheckIntervalHours int    `json:"check_interval_hours"`
	HeartbeatSec       int    `json:"heartbeat_sec"`
	RetentionDays      int    `json:"retention_days"`
	UIPort             int    `json:"ui_port"`
	MasterURL          string `json:"master_url"`
	GitHubRepo         string `json:"github_repo"`
	GitHubSeeded       bool   `json:"github_seeded"`
	OSInfoDays         int    `json:"os_info_days"`
	MinAgentVersion    string `json:"min_agent_version"`
	VulnDays           int    `json:"vuln_days"`
	BackupSchedule     string `json:"backup_schedule"`
	// ProbeResultsShown: Remote Probes also lists result deliveries.
	ProbeResultsShown bool `json:"probe_results_shown,omitempty"`
	// TFAMode: "" always ask the 2FA code, "untrusted" not from TFATrusted.
	TFAMode    string   `json:"tfa_mode,omitempty"`
	TFATrusted []string `json:"tfa_trusted,omitempty"`
	// TFANever: never trusted, also inside a trusted network (always wins).
	TFANever []string `json:"tfa_never,omitempty"`
}

var osInfoChoices = []int{3, 14, 30, 90}

const defaultOSInfoDays = 14

func validOSInfoDays(d int) bool {
	for _, c := range osInfoChoices {
		if d == c {
			return true
		}
	}
	return false
}

const (
	defaultHeartbeat = 30
	minHeartbeat     = 10
	maxHeartbeat     = 300
	defaultRetention = 3
	minRetention     = 1
	maxRetention     = 30
)

func (s *Settings) normalize() {
	if s.CheckIntervalHours <= 0 {
		s.CheckIntervalHours = 6
	}
	if s.HeartbeatSec == 0 {
		s.HeartbeatSec = defaultHeartbeat
	}
	s.HeartbeatSec = clamp(s.HeartbeatSec, minHeartbeat, maxHeartbeat)
	if s.RetentionDays == 0 {
		s.RetentionDays = defaultRetention
	}
	s.RetentionDays = clamp(s.RetentionDays, minRetention, maxRetention)
	if s.UIPort != 443 {
		s.UIPort = 8443
	}
	if !validOSInfoDays(s.OSInfoDays) {
		s.OSInfoDays = defaultOSInfoDays
	}
	if !validVulnDays(s.VulnDays) {
		s.VulnDays = defaultVulnDay
	}
	if !validSchedule(s.BackupSchedule) {
		s.BackupSchedule = defaultSchedule
	}
	if nets, err := parseTrusted(strings.Join(s.TFATrusted, ",")); err != nil {
		s.TFATrusted = nil
	} else {
		s.TFATrusted = nets
	}
	if nets, err := parseNever(strings.Join(s.TFANever, ",")); err != nil {
		s.TFANever = nil
	} else {
		s.TFANever = nets
	}
	if s.TFAMode != tfaUntrusted || len(s.TFATrusted) == 0 {
		s.TFAMode = tfaAlways
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

const (
	StatusPending    = "pending"
	StatusActive     = "active"
	StatusReportOnly = "report_only"
)

// Agent: the registration is configuration (state.dat); everything the
// agent reports and the work in progress is kept in agents.dat.
type Agent struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	GroupID    string `json:"group_id"`
	Order      int    `json:"order"`
	Status     string `json:"status"`
	AgentKey   string `json:"agent_key,omitempty"`
	EnrollIP   string `json:"enroll_ip,omitempty"`
	EnrolledAt int64  `json:"enrolled_at"`
	// ProbeHold: an admin cleared the agent's check list; no list is sent
	// until Resend, or until the list (this version) changes.
	ProbeHold *string `json:"probe_hold,omitempty"`
	// ProbeOn: an admin allowed the agent to act as a remote probe; only
	// then is it offered under "Checked by". Off again only without checks.
	ProbeOn bool `json:"probe_on,omitempty"`
	// Maint: in maintenance; no automatic jobs, no failures shown.
	Maint *Maintenance `json:"maint,omitempty"`
	// Checks: services (and optionally a ping) checked from outside, by the
	// master or another agent; nil until a service is added or the address
	// and checker are set.
	Checks *Checks `json:"checks,omitempty"`
	// Via: the device it is connected via ("Connected via"); "" = none.
	Via string `json:"via,omitempty"`
	// Disks: the filesystems its agent last reported (agent 0.3.12 and
	// later), for choosing what a Disk space service watches.
	Disks []proto.DiskInfo `json:"-"`

	ProbeCap    bool                       `json:"-"` // the agent can probe agentless devices
	ProbeVer    string                     `json:"-"` // the check list the agent runs
	ProbeAt     int64                      `json:"-"` // when it confirmed that list
	ReportOnly  bool                       `json:"-"`
	Pins        [2]string                  `json:"-"`
	PinsAt      int64                      `json:"-"`
	LastSeen    int64                      `json:"-"`
	BootAt      int64                      `json:"-"` // stored instead of the uptime, which changes all the time
	CoreVersion string                     `json:"-"`
	Protocol    int                        `json:"-"`
	Facts       proto.Facts                `json:"-"`
	Features    []proto.FeatureInfo        `json:"-"`
	DesiredCore string                     `json:"-"`
	State       map[string]json.RawMessage `json:"-"`
	StateAt     map[string]int64           `json:"-"`
	Removing    bool                       `json:"-"`
}

func (a *Agent) feature(name string) *proto.FeatureInfo {
	for i := range a.Features {
		if a.Features[i].Name == name {
			return &a.Features[i]
		}
	}
	return nil
}

type EnrollToken struct {
	Hash    string `json:"hash"`
	Prefix  string `json:"prefix"`
	Note    string `json:"note"`
	Created int64  `json:"created"`
	Expires int64  `json:"expires"`
}

const (
	JobQueued    = "queued"
	JobRunning   = "running"
	JobSuccess   = "success"
	JobFailed    = "failed"
	JobTimeout   = "timeout"
	JobCancelled = "cancelled"
)

type Job struct {
	ID       string          `json:"id"`
	AgentID  string          `json:"agent_id"`
	Type     string          `json:"type"`
	Feature  string          `json:"feature,omitempty"`
	Action   string          `json:"action,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
	Artifact *proto.Artifact `json:"artifact,omitempty"`
	Actor    string          `json:"actor"`
	Status   string          `json:"status"`
	Created  int64           `json:"created"`
	Started  int64           `json:"started"`
	Finished int64           `json:"finished"`
	Timeout  int             `json:"timeout"`
	Error    string          `json:"error,omitempty"`
	LogSize  int64           `json:"log_size"`
}

func (j *Job) done() bool { return j.Status != JobQueued && j.Status != JobRunning }

const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

type User struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	PasswordHash string `json:"password_hash"`
	Created      int64  `json:"created"`

	MustChange bool `json:"must_change,omitempty"`

	Locked bool `json:"locked,omitempty"`

	TOTPSecret string `json:"totp_secret,omitempty"`
	TOTPLast   int64  `json:"totp_last,omitempty"`
	TOTPSince  int64  `json:"totp_since,omitempty"`

	NotifyEmail string `json:"notify_email,omitempty"` // set by the user in My Account; personal

	LastLogin int64  `json:"last_login,omitempty"`
	LastIP    string `json:"last_ip,omitempty"`
	PrevLogin int64  `json:"prev_login,omitempty"`
	PrevIP    string `json:"prev_ip,omitempty"`
}

const adminName = "admin"

func (s *store) userByName(name string) *User {
	for _, u := range s.st.Users {
		if strings.EqualFold(u.Name, name) {
			return u
		}
	}
	return nil
}

func (s *store) userByID(id string) *User {
	for _, u := range s.st.Users {
		if u.ID == id {
			return u
		}
	}
	return nil
}

func (s *store) admins() int {
	n := 0
	for _, u := range s.st.Users {
		if u.Role == RoleAdmin {
			n++
		}
	}
	return n
}

func (s *store) lastActiveAdmin(u *User) bool {
	if u.Role != RoleAdmin || u.Locked {
		return false
	}
	for _, x := range s.st.Users {
		if x != u && x.Role == RoleAdmin && !x.Locked {
			return false
		}
	}
	return true
}

type State struct {
	Users       []*User                   `json:"users"`
	InitialUser string                    `json:"initial_user,omitempty"`
	Settings    Settings                  `json:"settings"`
	Groups      []*Group                  `json:"groups"`
	Agents      map[string]*Agent         `json:"agents"`
	Locals      map[string]*LocalDevice   `json:"locals,omitempty"`
	Local       LocalSettings             `json:"local"`
	Virtuals    map[string]*VirtualDevice `json:"virtuals,omitempty"`
	Upstream    UpstreamSettings          `json:"upstream"`
	Notify      NotifySettings            `json:"notify"`
	Tokens      map[string]*EnrollToken   `json:"tokens"`
	Jobs        []*Job                    `json:"-"` // jobs.dat

	NVDKey string `json:"nvd_key,omitempty"`

	// AcceptedCVEs: vulnerabilities an admin accepted for all devices (by ID).
	AcceptedCVEs map[string]*AcceptedCVE `json:"accepted_cves,omitempty"`

	CheckRoundAt int64 `json:"-"` // agents.dat

	LoginsFilled bool `json:"logins_filled,omitempty"`
}

const maxJobs = 2000

// store: the configuration (state.dat), the agents' reported data
// (agents.dat) and the jobs (jobs.dat). Each file is only written when its
// content changed; see datafiles.go.
type store struct {
	mu          sync.Mutex
	path        string
	dir         string
	st          State
	dirty       bool // state.dat may have changed
	jobsDirty   bool
	agentsDirty bool
	sums        map[string][32]byte // content last written, per file
	agentsCore  [32]byte            // agents.dat without last-seen times
	agentsAt    time.Time           // last write of agents.dat
	agentsLook  time.Time           // last comparison of agents.dat
	// frozen: a restore wrote the state file; nothing may overwrite it.
	frozen bool
}

func openStore(dir string) (*store, error) {
	s := &store{path: filepath.Join(dir, stateFile), dir: dir, sums: map[string][32]byte{}}
	b, err := readStateJSON(dir)
	if err == nil {
		if err := json.Unmarshal(b, &s.st); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if s.st.Agents == nil {
		s.st.Agents = map[string]*Agent{}
	}
	if s.st.Tokens == nil {
		s.st.Tokens = map[string]*EnrollToken{}
	}
	if s.st.Groups == nil {
		s.st.Groups = []*Group{}
	}

	if s.st.Locals == nil {
		s.st.Locals = map[string]*LocalDevice{}
	}
	s.st.Local.normalize()
	for _, d := range s.st.Locals {
		if d.GroupID != "" && s.group(d.GroupID) == nil {
			d.GroupID = ""
		}
	}
	if s.st.Virtuals == nil {
		s.st.Virtuals = map[string]*VirtualDevice{}
	}
	for _, v := range s.st.Virtuals {
		if v.GroupID != "" && s.group(v.GroupID) == nil {
			v.GroupID = ""
		}
	}
	s.st.Upstream.normalize()
	s.st.Notify.normalize()
	s.st.Settings.normalize()
	s.loadAgents()
	s.loadJobs()
	for _, a := range s.st.Agents {
		if a.State == nil {
			a.State = map[string]json.RawMessage{}
		}
		if a.StateAt == nil {
			a.StateAt = map[string]int64{}
		}
		if a.GroupID != "" && s.group(a.GroupID) == nil {
			a.GroupID = ""
		}
	}

	return s, s.writeNow()
}

func (s *store) group(id string) *Group {
	for _, g := range s.st.Groups {
		if g.ID == id {
			return g
		}
	}
	return nil
}

func (s *store) nextOrder(groupID string) int {
	n := 0
	for _, a := range s.st.Agents {
		if a.GroupID == groupID && a.Order >= n {
			n = a.Order + 1
		}
	}
	for _, d := range s.st.Locals {
		if d.GroupID == groupID && d.Order >= n {
			n = d.Order + 1
		}
	}
	for _, v := range s.st.Virtuals {
		if v.GroupID == groupID && v.Order >= n {
			n = v.Order + 1
		}
	}
	return n
}

// markDirty: something may have changed anywhere. Each file is compared
// and only written when its content differs.
func (s *store) markDirty() { s.dirty, s.jobsDirty, s.agentsDirty = true, true, true }

// markJobs: only jobs changed.
func (s *store) markJobs() { s.jobsDirty = true }

// markAgents: only what agents report changed.
func (s *store) markAgents() { s.agentsDirty = true }

// writeNow writes every file whose content changed, right away.
func (s *store) writeNow() error { return s.flush(time.Now(), true) }

func (s *store) flushLoop(stop <-chan struct{}) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			s.mu.Lock()
			if err := s.flush(time.Now(), true); err != nil {
				log.Printf("save: %v", err)
			}
			s.mu.Unlock()
			return
		case <-t.C:
			s.mu.Lock()
			if err := s.flush(time.Now(), false); err != nil {
				log.Printf("save: %v", err)
			}
			s.mu.Unlock()
		}
	}
}

func (s *store) jobByID(id string) *Job {
	for i := len(s.st.Jobs) - 1; i >= 0; i-- {
		if s.st.Jobs[i].ID == id {
			return s.st.Jobs[i]
		}
	}
	return nil
}

func (s *store) jobLogPath(id string) string { return filepath.Join(s.dir, "jobs", id+".log") }

func (s *store) addJob(j *Job) {
	s.st.Jobs = append(s.st.Jobs, j)
	if n := len(s.st.Jobs) - maxJobs; n > 0 {
		for _, old := range s.st.Jobs[:n] {
			os.Remove(s.jobLogPath(old.ID))
		}
		s.st.Jobs = append([]*Job(nil), s.st.Jobs[n:]...)
	}
	s.markJobs()
}

func (s *store) purgeJobs(cutoff int64) int {
	keep := s.st.Jobs[:0]
	removed := 0
	for _, j := range s.st.Jobs {
		end := j.Finished
		if end == 0 {
			end = j.Created
		}
		if j.done() && end < cutoff {
			os.Remove(s.jobLogPath(j.ID))
			removed++
			continue
		}
		keep = append(keep, j)
	}
	s.st.Jobs = keep
	known := map[string]bool{}
	for _, j := range s.st.Jobs {
		known[j.ID+".log"] = true
	}
	if entries, err := os.ReadDir(filepath.Join(s.dir, "jobs")); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".log") && !known[e.Name()] {
				os.Remove(filepath.Join(s.dir, "jobs", e.Name()))
			}
		}
	}
	if removed > 0 {
		s.markJobs()
	}
	return removed
}
