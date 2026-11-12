package master

// Known vulnerabilities (CVEs) of the tracked applications.
//
// The agents report each application's source package, its version and the
// package source it came from. For applications from the distribution
// itself, the master asks OSV.dev which advisories affect that exact
// version (OSV uses Debian's and Ubuntu's own data, so backported fixes
// count). Only the release ("Debian:12"), source package names and
// versions are sent; no host names or addresses.
//
// Severity, once per CVE: a CVSS 3 vector in the OSV record; otherwise the
// distribution's own word (Debian urgency, Ubuntu priority); NVD fills in a
// CVSS score where OSV has none. CISA's list of actively exploited CVEs
// (KEV) and FIRST's EPSS (chance of exploitation) are added on top.
//
// Everything downloaded is public data, kept in <data>/vulns.json.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"coremon/internal/feature/apps"
	"coremon/internal/feature/updates"
	"coremon/internal/proto"
)

const (
	osvBase   = "https://api.osv.dev"
	nvdBase   = "https://services.nvd.nist.gov/rest/json/cves/2.0"
	kevURL    = "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json"
	epssBase  = "https://api.first.org/data/v1/epss"
	osvLink   = "https://osv.dev/vulnerability/"
	userAgent = "coremon"

	vulnRetry      = 6 * time.Hour   // after a failed check
	vulnQuickEvery = 2 * time.Minute // new versions (an upgrade, a new system) are looked up this soon
	vulnKEVEvery   = 24 * time.Hour
	nvdNoneRetry   = 7 * 24 * time.Hour // NVD had no score yet: ask again after this
	nvdGapNoKey    = 6500 * time.Millisecond
	nvdGapKey      = 800 * time.Millisecond
	maxVulnBody    = 32 << 20
	maxVulnRecord  = 4 << 20
	osvBatch       = 1000
	defaultVulnDay = 1
)

// vulnChoices are the allowed check intervals in days.
var vulnChoices = []int{1, 7, 30, 90}

func validVulnDays(d int) bool {
	for _, c := range vulnChoices {
		if d == c {
			return true
		}
	}
	return false
}

var (
	cveRe    = regexp.MustCompile(`^CVE-\d{4}-\d{4,7}$`)
	osvIDRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{2,80}$`)
	nvdKeyRe = regexp.MustCompile(`^[A-Za-z0-9-]{20,64}$`)
)

// ---- stored data ----

type vulnAff struct {
	Eco    string      `json:"e"`
	Pkg    string      `json:"p"`
	Events [][2]string `json:"ev"`          // in order: ["introduced","0"], ["fixed","1.2-3"], ...
	Word   string      `json:"w,omitempty"` // the distribution's word for this package (Debian urgency)
}

type vulnRec struct {
	Modified string    `json:"m"`
	CVE      string    `json:"c"`
	Score    float64   `json:"s,omitempty"`  // from a CVSS 3 vector in the record
	Scored   bool      `json:"sc,omitempty"` //
	Word     string    `json:"w,omitempty"`  // record-wide word (Ubuntu priority)
	Aff      []vulnAff `json:"a"`
}

type cveMeta struct {
	Score  float64 `json:"score,omitempty"`
	Scored bool    `json:"scored,omitempty"`
	Src    string  `json:"src,omitempty"` // osv | nvd
	NVDAt  int64   `json:"nvd_at,omitempty"`
	EPSS   float64 `json:"epss,omitempty"`
	Pct    float64 `json:"pct,omitempty"` // EPSS percentile
}

type vulnQuery struct {
	At  int64    `json:"at"`
	IDs []string `json:"ids"`
}

type vulnData struct {
	Records map[string]*vulnRec   `json:"records"`
	Queries map[string]*vulnQuery `json:"queries"` // "eco|package|version"
	CVEs    map[string]*cveMeta   `json:"cves"`
	Ecos    map[string]bool       `json:"ecos"` // release -> OSV has data for it
	KEV     map[string]bool       `json:"kev"`
	KEVAt   int64                 `json:"kev_at"`
	EPSSAt  int64                 `json:"epss_at"`
	LastRun int64                 `json:"last_run"` // last complete check
	LastTry int64                 `json:"last_try"`
	Error   string                `json:"error"`
	NVDErr  string                `json:"nvd_error"`
}

type vulnStore struct {
	mu   sync.Mutex
	d    vulnData
	run  sync.Mutex // one check at a time
	kick chan struct{}
	nvd  chan struct{}
}

func newVulnData() vulnData {
	return vulnData{Records: map[string]*vulnRec{}, Queries: map[string]*vulnQuery{}, CVEs: map[string]*cveMeta{}, Ecos: map[string]bool{}, KEV: map[string]bool{}}
}

func (s *Server) vulnPath() string { return filepath.Join(s.cfg.DataDir, "vulns.json") }

func (s *Server) loadVulns() {
	d := newVulnData()
	if b, err := os.ReadFile(s.vulnPath()); err == nil {
		if json.Unmarshal(b, &d) != nil {
			d = newVulnData()
		}
	}
	nd := newVulnData()
	if d.Records == nil {
		d.Records = nd.Records
	}
	if d.Queries == nil {
		d.Queries = nd.Queries
	}
	if d.CVEs == nil {
		d.CVEs = nd.CVEs
	}
	if d.Ecos == nil {
		d.Ecos = nd.Ecos
	}
	if d.KEV == nil {
		d.KEV = nd.KEV
	}
	s.vul.mu.Lock()
	s.vul.d = d
	s.vul.mu.Unlock()
}

// saveVulns writes the file. Caller holds s.vul.mu.
func (s *Server) saveVulnsLocked() {
	b, err := json.Marshal(s.vul.d)
	if err != nil {
		return
	}
	tmp := s.vulnPath() + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, s.vulnPath())
	}
}

func (s *Server) kickVulns() {
	select {
	case s.vul.kick <- struct{}{}:
	default:
	}
}

func (s *Server) kickNVD() {
	select {
	case s.vul.nvd <- struct{}{}:
	default:
	}
}

func (s *Server) vulnDays() int {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	return s.st.st.Settings.VulnDays
}

func (s *Server) base(override, def string) string {
	if override != "" {
		return strings.TrimRight(override, "/")
	}
	return def
}

// ---- what to look up ----

// osvEcosystem is OSV's name for a system's release, "" if not Debian or
// Ubuntu: "Debian:12", "Ubuntu:24.04:LTS", "Ubuntu:25.10".
func osvEcosystem(f proto.Facts) string {
	switch f.OSID {
	case "debian":
		v := f.OSVersion
		if i := strings.IndexByte(v, '.'); i >= 0 {
			v = v[:i]
		}
		if v == "" || len(v) > 3 || strings.Trim(v, "0123456789") != "" {
			return ""
		}
		return "Debian:" + v
	case "ubuntu":
		v := f.OSVersion
		if !ubuntuVerRe.MatchString(v) {
			return ""
		}
		yy, _ := strconv.Atoi(v[:len(v)-3])
		if strings.HasSuffix(v, ".04") && yy%2 == 0 {
			return "Ubuntu:" + v + ":LTS"
		}
		return "Ubuntu:" + v
	}
	return ""
}

// fromDistro: the package comes from the system's own distribution.
func fromDistro(osID, origin string) bool {
	switch osID {
	case "debian":
		return origin == "Debian"
	case "ubuntu":
		return strings.HasPrefix(origin, "Ubuntu")
	}
	return false
}

func vulnKey(eco, pkg, ver string) string { return eco + "|" + pkg + "|" + ver }

// appsOf reads an agent's application scan. Caller holds s.st.mu.
func appsOf(a *Agent) *apps.State {
	raw, ok := a.State["apps"]
	if !ok {
		return nil
	}
	var st apps.State
	if json.Unmarshal(raw, &st) != nil {
		return nil
	}
	return &st
}

// wantedKeys: what the current systems need looked up. Caller holds s.st.mu.
func (s *Server) wantedKeysLocked() map[string]bool {
	out := map[string]bool{}
	for _, a := range s.st.st.Agents {
		if a.Status == StatusPending || s.belowMin(a) {
			continue
		}
		eco := osvEcosystem(a.Facts)
		st := appsOf(a)
		if eco == "" || st == nil {
			continue
		}
		for _, ap := range st.Apps {
			if ap.Installed && ap.Source != "" && ap.SourceVersion != "" && fromDistro(a.Facts.OSID, ap.Origin) {
				out[vulnKey(eco, ap.Source, ap.SourceVersion)] = true
			}
		}
	}
	return out
}

// ---- the check ----

func (s *Server) vulnLoop(stop <-chan struct{}) {
	t := time.NewTicker(vulnQuickEvery)
	defer t.Stop()
	for {
		s.vulnTick(false)
		select {
		case <-stop:
			return
		case <-t.C:
		case <-s.vul.kick:
		}
	}
}

// vulnTick runs a full check when one is due, otherwise only looks up what
// is new (an upgraded application, a new system) and refreshes the KEV list
// once a day.
func (s *Server) vulnTick(force bool) error {
	days := s.vulnDays()
	s.st.mu.Lock()
	want := s.wantedKeysLocked()
	s.st.mu.Unlock()
	now := time.Now()
	s.vul.mu.Lock()
	d := &s.vul.d
	failedLately := d.Error != "" && now.Sub(time.Unix(d.LastTry, 0)) < vulnRetry
	full := force || (!failedLately && len(want) > 0 && now.Sub(time.Unix(d.LastRun, 0)) >= time.Duration(days)*24*time.Hour)
	var missing []string
	for k := range want {
		if d.Queries[k] == nil {
			missing = append(missing, k)
		}
	}
	kevDue := now.Sub(time.Unix(d.KEVAt, 0)) >= vulnKEVEvery
	s.vul.mu.Unlock()
	switch {
	case full:
		return s.runVulnCheck(want, true)
	case len(missing) > 0 && !failedLately:
		m := map[string]bool{}
		for _, k := range missing {
			m[k] = true
		}
		return s.runVulnCheck(m, false)
	case kevDue && len(want) > 0 && !failedLately:
		s.vul.run.Lock()
		defer s.vul.run.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err := s.refreshKEV(ctx)
		s.vul.mu.Lock()
		if err != nil {
			s.vul.d.Error, s.vul.d.LastTry = "CISA list: "+err.Error(), now.Unix()
		}
		s.saveVulnsLocked()
		s.vul.mu.Unlock()
		return err
	}
	return nil
}

// runVulnCheck looks up keys at OSV and fetches new or changed records. A
// full check also drops what no system needs any more and refreshes KEV and
// EPSS.
func (s *Server) runVulnCheck(keys map[string]bool, full bool) (err error) {
	s.vul.run.Lock()
	defer s.vul.run.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	started := time.Now()
	defer func() {
		s.vul.mu.Lock()
		s.vul.d.LastTry = started.Unix()
		if err != nil {
			s.vul.d.Error = err.Error()
		} else {
			s.vul.d.Error = ""
			if full {
				s.vul.d.LastRun = started.Unix()
			}
		}
		s.saveVulnsLocked()
		s.vul.mu.Unlock()
		if err != nil {
			log.Printf("vulnerability check: %v", err)
		}
		s.kickNVD()
	}()

	// 1. Releases: does OSV have data for them? (An unknown name gives no
	// results, which must not read as "no vulnerabilities".)
	ecos := map[string]bool{}
	for k := range keys {
		ecos[strings.SplitN(k, "|", 2)[0]] = true
	}
	s.vul.mu.Lock()
	var probe []string
	for e := range ecos {
		if _, known := s.vul.d.Ecos[e]; full || !known {
			probe = append(probe, e)
		}
	}
	s.vul.mu.Unlock()
	if len(probe) > 0 {
		qs := make([]osvQuery, len(probe))
		for i, e := range probe {
			qs[i] = osvQuery{Eco: e, Pkg: "openssl", Ver: "0.1"} // every Debian and Ubuntu release ships openssl
		}
		res, err := s.osvQueryBatch(ctx, qs)
		if err != nil {
			return fmt.Errorf("OSV: %w", err)
		}
		s.vul.mu.Lock()
		for i, e := range probe {
			s.vul.d.Ecos[e] = len(res[i]) > 0
		}
		s.vul.mu.Unlock()
	}

	// 2. The lookups.
	s.vul.mu.Lock()
	var qs []osvQuery
	for k := range keys {
		p := strings.SplitN(k, "|", 3)
		if len(p) == 3 && s.vul.d.Ecos[p[0]] {
			qs = append(qs, osvQuery{Eco: p[0], Pkg: p[1], Ver: p[2]})
		}
	}
	s.vul.mu.Unlock()
	sort.Slice(qs, func(i, j int) bool { return qs[i].key() < qs[j].key() })
	results, err := s.osvQueryBatch(ctx, qs)
	if err != nil {
		return fmt.Errorf("OSV: %w", err)
	}

	// 3. Records: only the per-CVE ones (DEBIAN-CVE-…, UBUNTU-CVE-…);
	// advisories (DSA, DLA, USN) repeat what those say.
	need := map[string]string{} // id -> modified
	now := time.Now().Unix()
	s.vul.mu.Lock()
	for i, q := range qs {
		var ids []string
		for _, v := range results[i] {
			if !strings.Contains(v.ID, "CVE-") || !osvIDRe.MatchString(v.ID) {
				continue
			}
			ids = append(ids, v.ID)
			if r := s.vul.d.Records[v.ID]; r == nil || r.Modified != v.Modified {
				need[v.ID] = v.Modified
			}
		}
		sort.Strings(ids)
		s.vul.d.Queries[q.key()] = &vulnQuery{At: now, IDs: ids}
	}
	s.vul.mu.Unlock()
	if err := s.fetchRecords(ctx, need); err != nil {
		return fmt.Errorf("OSV: %w", err)
	}

	if !full {
		return nil
	}
	// 4. Forget what no system needs any more.
	s.vul.mu.Lock()
	used := map[string]bool{}
	for k, q := range s.vul.d.Queries {
		if !keys[k] {
			delete(s.vul.d.Queries, k)
			continue
		}
		for _, id := range q.IDs {
			used[id] = true
		}
	}
	cves := map[string]bool{}
	for id, r := range s.vul.d.Records {
		if !used[id] {
			delete(s.vul.d.Records, id)
			continue
		}
		cves[r.CVE] = true
	}
	for c := range s.vul.d.CVEs {
		if !cves[c] {
			delete(s.vul.d.CVEs, c)
		}
	}
	list := make([]string, 0, len(cves))
	for c := range cves {
		list = append(list, c)
	}
	s.vul.mu.Unlock()
	sort.Strings(list)

	// 5. Exploited (KEV) and chance of exploitation (EPSS). Failures here do
	// not spoil the check: the previous lists stay.
	if err := s.refreshKEV(ctx); err != nil {
		log.Printf("vulnerability check: CISA list: %v", err)
	}
	if err := s.refreshEPSS(ctx, list); err != nil {
		log.Printf("vulnerability check: EPSS: %v", err)
	}
	return nil
}

// ---- OSV ----

type osvQuery struct{ Eco, Pkg, Ver string }

func (q osvQuery) key() string { return vulnKey(q.Eco, q.Pkg, q.Ver) }

type osvHit struct {
	ID       string `json:"id"`
	Modified string `json:"modified"`
}

func (s *Server) vulnGet(ctx context.Context, method, u string, body any, hdr map[string]string, limit int64, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rd = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	hc := &http.Client{Timeout: 90 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return resp.StatusCode, fmt.Errorf("%s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return resp.StatusCode, err
	}
	if int64(len(b)) > limit {
		return resp.StatusCode, errors.New("answer too large")
	}
	if err := json.Unmarshal(b, out); err != nil {
		return resp.StatusCode, errors.New("answer is not valid JSON")
	}
	return resp.StatusCode, nil
}

// osvQueryBatch asks which records affect each package version, following
// page tokens where a query has more results than fit in one answer.
func (s *Server) osvQueryBatch(ctx context.Context, qs []osvQuery) ([][]osvHit, error) {
	out := make([][]osvHit, len(qs))
	type q struct {
		Package struct {
			Name      string `json:"name"`
			Ecosystem string `json:"ecosystem"`
		} `json:"package"`
		Version   string `json:"version"`
		PageToken string `json:"page_token,omitempty"`
	}
	pending := make([]int, len(qs)) // indexes still to (re)ask
	tokens := make([]string, len(qs))
	for i := range qs {
		pending[i] = i
	}
	for round := 0; len(pending) > 0; round++ {
		if round > 50 {
			return nil, errors.New("too many result pages")
		}
		var next []int
		for start := 0; start < len(pending); start += osvBatch {
			idx := pending[start:min(start+osvBatch, len(pending))]
			body := struct {
				Queries []q `json:"queries"`
			}{}
			for _, i := range idx {
				var x q
				x.Package.Name, x.Package.Ecosystem, x.Version, x.PageToken = qs[i].Pkg, qs[i].Eco, qs[i].Ver, tokens[i]
				body.Queries = append(body.Queries, x)
			}
			var resp struct {
				Results []struct {
					Vulns []osvHit `json:"vulns"`
					Next  string   `json:"next_page_token"`
				} `json:"results"`
			}
			if _, err := s.vulnGet(ctx, "POST", s.base(s.cfg.OSVURL, osvBase)+"/v1/querybatch", body, nil, maxVulnBody, &resp); err != nil {
				return nil, err
			}
			if len(resp.Results) != len(idx) {
				return nil, errors.New("answer does not match the questions")
			}
			for j, i := range idx {
				out[i] = append(out[i], resp.Results[j].Vulns...)
				if len(out[i]) > 20000 {
					return nil, errors.New("too many results")
				}
				if t := resp.Results[j].Next; t != "" {
					tokens[i] = t
					next = append(next, i)
				}
			}
		}
		pending = next
	}
	return out, nil
}

type osvRecord struct {
	ID       string   `json:"id"`
	Modified string   `json:"modified"`
	Aliases  []string `json:"aliases"`
	Upstream []string `json:"upstream"`
	Severity []struct {
		Type  string `json:"type"`
		Score string `json:"score"`
	} `json:"severity"`
	Affected []struct {
		Package struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
		} `json:"package"`
		Ranges []struct {
			Type   string              `json:"type"`
			Events []map[string]string `json:"events"`
		} `json:"ranges"`
		EcoSpecific map[string]any `json:"ecosystem_specific"`
	} `json:"affected"`
	DBSpecific map[string]any `json:"database_specific"`
}

// reduceRecord keeps what Coremon needs from an OSV record.
func reduceRecord(r osvRecord) (*vulnRec, bool) {
	cve := ""
	if i := strings.Index(r.ID, "CVE-"); i >= 0 && cveRe.MatchString(r.ID[i:]) {
		cve = r.ID[i:]
	}
	for _, l := range [][]string{r.Upstream, r.Aliases} {
		for _, a := range l {
			if cve == "" && cveRe.MatchString(a) {
				cve = a
			}
		}
	}
	if cve == "" {
		return nil, false
	}
	out := &vulnRec{Modified: r.Modified, CVE: cve}
	for _, sv := range r.Severity {
		switch {
		case strings.HasPrefix(sv.Type, "CVSS_V3"):
			if sc, ok := cvss3Score(sv.Score); ok && (!out.Scored || sc > out.Score) {
				out.Score, out.Scored = sc, true
			}
		case wordLevel(sv.Score) > 0 || minorWord(sv.Score):
			if wordLevel(sv.Score) >= wordLevel(out.Word) {
				out.Word = strings.ToLower(sv.Score)
			}
		}
	}
	if w, ok := r.DBSpecific["severity"].(string); ok && out.Word == "" && (wordLevel(w) > 0 || minorWord(w)) {
		out.Word = strings.ToLower(w)
	}
	for _, a := range r.Affected {
		e := a.Package.Ecosystem
		if !(strings.HasPrefix(e, "Debian:") || strings.HasPrefix(e, "Ubuntu:")) || strings.HasPrefix(e, "Ubuntu:Pro:") {
			continue
		}
		af := vulnAff{Eco: e, Pkg: a.Package.Name}
		for _, k := range []string{"urgency", "severity", "priority"} {
			if w, ok := a.EcoSpecific[k].(string); ok && (wordLevel(w) > 0 || minorWord(w)) && af.Word == "" {
				af.Word = strings.ToLower(w)
			}
		}
		for _, rg := range a.Ranges {
			if rg.Type != "ECOSYSTEM" {
				continue
			}
			for _, ev := range rg.Events {
				for _, k := range []string{"introduced", "fixed", "last_affected"} {
					if v, ok := ev[k]; ok && len(v) < 128 {
						af.Events = append(af.Events, [2]string{k, v})
					}
				}
			}
		}
		out.Aff = append(out.Aff, af)
	}
	return out, true
}

// fetchRecords downloads records, a few at a time.
func (s *Server) fetchRecords(ctx context.Context, need map[string]string) error {
	ids := make([]string, 0, len(need))
	for id := range need {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		failed   int
	)
	sem := make(chan struct{}, 6)
	for _, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(id string) {
			defer wg.Done()
			defer func() { <-sem }()
			var r osvRecord
			code, err := s.vulnGet(ctx, "GET", s.base(s.cfg.OSVURL, osvBase)+"/v1/vulns/"+url.PathEscape(id), nil, nil, maxVulnRecord, &r)
			if err == nil && r.ID != id {
				err = errors.New("answer is for another record")
			}
			if err != nil {
				mu.Lock()
				if code != http.StatusNotFound { // a withdrawn record is no error
					failed++
					if firstErr == nil {
						firstErr = fmt.Errorf("record %s: %w", id, err)
					}
				}
				mu.Unlock()
				return
			}
			rec, ok := reduceRecord(r)
			if !ok {
				return
			}
			s.vul.mu.Lock()
			s.vul.d.Records[id] = rec
			m := s.vul.d.CVEs[rec.CVE]
			if m == nil {
				m = &cveMeta{}
				s.vul.d.CVEs[rec.CVE] = m
			}
			// The first CVSS score stays; a later one only fills a gap.
			if rec.Scored && !m.Scored {
				m.Score, m.Scored, m.Src = rec.Score, true, "osv"
			}
			s.vul.mu.Unlock()
		}(id)
	}
	wg.Wait()
	if failed > 0 {
		return fmt.Errorf("%d record(s) could not be fetched, e.g. %w", failed, firstErr)
	}
	return nil
}

// ---- KEV and EPSS ----

func (s *Server) refreshKEV(ctx context.Context) error {
	var resp struct {
		Vulns []struct {
			CVE string `json:"cveID"`
		} `json:"vulnerabilities"`
	}
	if _, err := s.vulnGet(ctx, "GET", s.base(s.cfg.KEVURL, kevURL), nil, nil, maxVulnBody, &resp); err != nil {
		return err
	}
	m := map[string]bool{}
	for _, v := range resp.Vulns {
		if cveRe.MatchString(v.CVE) {
			m[v.CVE] = true
		}
	}
	if len(m) < 100 {
		return fmt.Errorf("only %d entries; not used", len(m))
	}
	s.vul.mu.Lock()
	s.vul.d.KEV, s.vul.d.KEVAt = m, time.Now().Unix()
	s.vul.mu.Unlock()
	return nil
}

func (s *Server) refreshEPSS(ctx context.Context, cves []string) error {
	got := map[string][2]float64{}
	for start := 0; start < len(cves); start += 100 {
		part := cves[start:min(start+100, len(cves))]
		var resp struct {
			Data []struct {
				CVE  string `json:"cve"`
				EPSS string `json:"epss"`
				Pct  string `json:"percentile"`
			} `json:"data"`
		}
		u := s.base(s.cfg.EPSSURL, epssBase) + "?limit=100&cve=" + url.QueryEscape(strings.Join(part, ","))
		if _, err := s.vulnGet(ctx, "GET", u, nil, nil, 4<<20, &resp); err != nil {
			return err
		}
		for _, d := range resp.Data {
			e, err1 := strconv.ParseFloat(d.EPSS, 64)
			p, err2 := strconv.ParseFloat(d.Pct, 64)
			if err1 == nil && err2 == nil && e >= 0 && e <= 1 && p >= 0 && p <= 1 {
				got[d.CVE] = [2]float64{e, p}
			}
		}
	}
	s.vul.mu.Lock()
	for c, m := range s.vul.d.CVEs {
		if v, ok := got[c]; ok {
			m.EPSS, m.Pct = v[0], v[1]
		}
	}
	s.vul.d.EPSSAt = time.Now().Unix()
	s.vul.mu.Unlock()
	return nil
}

// ---- NVD ----

// nvdLoop fills in CVSS scores for CVEs that OSV has none for, one CVE at
// a time within NVD's limits (5 requests per 30 s without a key).
func (s *Server) nvdLoop(stop <-chan struct{}) {
	for {
		id, key := s.nextNVD(time.Now())
		if id == "" {
			select {
			case <-stop:
				return
			case <-s.vul.nvd:
			case <-time.After(30 * time.Minute):
			}
			continue
		}
		wait := nvdGapNoKey
		if key != "" {
			wait = nvdGapKey
		}
		if err := s.lookupNVD(id, key); err != nil {
			s.vul.mu.Lock()
			s.vul.d.NVDErr = err.Error()
			s.vul.mu.Unlock()
			wait = 2 * time.Minute
		} else {
			s.vul.mu.Lock()
			s.vul.d.NVDErr = ""
			s.vul.mu.Unlock()
		}
		select {
		case <-stop:
			return
		case <-time.After(wait):
		}
	}
}

// nextNVD picks a CVE without a CVSS score that is due for a lookup.
func (s *Server) nextNVD(now time.Time) (string, string) {
	s.st.mu.Lock()
	key := s.st.st.NVDKey
	s.st.mu.Unlock()
	s.vul.mu.Lock()
	defer s.vul.mu.Unlock()
	best := ""
	for c, m := range s.vul.d.CVEs {
		if m.Scored || (m.NVDAt != 0 && now.Sub(time.Unix(m.NVDAt, 0)) < nvdNoneRetry) {
			continue
		}
		if best == "" || c > best { // newest CVEs first
			best = c
		}
	}
	return best, key
}

func (s *Server) nvdPending() int {
	now := time.Now()
	s.vul.mu.Lock()
	defer s.vul.mu.Unlock()
	n := 0
	for _, m := range s.vul.d.CVEs {
		if !m.Scored && (m.NVDAt == 0 || now.Sub(time.Unix(m.NVDAt, 0)) >= nvdNoneRetry) {
			n++
		}
	}
	return n
}

func (s *Server) lookupNVD(id, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	type metric struct {
		Type string `json:"type"`
		Data struct {
			Score float64 `json:"baseScore"`
		} `json:"cvssData"`
	}
	var resp struct {
		Vulns []struct {
			CVE struct {
				ID      string              `json:"id"`
				Metrics map[string][]metric `json:"metrics"`
			} `json:"cve"`
		} `json:"vulnerabilities"`
	}
	hdr := map[string]string{}
	if key != "" {
		hdr["apiKey"] = key
	}
	if _, err := s.vulnGet(ctx, "GET", s.base(s.cfg.NVDURL, nvdBase)+"?cveId="+url.QueryEscape(id), nil, hdr, 8<<20, &resp); err != nil {
		return fmt.Errorf("NVD: %w", err)
	}
	score, found := 0.0, false
	for _, v := range resp.Vulns {
		if v.CVE.ID != id {
			continue
		}
		for _, k := range []string{"cvssMetricV31", "cvssMetricV30", "cvssMetricV40", "cvssMetricV2"} {
			list := v.CVE.Metrics[k]
			for _, m := range list { // the NVD's own (Primary) score first
				if m.Type == "Primary" && !found {
					score, found = m.Data.Score, true
				}
			}
			if !found && len(list) > 0 {
				score, found = list[0].Data.Score, true
			}
			if found {
				break
			}
		}
	}
	s.vul.mu.Lock()
	defer s.vul.mu.Unlock()
	m := s.vul.d.CVEs[id]
	if m == nil {
		return nil // no longer needed
	}
	m.NVDAt = time.Now().Unix()
	if found && score >= 0 && score <= 10 && !m.Scored {
		m.Score, m.Scored, m.Src = score, true, "nvd"
	}
	s.saveVulnsLocked()
	return nil
}

// ---- results for the website ----

type cveView struct {
	ID       string  `json:"id"`
	Link     string  `json:"link"`
	Level    int     `json:"level"`           // 0 unknown, 1 low ... 4 critical
	Score    float64 `json:"score,omitempty"` // CVSS
	Scored   bool    `json:"scored,omitempty"`
	Src      string  `json:"src,omitempty"` // where the severity came from: osv | nvd | distro
	KEV      bool    `json:"kev,omitempty"` // actively exploited
	EPSS     float64 `json:"epss,omitempty"`
	Pct      float64 `json:"pct,omitempty"`
	Fixed    string  `json:"fixed,omitempty"` // version that fixes it, "" if none yet
	ByUpdate bool    `json:"by_update,omitempty"`
	Minor    bool    `json:"minor,omitempty"` // the distribution considers it not worth fixing
	Word     string  `json:"word,omitempty"`
}

func (c cveView) red() bool { return c.KEV || (!c.Minor && c.Fixed != "" && c.Level >= 3) }

type appVulns struct {
	// ok | found | pending (not looked up yet) | not_distro (from another
	// package source) | manual (no package source) | old_agent
	Status  string    `json:"status"`
	Origin  string    `json:"origin,omitempty"`
	Fixable int       `json:"fixable"` // not minor, fix released
	Open    int       `json:"open"`    // not minor, no fix yet
	Minor   int       `json:"minor"`
	Red     bool      `json:"red"`
	KEV     int       `json:"kev"`
	CVEs    []cveView `json:"cves,omitempty"`
}

type vulnView struct {
	// checked | pending | unsupported (no data for this release) | old_agent
	Status      string               `json:"status"`
	Badge       int                  `json:"badge"` // red: fixable High/Critical or actively exploited
	CheckedAt   int64                `json:"checked_at,omitempty"`
	Apps        map[string]*appVulns `json:"apps,omitempty"`
	UpdateFixes map[string][]string  `json:"update_fixes,omitempty"` // package -> CVEs its waiting update fixes
}

// vulnsFor builds a system's results. Caller holds s.st.mu (not s.vul.mu).
func (s *Server) vulnsFor(a *Agent, full bool) *vulnView {
	st := appsOf(a)
	if st == nil || a.Status == StatusPending {
		return nil
	}
	eco := osvEcosystem(a.Facts)
	if eco == "" {
		return nil // not Debian or Ubuntu
	}
	var upd map[string]string
	if full {
		if raw, ok := a.State["updates"]; ok {
			var u updates.Snapshot
			if json.Unmarshal(raw, &u) == nil {
				upd = map[string]string{}
				for _, p := range u.Packages {
					upd[p.Name] = p.Available
				}
			}
		}
	}
	s.vul.mu.Lock()
	defer s.vul.mu.Unlock()
	d := &s.vul.d
	v := &vulnView{Status: "checked", Apps: map[string]*appVulns{}}
	if known, probed := d.Ecos[eco]; probed && !known {
		v.Status, v.Apps = "unsupported", nil
		return v
	}
	red := map[string]bool{}
	anyOld, anyPending := false, false
	for _, ap := range st.Apps {
		if !ap.Installed {
			continue
		}
		av := &appVulns{Origin: ap.Origin}
		v.Apps[ap.Name] = av
		switch {
		case ap.Source == "" || ap.SourceVersion == "":
			av.Status, anyOld = "old_agent", true
			continue
		case ap.Origin == "":
			av.Status = "manual"
			continue
		case !fromDistro(a.Facts.OSID, ap.Origin):
			av.Status = "not_distro"
			continue
		}
		q := d.Queries[vulnKey(eco, ap.Source, ap.SourceVersion)]
		if q == nil {
			av.Status, anyPending = "pending", true
			continue
		}
		if q.At > v.CheckedAt {
			v.CheckedAt = q.At
		}
		seen := map[string]bool{}
		for _, id := range q.IDs {
			r := d.Records[id]
			if r == nil || seen[r.CVE] {
				continue
			}
			seen[r.CVE] = true
			c := cveView{ID: r.CVE, Link: osvLink + url.PathEscape(id), KEV: d.KEV[r.CVE], Word: r.Word}
			for _, af := range r.Aff {
				if af.Eco != eco || af.Pkg != ap.Source {
					continue
				}
				if af.Word != "" {
					c.Word = af.Word
				}
				for _, ev := range af.Events {
					if ev[0] == "fixed" && debVerCmp(ev[1], ap.SourceVersion) > 0 && (c.Fixed == "" || debVerCmp(ev[1], c.Fixed) < 0) {
						c.Fixed = ev[1]
					}
				}
			}
			c.Minor = minorWord(c.Word)
			if m := d.CVEs[r.CVE]; m != nil {
				c.EPSS, c.Pct = m.EPSS, m.Pct
				if m.Scored {
					c.Score, c.Scored, c.Src = m.Score, true, m.Src
					c.Level = scoreLevel(m.Score)
				}
			}
			if !c.Scored {
				// No CVSS score (yet): the distribution's word, if any.
				for _, w := range []string{c.Word, r.Word} {
					if l := wordLevel(w); l > c.Level {
						c.Level, c.Src = l, "distro"
					}
				}
			}
			if c.Fixed != "" && upd != nil {
				if nv := upd[ap.Package]; nv != "" && debVerCmp(nv, c.Fixed) >= 0 {
					c.ByUpdate = true
				}
			}
			switch {
			case c.Minor:
				av.Minor++
			case c.Fixed != "":
				av.Fixable++
			default:
				av.Open++
			}
			if c.KEV {
				av.KEV++
			}
			if c.red() {
				av.Red = true
				red[c.ID] = true
			}
			av.CVEs = append(av.CVEs, c)
		}
		av.Status = "ok"
		if av.Fixable+av.Open+av.KEV > 0 {
			av.Status = "found"
		}
		sort.Slice(av.CVEs, func(i, j int) bool {
			x, y := av.CVEs[i], av.CVEs[j]
			if x.KEV != y.KEV {
				return x.KEV
			}
			if x.Minor != y.Minor {
				return !x.Minor
			}
			if x.Level != y.Level {
				return x.Level > y.Level
			}
			if (x.Fixed != "") != (y.Fixed != "") {
				return x.Fixed != ""
			}
			if x.Score != y.Score {
				return x.Score > y.Score
			}
			return x.ID > y.ID
		})
		if full && upd != nil {
			for _, c := range av.CVEs {
				if c.ByUpdate {
					if v.UpdateFixes == nil {
						v.UpdateFixes = map[string][]string{}
					}
					v.UpdateFixes[ap.Package] = append(v.UpdateFixes[ap.Package], c.ID)
				}
			}
		}
		if !full {
			av.CVEs = nil
		}
	}
	v.Badge = len(red)
	switch {
	case len(v.Apps) > 0 && anyOld:
		v.Status = "old_agent"
	case anyPending && v.CheckedAt == 0:
		v.Status = "pending"
	}
	if !full {
		v.Apps = nil
	}
	return v
}

// vulnStatus is shown under Settings > General.
func (s *Server) vulnStatus(days int) map[string]any {
	s.st.mu.Lock()
	keySet := s.st.st.NVDKey != ""
	systems := 0
	for _, a := range s.st.st.Agents {
		if a.Status != StatusPending && osvEcosystem(a.Facts) != "" && appsOf(a) != nil {
			systems++
		}
	}
	s.st.mu.Unlock()
	pending := s.nvdPending()
	s.vul.mu.Lock()
	defer s.vul.mu.Unlock()
	d := &s.vul.d
	var unsupported []string
	for e, ok := range d.Ecos {
		if !ok {
			unsupported = append(unsupported, e)
		}
	}
	sort.Strings(unsupported)
	next := int64(0)
	if d.LastRun > 0 {
		next = d.LastRun + int64(days)*86400
	}
	if d.Error != "" && d.LastTry+int64(vulnRetry/time.Second) > next {
		next = d.LastTry + int64(vulnRetry/time.Second)
	}
	return map[string]any{
		"last_run": d.LastRun, "last_try": d.LastTry, "error": d.Error, "next_at": next,
		"cves": len(d.CVEs), "systems": systems, "kev_at": d.KEVAt, "kev_count": len(d.KEV), "epss_at": d.EPSSAt,
		"nvd_pending": pending, "nvd_key_set": keySet, "nvd_error": d.NVDErr, "unsupported": unsupported,
	}
}

func (s *Server) handleVulnCheck(w http.ResponseWriter, r *http.Request) {
	err := s.vulnTick(true)
	s.audit.write(actor(r), clientIP(r), "vulns.check", "", map[bool]string{true: "vulnerability data updated", false: fmt.Sprint("failed: ", err)}[err == nil])
	writeJSON(w, map[string]any{"vulns": s.vulnStatus(s.vulnDays())})
}

func (s *Server) handleNVDKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if !readJSON(w, r, &req, 1024) {
		return
	}
	req.Key = strings.TrimSpace(req.Key)
	if req.Key != "" && !nvdKeyRe.MatchString(req.Key) {
		http.Error(w, "that does not look like an NVD API key", http.StatusBadRequest)
		return
	}
	s.st.mu.Lock()
	s.st.st.NVDKey = req.Key
	s.st.markDirty()
	s.st.mu.Unlock()
	s.audit.write(actor(r), clientIP(r), "settings.nvd_key", "", map[bool]string{true: "NVD API key set", false: "NVD API key removed"}[req.Key != ""])
	s.kickNVD()
	writeJSON(w, map[string]any{"vulns": s.vulnStatus(s.vulnDays())})
}
