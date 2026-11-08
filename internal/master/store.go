package master

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"coremon/internal/proto"
)

// Settings are editable in the UI.
type Settings struct {
	CheckIntervalHours int    `json:"check_interval_hours"`
	MasterURL          string `json:"master_url"` // used in install commands; empty = derive from browser
	GitHubRepo         string `json:"github_repo"`        // owner/name to mirror releases from
	GitHubAutoImport   bool   `json:"github_auto_import"` // import new releases automatically (never rolls out)
	GitHubSeeded       bool   `json:"github_seeded"`
}

// Agent is a registered system.
type Agent struct {
	ID             string                     `json:"id"`
	Name           string                     `json:"name"`
	Tags           []string                   `json:"tags"`
	Status         string                     `json:"status"` // pending | active | paused
	SecretHash     string                     `json:"secret_hash"`
	EnrolledAt     int64                      `json:"enrolled_at"`
	LastSeen       int64                      `json:"last_seen"`
	CoreVersion    string                     `json:"core_version"`
	Facts          proto.Facts                `json:"facts"`
	Modules        []proto.ModuleInfo         `json:"modules"`
	DesiredModules map[string]string          `json:"desired_modules"` // name -> version
	DesiredCore    string                     `json:"desired_core"`
	State          map[string]json.RawMessage `json:"state"` // module name -> state
	StateAt        map[string]int64           `json:"state_at"`
	Removing       bool                       `json:"removing"`
}

func (a *Agent) module(name string) *proto.ModuleInfo {
	for i := range a.Modules {
		if a.Modules[i].Name == name {
			return &a.Modules[i]
		}
	}
	return nil
}

// EnrollToken is a single-use enrollment token (only its hash is stored).
type EnrollToken struct {
	Hash    string `json:"hash"`
	Prefix  string `json:"prefix"`
	Note    string `json:"note"`
	Created int64  `json:"created"`
	Expires int64  `json:"expires"`
}

// Job statuses.
const (
	JobQueued  = "queued"
	JobRunning = "running"
	JobSuccess = "success"
	JobFailed  = "failed"
	JobTimeout = "timeout"
)

// Job is one command sent (or to be sent) to one agent.
type Job struct {
	ID       string          `json:"id"`
	AgentID  string          `json:"agent_id"`
	Type     string          `json:"type"`
	Module   string          `json:"module,omitempty"`
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

// State is everything the master persists (besides logs and audit).
type State struct {
	PasswordHash string                  `json:"password_hash"`
	Settings     Settings                `json:"settings"`
	Agents       map[string]*Agent       `json:"agents"`
	Tokens       map[string]*EnrollToken `json:"tokens"`
	Jobs         []*Job                  `json:"jobs"` // oldest first
}

const maxJobs = 2000

// store persists State as a single JSON file, written atomically and
// debounced. The master has little data, so this keeps dependencies at zero.
type store struct {
	mu    sync.Mutex
	path  string
	dir   string
	st    State
	dirty bool
}

func openStore(dir string) (*store, error) {
	s := &store{path: filepath.Join(dir, "state.json"), dir: dir}
	b, err := os.ReadFile(s.path)
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
	if s.st.Settings.CheckIntervalHours <= 0 {
		s.st.Settings.CheckIntervalHours = 6
	}
	// Jobs in flight when the master stopped will never get a result.
	for _, j := range s.st.Jobs {
		if !j.done() {
			j.Status, j.Error, j.Finished = JobFailed, "master restarted", time.Now().Unix()
		}
	}
	return s, s.writeNow()
}

func (s *store) markDirty() { s.dirty = true }

func (s *store) writeNow() error {
	b, err := json.Marshal(&s.st)
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
