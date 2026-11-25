package master

// End-of-support dates of Debian and Ubuntu releases (distro-info-data),
// and of Rocky Linux, AlmaLinux, RHEL, SLES, openSUSE Leap and Alpine
// (endoflife.date). For RHEL and SLES the end of normal support counts, not
// the paid extended support. openSUSE Tumbleweed is rolling: no end.

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
	eolBase        = "https://endoflife.date/api"
	osInfoRetry    = 6 * time.Hour
	osInfoSoon     = 183 * 24 * time.Hour
	maxOSInfoBytes = 1 << 20
)

type osRelease struct {
	Version string `json:"version"`
	Release string `json:"release"`
	EOL     string `json:"eol"`
	EOLLTS  string `json:"eol_lts"`
	EOLESM  string `json:"eol_esm"`
	// Full: Rocky Linux, AlmaLinux, RHEL: end of full support (then
	// maintenance until EOL).
	Full string `json:"full,omitempty"`
}

type osInfoData struct {
	FetchedAt int64       `json:"fetched_at"`
	Debian    []osRelease `json:"debian"`
	Ubuntu    []osRelease `json:"ubuntu"`
	// EL: per os_id (rocky, almalinux, rhel), from endoflife.date.
	EL map[string][]osRelease `json:"el,omitempty"`
}

// eolProduct: an OS whose dates come from endoflife.date. parts: how much
// of VERSION_ID names the release ("10.3" → "10"; "15.6"; "3.20.3" →
// "3.20"). full: it has a full-support phase before maintenance (EL).
type eolProduct struct {
	product string
	parts   int
	full    bool
}

// elProducts: os_id → endoflife.date product (the EL map of osInfoData
// holds all of them, by os_id).
var elProducts = map[string]eolProduct{
	"rocky":         {"rocky-linux", 1, true},
	"almalinux":     {"almalinux", 1, true},
	"rhel":          {"rhel", 1, true},
	"sles":          {"sles", 2, false},
	"opensuse-leap": {"opensuse", 2, false},
	"alpine":        {"alpine-linux", 2, false},
}

func isEL(osID string) bool { _, ok := elProducts[osID]; return ok }

// releaseOf: the part of VERSION_ID that names the release.
func releaseOf(version string, parts int) string {
	p := strings.Split(version, ".")
	if len(p) < parts {
		return version
	}
	return strings.Join(p[:parts], ".")
}

type osInfoFile struct {
	Data    *osInfoData `json:"data"`
	LastTry int64       `json:"last_try"`
	Error   string      `json:"error"`
}

type osInfo struct {
	mu    sync.Mutex
	f     osInfoFile
	fetch sync.Mutex
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
		}
	}
}

func (s *Server) refreshOSInfo() error {
	s.osi.fetch.Lock()
	defer s.osi.fetch.Unlock()
	s.osi.mu.Lock()
	var old map[string][]osRelease
	if s.osi.f.Data != nil {
		old = s.osi.f.Data.EL
	}
	s.osi.mu.Unlock()
	data, err := s.fetchOSInfo(old)
	s.osi.mu.Lock()
	s.osi.f.LastTry = time.Now().Unix()
	if data != nil { // also when only endoflife.date failed: its old dates stay
		s.osi.f.Data = data
	}
	s.osi.f.Error = ""
	if err != nil {
		s.osi.f.Error = err.Error()
	}
	f := s.osi.f
	s.osi.mu.Unlock()
	if b, mErr := json.MarshalIndent(f, "", "  "); mErr == nil {
		tmp := s.osInfoPath() + ".tmp"
		if os.WriteFile(tmp, b, 0o600) == nil {
			os.Rename(tmp, s.osInfoPath())
		}
	}
	if data != nil {
		s.decideOSSupportAll()
	}
	return err
}

func (s *Server) eolBase() string {
	if s.cfg.EOLURL != "" {
		return strings.TrimRight(s.cfg.EOLURL, "/")
	}
	return eolBase
}

// fetchOSInfo: nil when Debian's data failed; with an error but data when
// only endoflife.date failed (the old EL dates are kept).
func (s *Server) fetchOSInfo(oldEL map[string][]osRelease) (*osInfoData, error) {
	hc := &http.Client{Timeout: 60 * time.Second}
	get := func(name string) ([]byte, error) {
		return s.osInfoGet(hc, s.osInfoBase()+"/"+name, name)
	}
	deb, err := get("debian.csv")
	if err != nil {
		return nil, err
	}
	ubu, err := get("ubuntu.csv")
	if err != nil {
		return nil, err
	}
	d := &osInfoData{FetchedAt: time.Now().Unix(), EL: map[string][]osRelease{}}
	if d.Debian, err = parseDistroInfo(deb, "debian"); err != nil {
		return nil, fmt.Errorf("debian.csv: %w", err)
	}
	if d.Ubuntu, err = parseDistroInfo(ubu, "ubuntu"); err != nil {
		return nil, fmt.Errorf("ubuntu.csv: %w", err)
	}
	var elErr error
	for id, p := range elProducts {
		product := p.product
		b, err := s.osInfoGet(hc, s.eolBase()+"/"+product+".json", product)
		var list []osRelease
		if err == nil {
			list, err = parseEOL(b)
		}
		if err != nil {
			if elErr == nil {
				elErr = fmt.Errorf("endoflife.date %s: %w", product, err)
			}
			list = oldEL[id]
		}
		if len(list) > 0 {
			d.EL[id] = list
		}
	}
	return d, elErr
}

func (s *Server) osInfoGet(hc *http.Client, u, name string) ([]byte, error) {
	resp, err := hc.Get(u)
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

var elCycleRe = regexp.MustCompile(`^\d{1,2}(\.\d{1,2})?$`)

// parseEOL: endoflife.date releases. "eol": end of maintenance (the end
// that counts), "support": end of full support. A date or false (open).
func parseEOL(b []byte) ([]osRelease, error) {
	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, err
	}
	date := func(row map[string]any, k string) (string, error) {
		switch v := row[k].(type) {
		case nil, bool:
			return "", nil
		case string:
			if _, err := time.Parse("2006-01-02", v); err != nil || !dateRe.MatchString(v) {
				return "", fmt.Errorf("bad date %q", v)
			}
			return v, nil
		}
		return "", fmt.Errorf("bad %s", k)
	}
	var out []osRelease
	for _, row := range rows {
		c, _ := row["cycle"].(string)
		if !elCycleRe.MatchString(c) {
			continue // "7 (ELS)" and the like
		}
		r := osRelease{Version: c}
		var err error
		if r.Release, err = date(row, "releaseDate"); err == nil {
			if r.EOL, err = date(row, "eol"); err == nil {
				r.Full, err = date(row, "support")
			}
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", c, err)
		}
		out = append(out, r)
		if len(out) > 100 {
			return nil, errors.New("too many releases")
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no releases")
	}
	return out, nil
}

var (
	dateRe      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	debianVerRe = regexp.MustCompile(`^\d{1,3}(\.\d{1,2})?$`)
	ubuntuVerRe = regexp.MustCompile(`^\d{1,2}\.\d{2}$`)
)

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
				rel.EOL = server
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

type osSupport struct {
	Status   string `json:"status"`
	Phase    string `json:"phase"`
	Until    string `json:"until,omitempty"`
	End      string `json:"end,omitempty"`
	LTSUntil string `json:"lts_until,omitempty"`
	ESMUntil string `json:"esm_until,omitempty"`
	// MaintUntil: Rocky Linux, AlmaLinux, RHEL in full support: maintenance
	// until then.
	MaintUntil string `json:"maint_until,omitempty"`
}

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
	case "opensuse-tumbleweed": // rolling: no end
		return &osSupport{Status: "supported", Phase: "open"}
	default:
		p, ok := elProducts[osID]
		if !ok {
			return nil
		}
		list = d.EL[osID]
		version = releaseOf(version, p.parts)
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
	day := func(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t.Add(24 * time.Hour) }
	o := &osSupport{}
	if isEL(osID) {
		o.End = rel.EOL
		switch {
		case o.End == "":
			o.Phase = "open"
		case rel.Full != "" && now.Before(day(rel.Full)):
			o.Phase, o.Until, o.MaintUntil = "full", rel.Full, rel.EOL
		case now.Before(day(o.End)) && elProducts[osID].full:
			o.Phase, o.Until = "maintenance", o.End
		case now.Before(day(o.End)):
			o.Phase, o.Until = "until", o.End // SUSE, Alpine: one support period
		default:
			o.Phase = "ended"
		}
	} else if osID == "debian" {
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

func (s *Server) osInfoStatus(days int) map[string]any {
	s.osi.mu.Lock()
	f := s.osi.f
	s.osi.mu.Unlock()
	out := map[string]any{"source": s.osInfoBase(), "source_el": s.eolBase(), "last_try": f.LastTry, "error": f.Error}
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

func (s *Server) handleOSInfoCheck(w http.ResponseWriter, r *http.Request) {
	err := s.refreshOSInfo()
	s.audit.write(actor(r), clientIP(r), "osinfo.check", "", map[bool]string{true: "OS support dates updated", false: fmt.Sprint("failed: ", err)}[err == nil])
	writeJSON(w, map[string]any{"os_info": s.osInfoStatus(s.osInfoDays())})
}
