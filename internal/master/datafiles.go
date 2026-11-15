package master

// The master's data lives in three files, so that frequent changes never
// rewrite the configuration:
//
//	state.dat   configuration: users, settings, groups, devices, agent registrations
//	agents.dat  what the agents report (system information, check results,
//	            last contact) and agent updates in progress
//	jobs.dat    the jobs
//
// (checks.dat, with the agentless check results, is in checksfile.go.)
// A file is only written when its content changed. agents.dat is written
// at most once a minute, and when only last-seen times changed at most
// every 15 minutes; everything is written at shutdown. A missing or
// damaged agents.dat or jobs.dat costs only that data: the agents report
// again within a heartbeat and are checked again right away.

import (
	"crypto/sha256"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"coremon/internal/proto"
)

const (
	agentsFileName = "agents.dat"
	jobsFileName   = "jobs.dat"
	agentsEvery    = time.Minute
	seenEvery      = 15 * time.Minute
	bootJitter     = 60 // seconds the computed boot time may move without counting as a reboot
)

// agentRun: what one agent reported, as kept in agents.dat.
type agentRun struct {
	LastSeen    int64                      `json:"last_seen,omitempty"`
	BootAt      int64                      `json:"boot_at,omitempty"`
	CoreVersion string                     `json:"core_version,omitempty"`
	Protocol    int                        `json:"protocol,omitempty"`
	ReportOnly  bool                       `json:"report_only,omitempty"`
	Pins        [2]string                  `json:"pins"`
	PinsAt      int64                      `json:"pins_at,omitempty"`
	Facts       proto.Facts                `json:"facts"`
	Features    []proto.FeatureInfo        `json:"features,omitempty"`
	DesiredCore string                     `json:"desired_core,omitempty"`
	Removing    bool                       `json:"removing,omitempty"`
	State       map[string]json.RawMessage `json:"state,omitempty"`
	StateAt     map[string]int64           `json:"state_at,omitempty"`
}

type agentsData struct {
	CheckRoundAt int64                `json:"check_round_at,omitempty"`
	Agents       map[string]*agentRun `json:"agents"`
}

type jobsData struct {
	Jobs []Job `json:"jobs"`
}

// run: the agent's reported data. core leaves out what changes without
// news: the last-seen times and when the health was collected.
func (a *Agent) run(core bool) *agentRun {
	r := &agentRun{LastSeen: a.LastSeen, BootAt: a.BootAt, CoreVersion: a.CoreVersion, Protocol: a.Protocol, ReportOnly: a.ReportOnly,
		Pins: a.Pins, PinsAt: a.PinsAt, Facts: a.Facts, Features: a.Features, DesiredCore: a.DesiredCore, Removing: a.Removing,
		State: a.State, StateAt: a.StateAt}
	r.Facts.UptimeSec = 0 // BootAt instead
	if core {
		r.LastSeen, r.PinsAt = 0, 0
		if h := r.Facts.Health; h != nil {
			hc := *h
			hc.At = 0
			r.Facts.Health = &hc
		}
	}
	return r
}

func (a *Agent) setRun(r *agentRun) {
	a.LastSeen, a.BootAt, a.CoreVersion, a.Protocol, a.ReportOnly = r.LastSeen, r.BootAt, r.CoreVersion, r.Protocol, r.ReportOnly
	a.Pins, a.PinsAt, a.Facts, a.Features, a.DesiredCore, a.Removing = r.Pins, r.PinsAt, r.Facts, r.Features, r.DesiredCore, r.Removing
	a.State, a.StateAt = r.State, r.StateAt
	if a.LastSeen > 0 && a.BootAt > 0 && a.LastSeen >= a.BootAt {
		a.Facts.UptimeSec = a.LastSeen - a.BootAt // the uptime at the last contact
	}
}

// setBoot keeps the boot time unless it really moved (a reboot): the
// uptime is reported in whole seconds, so now-uptime wobbles a little.
func (a *Agent) setBoot(now int64) {
	if a.Facts.UptimeSec <= 0 {
		return
	}
	b := now - a.Facts.UptimeSec
	if d := b - a.BootAt; a.BootAt == 0 || d > bootJitter || d < -bootJitter {
		a.BootAt = b
	}
}

func (s *store) agentsJSON(core bool) ([]byte, error) {
	ad := agentsData{CheckRoundAt: s.st.CheckRoundAt, Agents: map[string]*agentRun{}}
	if core {
		ad.CheckRoundAt = 0
	}
	for id, a := range s.st.Agents {
		ad.Agents[id] = a.run(core)
	}
	return json.Marshal(&ad)
}

// jobsJSON: the jobs without their log size, which is read from the log
// file (live output would otherwise rewrite jobs.dat all the time).
func (s *store) jobsJSON() ([]byte, error) {
	jd := jobsData{Jobs: make([]Job, len(s.st.Jobs))}
	for i, j := range s.st.Jobs {
		jd.Jobs[i] = *j
		jd.Jobs[i].LogSize = 0
	}
	return json.Marshal(&jd)
}

// readData reads one of the data files; ok is false when it is missing
// or cannot be read (logged).
func (s *store) readData(name string, v any) bool {
	b, err := os.ReadFile(filepath.Join(s.dir, name))
	if os.IsNotExist(err) {
		return false
	}
	if err == nil {
		var js []byte
		if js, err = decodeState(b); err == nil {
			err = json.Unmarshal(js, v)
		}
	}
	if err != nil {
		log.Printf("%s cannot be read (%v): starting without it; the configuration is not affected", name, err)
		return false
	}
	return true
}

func (s *store) loadAgents() {
	var ad agentsData
	if !s.readData(agentsFileName, &ad) {
		return
	}
	s.st.CheckRoundAt = ad.CheckRoundAt
	for id, r := range ad.Agents {
		if a := s.st.Agents[id]; a != nil && r != nil {
			a.setRun(r)
		}
	}
}

func (s *store) loadJobs() {
	var jd jobsData
	s.st.Jobs = nil
	if !s.readData(jobsFileName, &jd) {
		return
	}
	now := time.Now().Unix()
	for i := range jd.Jobs {
		j := &jd.Jobs[i]
		if fi, err := os.Stat(s.jobLogPath(j.ID)); err == nil {
			j.LogSize = fi.Size()
		}
		if !j.done() {
			j.Status, j.Error, j.Finished = JobFailed, "master restarted", now
		}
		s.st.Jobs = append(s.st.Jobs, j)
	}
}

// put writes one file if its content differs from what was last written.
func (s *store) put(name string, js []byte) error {
	sum := sha256.Sum256(js)
	if sum == s.sums[name] {
		return nil
	}
	b, err := encodeState(js)
	if err == nil {
		err = writeAtomic(filepath.Join(s.dir, name), b)
	}
	if err == nil {
		s.sums[name] = sum
	}
	return err
}

// flush writes what changed. force: everything now (start, shutdown,
// tests); otherwise agents.dat keeps to its rhythm. Caller holds s.mu.
func (s *store) flush(now time.Time, force bool) error {
	if s.frozen {
		return nil
	}
	var first error
	keep := func(err error) bool {
		if err != nil && first == nil {
			first = err
		}
		return err == nil
	}
	if s.dirty || force {
		js, err := json.Marshal(&s.st)
		if err == nil {
			err = s.put(stateFile, js)
		}
		if keep(err) {
			s.dirty = false
		}
	}
	if s.jobsDirty || force {
		js, err := s.jobsJSON()
		if err == nil {
			err = s.put(jobsFileName, js)
		}
		if keep(err) {
			s.jobsDirty = false
		}
	}
	if (s.agentsDirty || force) && (force || now.Sub(s.agentsLook) >= agentsEvery) {
		s.agentsLook = now
		full, err := s.agentsJSON(false)
		var core []byte
		if err == nil {
			core, err = s.agentsJSON(true)
		}
		if keep(err) {
			coreSum := sha256.Sum256(core)
			switch {
			case sha256.Sum256(full) == s.sums[agentsFileName]:
				s.agentsDirty = false
			case force || coreSum != s.agentsCore || now.Sub(s.agentsAt) >= seenEvery:
				if keep(s.put(agentsFileName, full)) {
					s.agentsCore, s.agentsAt, s.agentsDirty = coreSum, now, false
				}
			}
		}
	}
	return first
}
