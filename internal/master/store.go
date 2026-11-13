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

type Agent struct {
	ID          string                     `json:"id"`
	Name        string                     `json:"name"`
	GroupID     string                     `json:"group_id"`
	Order       int                        `json:"order"`
	Status      string                     `json:"status"`
	ReportOnly  bool                       `json:"agent_report_only"`
	Pins        [2]string                  `json:"pins"`
	PinsAt      int64                      `json:"pins_at"`
	AgentKey    string                     `json:"agent_key,omitempty"`
	EnrollIP    string                     `json:"enroll_ip,omitempty"`
	EnrolledAt  int64                      `json:"enrolled_at"`
	LastSeen    int64                      `json:"last_seen"`
	CoreVersion string                     `json:"core_version"`
	Protocol    int                        `json:"protocol"`
	Facts       proto.Facts                `json:"facts"`
	Features    []proto.FeatureInfo        `json:"features"`
	DesiredCore string                     `json:"desired_core"`
	State       map[string]json.RawMessage `json:"state"`
	StateAt     map[string]int64           `json:"state_at"`
	Removing    bool                       `json:"removing"`
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
	Users       []*User                 `json:"users"`
	InitialUser string                  `json:"initial_user,omitempty"`
	Settings    Settings                `json:"settings"`
	Groups      []*Group                `json:"groups"`
	Agents      map[string]*Agent       `json:"agents"`
	Tokens      map[string]*EnrollToken `json:"tokens"`
	Jobs        []*Job                  `json:"jobs"`

	NVDKey string `json:"nvd_key,omitempty"`
}

const maxJobs = 2000

type store struct {
	mu    sync.Mutex
	path  string
	dir   string
	st    State
	dirty bool
	// frozen: a restore wrote the state file; nothing may overwrite it.
	frozen bool
}

func openStore(dir string) (*store, error) {
	s := &store{path: filepath.Join(dir, stateFile), dir: dir}
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

	s.st.Settings.normalize()
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

	for _, j := range s.st.Jobs {
		if !j.done() {
			j.Status, j.Error, j.Finished = JobFailed, "master restarted", time.Now().Unix()
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
	return n
}

func (s *store) markDirty() { s.dirty = true }

func (s *store) writeNow() error {
	if s.frozen {
		return nil
	}
	js, err := json.Marshal(&s.st)
	if err != nil {
		return err
	}
	b, err := encodeState(js)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *store) flushLoop(stop <-chan struct{}) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			s.mu.Lock()
			if s.dirty {
				_ = s.writeNow()
			}
			s.mu.Unlock()
			return
		case <-t.C:
			s.mu.Lock()
			if s.dirty {
				if err := s.writeNow(); err != nil {
					log.Printf("save state: %v", err)
				} else {
					s.dirty = false
				}
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
	s.markDirty()
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
		s.markDirty()
	}
	return removed
}
