package master

// checks.dat: the results of the ping and service checks of agentless and
// agent devices (latest result and the
// 24-hour history of the ping and every service). They change with every
// check, so they live apart from state.dat, which then only changes when
// the configuration or an agent's data does. Written once a minute and at
// shutdown; if the file is lost or damaged only check results are lost.

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	checksFileName = "checks.dat"
	checksEvery    = time.Minute
)

// runFields: the result side of one check.
type runFields struct {
	Checked   bool       `json:"checked,omitempty"`
	Up        bool       `json:"up,omitempty"`
	Since     int64      `json:"since,omitempty"`
	LastCheck int64      `json:"last_check,omitempty"`
	LastOK    int64      `json:"last_ok,omitempty"`
	RTTUs     int64      `json:"rtt_us,omitempty"`
	Fails     int        `json:"fails,omitempty"`
	LastError string     `json:"last_error,omitempty"`
	Banner    string     `json:"banner,omitempty"`
	Failed    string     `json:"failed,omitempty"` // services: the sub-probe of the last failed check
	Hist      []histSlot `json:"hist,omitempty"`
	RTT       []rttSlot  `json:"rtt,omitempty"` // ping only: round-trip times in five-minute slots
}

type devRun struct {
	Ping     runFields            `json:"ping"`
	Services map[string]runFields `json:"services,omitempty"`
}

type checksData struct {
	Saved   int64              `json:"saved"`
	Devices map[string]*devRun `json:"devices"`
}

func (d *Checks) run() runFields {
	return runFields{Checked: d.Checked, Up: d.Up, Since: d.Since, LastCheck: d.LastCheck, LastOK: d.LastUp,
		RTTUs: d.RTTUs, Fails: d.Fails, LastError: d.LastError, Hist: d.Hist, RTT: d.RTT}
}

func (d *Checks) setRun(r runFields) {
	d.Checked, d.Up, d.Since, d.LastCheck, d.LastUp, d.RTTUs, d.Fails, d.LastError, d.Hist, d.RTT =
		r.Checked, r.Up, r.Since, r.LastCheck, r.LastOK, r.RTTUs, r.Fails, r.LastError, r.Hist, r.RTT
}

func (sv *Service) run() runFields {
	return runFields{Checked: sv.Checked, Up: sv.Up, Since: sv.Since, LastCheck: sv.LastCheck, LastOK: sv.LastOK,
		RTTUs: sv.RTTUs, Fails: sv.Fails, LastError: sv.LastError, Banner: sv.Banner, Failed: sv.Failed, Hist: sv.Hist}
}

func (sv *Service) setRun(r runFields) {
	sv.Checked, sv.Up, sv.Since, sv.LastCheck, sv.LastOK, sv.RTTUs, sv.Fails, sv.LastError, sv.Banner, sv.Failed, sv.Hist =
		r.Checked, r.Up, r.Since, r.LastCheck, r.LastOK, r.RTTUs, r.Fails, r.LastError, r.Banner, r.Failed, r.Hist
}

func (s *Server) checksPath() string { return filepath.Join(s.cfg.DataDir, checksFileName) }

// markChecks: a check result changed. Caller holds s.st.mu.
func (s *Server) markChecks() { s.checksDirty = true }

// checksJSON: the current results. Caller holds s.st.mu.
func (s *Server) checksJSON() ([]byte, error) {
	cd := checksData{Saved: time.Now().Unix(), Devices: map[string]*devRun{}}
	for _, d := range s.checkDevs() {
		dr := &devRun{Ping: d.run(), Services: map[string]runFields{}}
		for _, sv := range d.Services {
			dr.Services[sv.ID] = sv.run()
		}
		cd.Devices[d.ID] = dr
	}
	return json.Marshal(&cd)
}

// loadChecks puts the stored results back on the devices. Called once at
// start; a missing or damaged file only means no results yet.
func (s *Server) loadChecks() {
	b, err := os.ReadFile(s.checksPath())
	if os.IsNotExist(err) {
		return
	}
	var cd checksData
	if err == nil {
		var js []byte
		if js, err = decodeState(b); err == nil {
			err = json.Unmarshal(js, &cd)
		}
	}
	if err != nil {
		log.Printf("%s cannot be read (%v): check results start empty; the configuration is not affected", checksFileName, err)
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	for id, dr := range cd.Devices {
		d := s.checkDevByID(id)
		if d == nil || dr == nil {
			continue
		}
		d.setRun(dr.Ping)
		for _, sv := range d.Services {
			if r, ok := dr.Services[sv.ID]; ok {
				sv.setRun(r)
			}
		}
	}
}

// saveChecks writes the results if they changed (temporary file, then rename).
func (s *Server) saveChecks() {
	s.checksMu.Lock()
	defer s.checksMu.Unlock()
	s.st.mu.Lock()
	if s.st.frozen || !s.checksDirty {
		s.st.mu.Unlock()
		return
	}
	js, err := s.checksJSON()
	s.checksDirty = false
	s.st.mu.Unlock()
	if err == nil {
		var b []byte
		if b, err = encodeState(js); err == nil {
			err = writeAtomic(s.checksPath(), b)
		}
	}
	if err != nil {
		log.Printf("save %s: %v", checksFileName, err)
		s.st.mu.Lock()
		s.checksDirty = true
		s.st.mu.Unlock()
	}
}

func (s *Server) checksLoop(stop <-chan struct{}) {
	t := time.NewTicker(checksEvery)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			s.saveChecks()
		}
	}
}
