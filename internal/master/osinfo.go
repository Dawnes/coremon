package master

// OS support status: the end-of-support dates of Debian and Ubuntu releases,
// looked up from Debian's distro-info-data (the files the distributions' own
// tools use) and stored on the master. Not signed, so they are checked
// strictly and only ever affect the EOL badges.

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	osInfoBase     = "https://salsa.debian.org/debian/distro-info-data/-/raw/main"
	osInfoRetry    = 6 * time.Hour        // after a failed lookup
	osInfoSoon     = 183 * 24 * time.Hour // "EOL soon": within about 6 months
	maxOSInfoBytes = 1 << 20
)

// osRelease is one release from debian.csv or ubuntu.csv. Dates are
// YYYY-MM-DD, "" if not known yet.
type osRelease struct {
	Version string `json:"version"` // "12", "24.04"
	Release string `json:"release"`
	EOL     string `json:"eol"`     // Debian: security support; Ubuntu: standard support
	EOLLTS  string `json:"eol_lts"` // Debian LTS
	EOLESM  string `json:"eol_esm"` // Ubuntu ESM (Ubuntu Pro)
}

type osInfoData struct {
	FetchedAt int64       `json:"fetched_at"`
	Debian    []osRelease `json:"debian"`
	Ubuntu    []osRelease `json:"ubuntu"`
}

type osInfoFile struct {
	Data    *osInfoData `json:"data"`
	LastTry int64       `json:"last_try"`
	Error   string      `json:"error"`
}

type osInfo struct {
	mu    sync.Mutex
	f     osInfoFile
	fetch sync.Mutex // one lookup at a time
	kick  chan struct{}
}

func (s *Server) osInfoPath() string { return filepath.Join(s.cfg.DataDir, "os-support.json") }

func (s *Server) osInfoBase() string {
	if s.cfg.OSInfoURL != "" {
		return strings.TrimRight(s.cfg.OSInfoURL, "/")
	}
	return osInfoBase
}

func (s *Server) loadOSInfo() {
	var f osInfoFile
	if b, err := os.ReadFile(s.osInfoPath()); err == nil && json.Unmarshal(b, &f) == nil {
		s.osi.mu.Lock()
		s.osi.f = f
		s.osi.mu.Unlock()
	}
}

func (s *Server) kickOSInfo() {
	select {
	case s.osi.kick <- struct{}{}:
	default:
	}
}

func (s *Server) osInfoDays() int {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	return s.st.st.Settings.OSInfoDays
}

// osInfoDue: no data yet or older than the interval; after a failure, retry
// after osInfoRetry.
func osInfoDue(f osInfoFile, days int, now time.Time) bool {
	if f.Error != "" && now.Sub(time.Unix(f.LastTry, 0)) < osInfoRetry {
		return false
	}
	return f.Data == nil || now.Sub(time.Unix(f.Data.FetchedAt, 0)) >= time.Duration(days)*24*time.Hour
}

func (s *Server) osInfoLoop(stop <-chan struct{}) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		days := s.osInfoDays()
		s.osi.mu.Lock()
		due := osInfoDue(s.osi.f, days, time.Now())
		s.osi.mu.Unlock()
		if due {
			if err := s.refreshOSInfo(); err != nil {
				log.Printf("OS support lookup: %v (retrying in %s)", err, osInfoRetry)
			}
		}
		select {
		case <-stop:
			return
		case <-t.C:
		case <-s.osi.kick:
			// A changed interval: re-evaluate now.
		}
	}
}

// refreshOSInfo looks the dates up now. On failure the previous data stays.
func (s *Server) refreshOSInfo() error {
	s.osi.fetch.Lock()
	defer s.osi.fetch.Unlock()
	data, err := s.fetchOSInfo()
	s.osi.mu.Lock()
	s.osi.f.LastTry = time.Now().Unix()
	if err != nil {
		s.osi.f.Error = err.Error()
	} else {
		s.osi.f.Data, s.osi.f.Error = data, ""
	}
	f := s.osi.f
	s.osi.mu.Unlock()
	if b, mErr := json.MarshalIndent(f, "", "  "); mErr == nil {
		tmp := s.osInfoPath() + ".tmp"
		if os.WriteFile(tmp, b, 0o600) == nil {
			os.Rename(tmp, s.osInfoPath())
		}
	}
	return err
}

func (s *Server) fetchOSInfo() (*osInfoData, error) {
	hc := &http.Client{Timeout: 60 * time.Second}
	get := func(name string) ([]byte, error) {
		resp, err := hc.Get(s.osInfoBase() + "/" + name)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: %s", name, resp.Status)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxOSInfoBytes+1))
		if err == nil && len(b) > maxOSInfoBytes {
			err = fmt.Errorf("%s: too large", name)
		}
		return b, err
	}
	deb, err := get("debian.csv")
	if err != nil {
		return nil, err
	}
	ubu, err := get("ubuntu.csv")
	if err != nil {
		return nil, err
	}
	d := &osInfoData{FetchedAt: time.Now().Unix()}
	if d.Debian, err = parseDistroInfo(deb, "debian"); err != nil {
		return nil, fmt.Errorf("debian.csv: %w", err)
	}
	if d.Ubuntu, err = parseDistroInfo(ubu, "ubuntu"); err != nil {
		return nil, fmt.Errorf("ubuntu.csv: %w", err)
	}
	return d, nil
}

var (
	dateRe      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	debianVerRe = regexp.MustCompile(`^\d{1,3}(\.\d{1,2})?$`) // old releases: "1.1" ... "3.1"
	ubuntuVerRe = regexp.MustCompile(`^\d{1,2}\.\d{2}$`) // "4.10" ... "26.04"
)

// parseDistroInfo reads debian.csv or ubuntu.csv strictly: known columns,
// valid dates, sane versions; rows without a version (sid) are skipped.
func parseDistroInfo(b []byte, distro string) ([]osRelease, error) {
	r := csv.NewReader(strings.NewReader(string(b)))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, errors.New("no releases")
	}
	col := map[string]int{}
	for i, h := range rows[0] {
		col[strings.TrimSpace(h)] = i
	}
	need := map[string][]string{"debian": {"version", "release", "eol", "eol-lts"}, "ubuntu": {"version", "release", "eol", "eol-server", "eol-esm"}}[distro]
	for _, c := range need {
		if _, ok := col[c]; !ok {
			return nil, fmt.Errorf("column %q missing", c)
		}
	}
	field := func(row []string, name string) (string, error) {
		i, ok := col[name]
		if !ok || i >= len(row) {
			return "", nil
		}
		v := strings.TrimSpace(row[i])
		if v != "" && !dateRe.MatchString(v) {
			return "", fmt.Errorf("bad date %q", v)
		}
		if v != "" {
			if _, err := time.Parse("2006-01-02", v); err != nil {
				return "", fmt.Errorf("bad date %q", v)
			}
		}
		return v, nil
	}
	var out []osRelease
	for _, row := range rows[1:] {
		v := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(row[0]), " LTS"))
		if v == "" {
			continue
		}
		if (distro == "debian" && !debianVerRe.MatchString(v)) || (distro == "ubuntu" && !ubuntuVerRe.MatchString(v)) {
			return nil, fmt.Errorf("bad version %q", v)
		}
		rel := osRelease{Version: v}
		var err error
		if rel.Release, err = field(row, "release"); err != nil {
			return nil, err
		}
		if rel.EOL, err = field(row, "eol"); err != nil {
			return nil, err
		}
		if distro == "debian" {
			rel.EOLLTS, err = field(row, "eol-lts")
		} else {
			var server string
			if server, err = field(row, "eol-server"); err == nil && server != "" {
				rel.EOL = server // server support is what matters here
			}
			if err == nil {
				rel.EOLESM, err = field(row, "eol-esm")
			}
		}
		if err != nil {
			return nil, err
		}
		out = append(out, rel)
		if len(out) > 500 {
			return nil, errors.New("too many releases")
		}
	}
	if len(out) < 3 {
		return nil, errors.New("too few releases")
	}
	return out, nil
}

// osSupport is a system's support status for the UI.
type osSupport struct {
	Status   string `json:"status"`              // supported | soon | expired
	Phase    string `json:"phase"`               // security | lts | standard | ended | open (no end date yet)
	Until    string `json:"until,omitempty"`     // end of the current phase (YYYY-MM-DD)
	End      string `json:"end,omitempty"`       // end of free support (Debian LTS, Ubuntu standard)
	LTSUntil string `json:"lts_until,omitempty"` // Debian: LTS end, while still in security support
	ESMUntil string `json:"esm_until,omitempty"` // Ubuntu: ESM end (Ubuntu Pro)
}

// supportFor works out a system's support status; nil if unknown.
func (s *Server) supportFor(osID, version string, now time.Time) *osSupport {
	s.osi.mu.Lock()
	d := s.osi.f.Data
	s.osi.mu.Unlock()
	if d == nil {
		return nil
	}
	return supportStatus(d, osID, version, now)
}

func supportStatus(d *osInfoData, osID, version string, now time.Time) *osSupport {
	var list []osRelease
	switch osID {
	case "debian":
		list = d.Debian
	case "ubuntu":
		list = d.Ubuntu
	default:
		return nil
	}
	var rel *osRelease
	for i := range list {
		if list[i].Version == version {
			rel = &list[i]
		}
	}
	if rel == nil || version == "" {
		return nil
	}
	day := func(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t.Add(24 * time.Hour) } // end of that day
	o := &osSupport{}
	if osID == "debian" {
		o.End = rel.EOLLTS
		if o.End == "" {
			o.End = rel.EOL
		}
		switch {
		case o.End == "":
			o.Phase = "open"
		case rel.EOL != "" && now.Before(day(rel.EOL)):
			o.Phase, o.Until, o.LTSUntil = "security", rel.EOL, rel.EOLLTS
		case now.Before(day(o.End)):
			o.Phase, o.Until = "lts", o.End
		default:
			o.Phase = "ended"
		}
	} else {
		o.End, o.ESMUntil = rel.EOL, rel.EOLESM
		switch {
		case o.End == "":
			o.Phase = "open"
		case now.Before(day(o.End)):
			o.Phase, o.Until = "standard", o.End
		default:
			o.Phase = "ended"
		}
	}
	switch {
	case o.Phase == "ended":
		o.Status = "expired"
	case o.End != "" && day(o.End).Sub(now) <= osInfoSoon:
		o.Status = "soon"
	default:
		o.Status = "supported"
	}
	return o
}

// osInfoStatus is shown under Settings > General. days: the interval.
func (s *Server) osInfoStatus(days int) map[string]any {
	s.osi.mu.Lock()
	f := s.osi.f
	s.osi.mu.Unlock()
	out := map[string]any{"source": s.osInfoBase(), "last_try": f.LastTry, "error": f.Error}
	var next int64
	if f.Data != nil {
		out["fetched_at"] = f.Data.FetchedAt
		next = f.Data.FetchedAt + int64(days)*86400
	}
	if f.Error != "" && f.LastTry+int64(osInfoRetry/time.Second) > next {
		next = f.LastTry + int64(osInfoRetry/time.Second)
	}
	out["next_at"] = next
	return out
}

// handleOSInfoCheck looks the dates up now (administrators).
func (s *Server) handleOSInfoCheck(w http.ResponseWriter, r *http.Request) {
	err := s.refreshOSInfo()
	s.audit.write(actor(r), clientIP(r), "osinfo.check", "", map[bool]string{true: "OS support dates updated", false: fmt.Sprint("failed: ", err)}[err == nil])
	writeJSON(w, map[string]any{"os_info": s.osInfoStatus(s.osInfoDays())})
}
