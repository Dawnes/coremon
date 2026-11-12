package master

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"coremon/internal/feature/apps"
	"coremon/internal/feature/updates"
	"coremon/internal/proto"
)

// fakeVulnSources plays OSV, NVD, CISA (KEV) and FIRST (EPSS).
func fakeVulnSources(t *testing.T) (*httptest.Server, *[]string) {
	var mu sync.Mutex
	var sent []string
	rec := func(id, cve string, sev, aff string) string {
		return fmt.Sprintf(`{"id":%q,"modified":"2026-11-01T00:00:00Z","upstream":[%q],"severity":[%s],"affected":[%s]}`, id, cve, sev, aff)
	}
	records := map[string]string{
		"DEBIAN-CVE-2025-1001": rec("DEBIAN-CVE-2025-1001", "CVE-2025-1001", `{"type":"CVSS_V3","score":"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"}`,
			`{"package":{"ecosystem":"Debian:12","name":"openssl"},"ranges":[{"type":"ECOSYSTEM","events":[{"introduced":"0"},{"fixed":"3.0.17-1~deb12u1"}]}],"ecosystem_specific":{"urgency":"not yet assigned"}},
			 {"package":{"ecosystem":"Debian:13","name":"openssl"},"ranges":[{"type":"ECOSYSTEM","events":[{"introduced":"0"},{"fixed":"3.5.1-1"}]}]}`),
		"DEBIAN-CVE-2025-1002": rec("DEBIAN-CVE-2025-1002", "CVE-2025-1002", ``,
			`{"package":{"ecosystem":"Debian:12","name":"openssl"},"ranges":[{"type":"ECOSYSTEM","events":[{"introduced":"0"}]}],"ecosystem_specific":{"urgency":"unimportant"}}`),
		"DEBIAN-CVE-2025-1003": rec("DEBIAN-CVE-2025-1003", "CVE-2025-1003", ``,
			`{"package":{"ecosystem":"Debian:12","name":"openssl"},"ranges":[{"type":"ECOSYSTEM","events":[{"introduced":"0"}]}],"ecosystem_specific":{"urgency":"not yet assigned"}}`),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		sent = append(sent, string(b))
		mu.Unlock()
		var req struct {
			Queries []struct {
				Package   struct{ Name, Ecosystem string } `json:"package"`
				Version   string                           `json:"version"`
				PageToken string                           `json:"page_token"`
			} `json:"queries"`
		}
		json.Unmarshal(b, &req)
		var out []string
		for _, q := range req.Queries {
			switch {
			case q.Package.Ecosystem != "Debian:12":
				out = append(out, `{}`) // OSV has no data for it
			case q.Version == "0.1":
				out = append(out, `{"vulns":[{"id":"DEBIAN-CVE-2025-1001","modified":"x"}]}`)
			case q.Package.Name == "openssl" && q.Version != "3.0.15-1~deb12u1": // after the upgrade: only the unfixed ones
				out = append(out, `{"vulns":[{"id":"DEBIAN-CVE-2025-1002","modified":"2026-11-01T00:00:00Z"},{"id":"DEBIAN-CVE-2025-1003","modified":"2026-11-01T00:00:00Z"}]}`)
			case q.Package.Name == "openssl" && q.PageToken == "":
				out = append(out, `{"vulns":[{"id":"DEBIAN-CVE-2025-1001","modified":"2026-11-01T00:00:00Z"},{"id":"DSA-5999-1","modified":"x"},{"id":"DEBIAN-CVE-2025-1002","modified":"2026-11-01T00:00:00Z"}],"next_page_token":"p2"}`)
			case q.Package.Name == "openssl" && q.PageToken == "p2":
				out = append(out, `{"vulns":[{"id":"DEBIAN-CVE-2025-1003","modified":"2026-11-01T00:00:00Z"}]}`)
			default:
				out = append(out, `{}`)
			}
		}
		fmt.Fprintf(w, `{"results":[%s]}`, strings.Join(out, ","))
	})
	mux.HandleFunc("GET /v1/vulns/{id}", func(w http.ResponseWriter, r *http.Request) {
		if b, ok := records[r.PathValue("id")]; ok {
			io.WriteString(w, b)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("GET /kev", func(w http.ResponseWriter, r *http.Request) {
		var v []string
		for i := 0; i < 150; i++ {
			v = append(v, fmt.Sprintf(`{"cveID":"CVE-2020-%d"}`, 10000+i))
		}
		v = append(v, `{"cveID":"CVE-2025-1003"}`)
		fmt.Fprintf(w, `{"vulnerabilities":[%s]}`, strings.Join(v, ","))
	})
	mux.HandleFunc("GET /epss", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"data":[{"cve":"CVE-2025-1001","epss":"0.91234","percentile":"0.998"}]}`)
	})
	mux.HandleFunc("GET /nvd", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("cveId") == "CVE-2025-1003" {
			io.WriteString(w, `{"vulnerabilities":[{"cve":{"id":"CVE-2025-1003","metrics":{"cvssMetricV31":[{"type":"Secondary","cvssData":{"baseScore":5.0}},{"type":"Primary","cvssData":{"baseScore":7.5}}]}}}]}`)
			return
		}
		io.WriteString(w, `{"vulnerabilities":[]}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &sent
}

func appsState(t *testing.T, list ...apps.App) json.RawMessage {
	b, _ := json.Marshal(apps.State{ScannedAt: time.Now().Unix(), Apps: list})
	raw, err := checkApps(b)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestVulnCheck(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	src, sent := fakeVulnSources(t)
	s.cfg.OSVURL, s.cfg.KEVURL, s.cfg.EPSSURL, s.cfg.NVDURL = src.URL, src.URL+"/kev", src.URL+"/epss", src.URL+"/nvd"

	upd, _ := json.Marshal(updates.Snapshot{Packages: []updates.Package{{Name: "libssl3", Current: "3.0.15-1~deb12u1", Available: "3.0.17-1~deb12u2"}}})
	s.st.mu.Lock()
	s.st.st.Agents["a_deb"] = &Agent{ID: "a_deb", Name: "secret-host-name", Status: StatusActive, CoreVersion: "0.3.6",
		Facts: proto.Facts{OSID: "debian", OSVersion: "12", Hostname: "secret-host-name"},
		State: map[string]json.RawMessage{"updates": upd, "apps": appsState(t,
			apps.App{Name: "OpenSSL", Installed: true, Package: "libssl3", Source: "openssl", SourceVersion: "3.0.15-1~deb12u1", Origin: "Debian"},
			apps.App{Name: "Docker", Installed: true, Package: "docker-ce", Source: "docker-ce", SourceVersion: "5:27.3.1-1~debian.12~bookworm", Origin: "Docker"},
			apps.App{Name: "sudo", Installed: true, Package: "sudo", Source: "sudo", SourceVersion: "1.9.13p3-1+deb12u1", Origin: ""},
			apps.App{Name: "Apache"},
		)}, StateAt: map[string]int64{}}
	s.st.st.Agents["a_new"] = &Agent{ID: "a_new", Name: "new", Status: StatusActive, CoreVersion: "0.3.6",
		Facts: proto.Facts{OSID: "ubuntu", OSVersion: "26.04"},
		State: map[string]json.RawMessage{"apps": appsState(t,
			apps.App{Name: "OpenSSL", Installed: true, Package: "libssl3t64", Source: "openssl", SourceVersion: "3.5.3-1ubuntu1", Origin: "Ubuntu"})},
		StateAt: map[string]int64{}}
	s.st.st.Agents["a_old"] = &Agent{ID: "a_old", Name: "old", Status: StatusActive, CoreVersion: "0.3.5",
		Facts:   proto.Facts{OSID: "debian", OSVersion: "12"},
		State:   map[string]json.RawMessage{"apps": appsState(t, apps.App{Name: "OpenSSL", Installed: true, Package: "libssl3"})},
		StateAt: map[string]int64{}}
	s.st.mu.Unlock()

	if err := s.vulnTick(true); err != nil {
		t.Fatal(err)
	}
	// Only release, package names and versions leave the master.
	for _, b := range *sent {
		if strings.Contains(b, "secret-host-name") || strings.Contains(b, "a_deb") {
			t.Fatalf("sent to OSV: %s", b)
		}
	}
	if !strings.Contains(strings.Join(*sent, ""), `"page_token":"p2"`) {
		t.Error("second result page not asked for")
	}
	view := func(id string, full bool) *vulnView {
		s.st.mu.Lock()
		defer s.st.mu.Unlock()
		return s.vulnsFor(s.st.st.Agents[id], full)
	}
	v := view("a_deb", true)
	o := v.Apps["OpenSSL"]
	if v.Status != "checked" || o == nil || o.Status != "found" || o.Fixable != 1 || o.Open != 1 || o.Minor != 1 || o.KEV != 1 || !o.Red || len(o.CVEs) != 3 {
		t.Fatalf("OpenSSL: %+v / %+v", v, o)
	}
	byID := map[string]cveView{}
	for _, c := range o.CVEs {
		byID[c.ID] = c
	}
	c1, c2, c3 := byID["CVE-2025-1001"], byID["CVE-2025-1002"], byID["CVE-2025-1003"]
	if c1.Fixed != "3.0.17-1~deb12u1" || !c1.ByUpdate || c1.Level != 4 || c1.Score != 9.8 || c1.Src != "osv" || c1.EPSS != 0.91234 || !strings.HasSuffix(c1.Link, "/DEBIAN-CVE-2025-1001") {
		t.Errorf("CVE-2025-1001: %+v", c1)
	}
	if !c2.Minor || c2.Fixed != "" || c2.Level != 1 {
		t.Errorf("CVE-2025-1002 (unimportant): %+v", c2)
	}
	if !c3.KEV || c3.Fixed != "" || c3.Level != 0 {
		t.Errorf("CVE-2025-1003 before NVD: %+v", c3)
	}
	if o.CVEs[0].ID != "CVE-2025-1003" { // actively exploited first
		t.Errorf("order: %v", o.CVEs)
	}
	if v.Badge != 2 { // fixable critical + exploited
		t.Errorf("badge %d, want 2", v.Badge)
	}
	if f := v.UpdateFixes["libssl3"]; len(f) != 1 || f[0] != "CVE-2025-1001" {
		t.Errorf("update fixes: %v", v.UpdateFixes)
	}
	if v.Apps["Docker"].Status != "not_distro" || v.Apps["Docker"].Origin != "Docker" || v.Apps["sudo"].Status != "manual" {
		t.Errorf("other sources: %+v %+v", v.Apps["Docker"], v.Apps["sudo"])
	}
	if _, ok := v.Apps["Apache"]; ok {
		t.Error("not installed app listed")
	}
	if ov := view("a_deb", false); ov.Badge != 2 || ov.Apps != nil {
		t.Errorf("overview: %+v", ov)
	}
	if nv := view("a_new", true); nv.Status != "unsupported" {
		t.Errorf("release without OSV data: %+v", nv)
	}
	if ol := view("a_old", true); ol.Status != "old_agent" {
		t.Errorf("agent before 0.3.6: %+v", ol)
	}
	if u := fmt.Sprint(s.vulnStatus(1)["unsupported"]); u != "[{Ubuntu:26.04:LTS ubuntu 26.04 false}]" {
		t.Errorf("unsupported releases: %v", u)
	}
	// The system is upgraded to a release with data: the message goes at once,
	// and the old release is forgotten at the next full check.
	s.st.mu.Lock()
	s.st.st.Agents["a_new"].Facts.OSID, s.st.st.Agents["a_new"].Facts.OSVersion = "debian", "12"
	s.st.mu.Unlock()
	if u := fmt.Sprint(s.vulnStatus(1)["unsupported"]); u != "[]" {
		t.Errorf("after the upgrade: %v", u)
	}
	if err := s.vulnTick(true); err != nil {
		t.Fatal(err)
	}
	s.vul.mu.Lock()
	_, stale := s.vul.d.Ecos["Ubuntu:26.04:LTS"]
	s.vul.mu.Unlock()
	if stale {
		t.Error("old release still remembered after a full check")
	}
	s.st.mu.Lock()
	s.st.st.Agents["a_new"].Facts.OSID, s.st.st.Agents["a_new"].Facts.OSVersion = "ubuntu", "26.04"
	s.st.mu.Unlock()
	if err := s.vulnTick(false); err != nil { // the release is looked at again
		t.Fatal(err)
	}

	// The list on the Releases page: releases once each, then the lookups,
	// what runs now first.
	queue := func() (rel []string, items []queueItem) {
		w := httptest.NewRecorder()
		s.handleVulnQueue(w, httptest.NewRequest("GET", "/api/ui/vulns/queue", nil))
		var out struct {
			Releases []string    `json:"releases"`
			Items    []queueItem `json:"items"`
		}
		json.Unmarshal(w.Body.Bytes(), &out)
		return out.Releases, out.Items
	}
	s.vul.mu.Lock()
	s.vul.nvdNow = "CVE-2025-1002"
	s.vul.mu.Unlock()
	rel, items := queue()
	if strings.Join(rel, ",") != "debian 12,ubuntu 26.04" {
		t.Errorf("releases: %v", rel)
	}
	if len(items) != 2 || items[0].What != "CVE-2025-1002" || items[0].Status != "now" || items[0].Where != "NVD" ||
		items[1].What != "CVE-2025-1003" || items[1].Status != "ratelimit" || items[1].At == 0 {
		t.Errorf("queue: %+v", items)
	}
	s.vul.mu.Lock()
	s.vul.nvdNow = ""
	s.vul.mu.Unlock()

	// NVD fills in the missing score; the badge does not change (no fix).
	if id, _ := s.nextNVD(time.Now()); id != "CVE-2025-1003" {
		t.Fatalf("next NVD lookup: %q", id)
	}
	if err := s.lookupNVD("CVE-2025-1003", ""); err != nil {
		t.Fatal(err)
	}
	c3 = view("a_deb", true).Apps["OpenSSL"].CVEs[0]
	if c3.Score != 7.5 || c3.Level != 3 || c3.Src != "nvd" {
		t.Errorf("after NVD: %+v", c3)
	}
	if id, _ := s.nextNVD(time.Now()); id != "CVE-2025-1002" {
		t.Errorf("next NVD lookup: %q", id)
	}
	s.lookupNVD("CVE-2025-1002", "") // NVD has no score: asked again in a week
	if id, _ := s.nextNVD(time.Now()); id != "" {
		t.Errorf("nothing left for NVD, got %q", id)
	}
	if _, items := queue(); len(items) != 1 || items[0].Status != "noscore" || items[0].At < time.Now().Add(6*24*time.Hour).Unix() {
		t.Errorf("queue after NVD had no score: %+v", items)
	}
	// A new version waits for the next look.
	s.st.mu.Lock()
	s.st.st.Agents["a_deb"].State["apps"] = appsState(t, apps.App{Name: "OpenSSL", Installed: true, Package: "libssl3", Source: "openssl", SourceVersion: "3.0.16-1~deb12u1", Origin: "Debian"})
	s.st.mu.Unlock()
	if _, items := queue(); len(items) != 2 || items[0].What != "Debian:12 openssl 3.0.16-1~deb12u1" || items[0].Where != "OSV" || items[0].Status != "next" {
		t.Errorf("queue with a new version: %+v", items)
	}

	// After the upgrade the new version is looked up by itself, and a full
	// check drops what nobody uses any more.
	s.st.mu.Lock()
	s.st.st.Agents["a_deb"].State["apps"] = appsState(t, apps.App{Name: "OpenSSL", Installed: true, Package: "libssl3", Source: "openssl", SourceVersion: "3.0.17-1~deb12u2", Origin: "Debian"})
	s.st.mu.Unlock()
	if err := s.vulnTick(false); err != nil {
		t.Fatal(err)
	}
	// CVE-2025-1001 is fixed now; the exploited one without a fix keeps the badge.
	if v := view("a_deb", true); v.Apps["OpenSSL"].Fixable != 0 || v.Apps["OpenSSL"].Open != 1 || v.Badge != 1 {
		t.Errorf("after upgrade: %+v", v.Apps["OpenSSL"])
	}
	if err := s.vulnTick(true); err != nil {
		t.Fatal(err)
	}
	s.vul.mu.Lock()
	nq, nr := len(s.vul.d.Queries), len(s.vul.d.Records)
	_, kept := s.vul.d.CVEs["CVE-2025-1003"]
	s.vul.mu.Unlock()
	if nq != 1 || nr != 2 || !kept {
		t.Errorf("after pruning: %d queries, %d records", nq, nr)
	}
	// Persisted and read back.
	s.loadVulns()
	if st := s.vulnStatus(1); st["kev_count"].(int) != 151 || st["error"] != "" {
		t.Errorf("status: %v", st)
	}
	if _, err := os.Stat(s.vulnPath()); err != nil {
		t.Error(err)
	}
}

func TestVulnInput(t *testing.T) {
	raw := appsState(t, apps.App{Name: "OpenSSL", Installed: true, Package: "libssl3", Source: "openssl;reboot", SourceVersion: "3.0 && x", Origin: "Debian\x1b[2J"})
	var st apps.State
	json.Unmarshal(raw, &st)
	if a := st.Apps[0]; a.Source != "" || a.SourceVersion != "" || a.Origin != "Debian[2J" {
		t.Errorf("not cleaned: %+v", a)
	}
	for _, c := range []struct{ id, ver, want string }{
		{"debian", "12", "Debian:12"},
		{"debian", "13.1", "Debian:13"},
		{"ubuntu", "24.04", "Ubuntu:24.04:LTS"},
		{"ubuntu", "25.10", "Ubuntu:25.10"},
		{"ubuntu", "23.04", "Ubuntu:23.04"},
		{"fedora", "40", ""},
		{"debian", "x", ""},
	} {
		if got := osvEcosystem(proto.Facts{OSID: c.id, OSVersion: c.ver}); got != c.want {
			t.Errorf("%s %s: %q, want %q", c.id, c.ver, got, c.want)
		}
	}
}

// TestVulnOnline asks the real OSV once (COREMON_ONLINE=1).
func TestVulnOnline(t *testing.T) {
	if os.Getenv("COREMON_ONLINE") != "1" {
		t.Skip("set COREMON_ONLINE=1")
	}
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.st.mu.Lock()
	s.st.st.Agents["a_x"] = &Agent{ID: "a_x", Name: "x", Status: StatusActive, CoreVersion: "0.3.6",
		Facts: proto.Facts{OSID: "debian", OSVersion: "12"},
		State: map[string]json.RawMessage{"apps": appsState(t,
			apps.App{Name: "OpenSSL", Installed: true, Package: "libssl3", Source: "openssl", SourceVersion: "3.0.15-1~deb12u1", Origin: "Debian"},
			apps.App{Name: "SSH server", Installed: true, Package: "openssh-server", Source: "openssh", SourceVersion: "1:9.2p1-2+deb12u3", Origin: "Debian"})},
		StateAt: map[string]int64{}}
	s.st.mu.Unlock()
	t0 := time.Now()
	if err := s.vulnTick(true); err != nil {
		t.Fatal(err)
	}
	s.st.mu.Lock()
	v := s.vulnsFor(s.st.st.Agents["a_x"], true)
	s.st.mu.Unlock()
	t.Logf("check took %s; badge %d; %d NVD lookups waiting", time.Since(t0).Round(time.Millisecond), v.Badge, s.nvdPending())
	for name, a := range v.Apps {
		t.Logf("%s: %s, %d fixable, %d without fix, %d minor, %d exploited", name, a.Status, a.Fixable, a.Open, a.Minor, a.KEV)
		for i, c := range a.CVEs {
			if i < 4 {
				t.Logf("   %s level %d score %.1f (%s) fixed %q exploited %v epss %.3f", c.ID, c.Level, c.Score, c.Src, c.Fixed, c.KEV, c.EPSS)
			}
		}
	}
	if v.Apps["OpenSSL"].Fixable == 0 {
		t.Error("an old OpenSSL should have fixable CVEs")
	}
}
