package master

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"coremon/internal/proto"
)

const testDebianCSV = `version,codename,series,created,release,eol,eol-lts,eol-elts
1.1,Buzz,buzz,1993-08-16,1996-06-17,1996-12-12
3.1,Sarge,sarge,2002-07-19,2005-06-06,2008-03-31
10,Buster,buster,2017-06-17,2019-07-06,2022-09-10,2024-06-30,2029-06-30
11,Bullseye,bullseye,2019-07-06,2021-08-14,2024-08-14,2026-08-31,2031-06-30
12,Bookworm,bookworm,2021-08-14,2023-06-10,2026-07-11,2028-06-30,2033-06-30
13,Trixie,trixie,2023-06-10,2025-08-09,2028-08-09,2030-06-30,2035-06-30
14,Forky,forky,2025-08-09
,Sid,sid,1993-08-16
`

const testUbuntuCSV = `version,codename,series,created,release,eol,eol-server,eol-esm,eol-legacy
4.10,Warty Warthog,warty,2004-03-05,2004-10-20,2006-04-30
6.06 LTS,Dapper Drake,dapper,2005-10-13,2006-06-01,2009-07-14,2011-06-01
20.04 LTS,Focal Fossa,focal,2019-10-17,2020-04-23,2025-05-29,2025-05-29,2030-04-23,2035-04-26
22.04 LTS,Jammy Jellyfish,jammy,2021-10-14,2022-04-21,2027-06-01,2027-06-01,2032-04-21,2037-04-30
24.04 LTS,Noble Numbat,noble,2023-10-12,2024-04-25,2029-05-31,2029-05-31,2034-04-25,2039-04-28
25.10,Questing Quokka,questing,2025-04-17,2025-10-09,2026-07-09
`

func TestOSSupportStatus(t *testing.T) {
	deb, err := parseDistroInfo([]byte(testDebianCSV), "debian")
	if err != nil {
		t.Fatal(err)
	}
	ubu, err := parseDistroInfo([]byte(testUbuntuCSV), "ubuntu")
	if err != nil {
		t.Fatal(err)
	}
	d := &osInfoData{Debian: deb, Ubuntu: ubu}
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		id, ver, status, phase, until string
	}{
		{"debian", "11", "expired", "ended", ""},
		{"debian", "12", "supported", "lts", "2028-06-30"},
		{"debian", "13", "supported", "security", "2028-08-09"},
		{"debian", "14", "supported", "open", ""},
		{"ubuntu", "20.04", "expired", "ended", ""},
		{"ubuntu", "22.04", "supported", "standard", "2027-06-01"},
		{"ubuntu", "25.10", "expired", "ended", ""},
	}
	for _, c := range cases {
		o := supportStatus(d, c.id, c.ver, now)
		if o == nil || o.Status != c.status || o.Phase != c.phase || o.Until != c.until {
			t.Errorf("%s %s: %+v", c.id, c.ver, o)
		}
	}
	if o := supportStatus(d, "debian", "13", now); o.LTSUntil != "2030-06-30" {
		t.Errorf("debian 13 LTS end: %+v", o)
	}
	if o := supportStatus(d, "ubuntu", "20.04", now); o.ESMUntil != "2030-04-23" {
		t.Errorf("ubuntu 20.04 ESM: %+v", o)
	}
	// Within six months of the end: soon.
	if o := supportStatus(d, "ubuntu", "22.04", time.Date(2027, 1, 10, 0, 0, 0, 0, time.UTC)); o.Status != "soon" {
		t.Errorf("ubuntu 22.04 in Jan 2027: %+v", o)
	}
	// The last day still counts.
	if o := supportStatus(d, "debian", "11", time.Date(2026, 8, 31, 23, 0, 0, 0, time.UTC)); o.Status == "expired" {
		t.Errorf("last day of LTS: %+v", o)
	}
	for _, unknown := range [][2]string{{"debian", ""}, {"debian", "99"}, {"fedora", "40"}} {
		if o := supportStatus(d, unknown[0], unknown[1], now); o != nil {
			t.Errorf("%v: %+v", unknown, o)
		}
	}
	// Strict parsing.
	for _, bad := range []string{
		"version,release,eol\n12,2023-06-10,2026-07-11\n",                                                  // eol-lts column missing
		strings.Replace(testDebianCSV, "2026-08-31", "31-08-2026", 1),                                      // bad date
		strings.Replace(testDebianCSV, "12,Bookworm", "<b>,Bookworm", 1),                                   // bad version
		"version,codename,series,created,release,eol,eol-lts\n12,B,b,x,2023-06-10,2026-07-11,2028-06-30\n", // too few
	} {
		if _, err := parseDistroInfo([]byte(bad), "debian"); err == nil {
			t.Errorf("accepted: %q", bad)
		}
	}
}

func TestOSInfoLookup(t *testing.T) {
	fail := false
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case fail:
			http.Error(w, "down", 503)
		case r.URL.Path == "/debian.csv":
			w.Write([]byte(testDebianCSV))
		case r.URL.Path == "/ubuntu.csv":
			w.Write([]byte(testUbuntuCSV))
		default:
			http.NotFound(w, r)
		}
	}))
	defer gh.Close()
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.cfg.OSInfoURL = gh.URL
	if s.supportFor("debian", "12", time.Now()) != nil {
		t.Fatal("support known before any lookup")
	}
	if err := s.refreshOSInfo(); err != nil {
		t.Fatal(err)
	}
	if o := s.supportFor("debian", "12", time.Now()); o == nil {
		t.Fatal("no status after lookup")
	}
	// A failed lookup keeps the data and is retried after 6 hours.
	fail = true
	if err := s.refreshOSInfo(); err == nil {
		t.Fatal("lookup should fail")
	}
	if s.supportFor("debian", "12", time.Now()) == nil {
		t.Fatal("data lost after a failed lookup")
	}
	s.osi.mu.Lock()
	f := s.osi.f
	s.osi.mu.Unlock()
	if f.Error == "" || osInfoDue(f, 14, time.Now()) {
		t.Fatal("a failed lookup must wait before retrying")
	}
	failedFirst := osInfoFile{Error: "down", LastTry: time.Now().Unix()}
	if osInfoDue(failedFirst, 14, time.Now().Add(5*time.Hour)) || !osInfoDue(failedFirst, 14, time.Now().Add(7*time.Hour)) {
		t.Fatal("retry after 6 hours")
	}
	// Stored on disk: a restarted master has it.
	s2 := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s2.cfg.DataDir = s.cfg.DataDir
	s2.loadOSInfo()
	if s2.supportFor("ubuntu", "22.04", time.Now()) == nil {
		t.Fatal("not stored")
	}
	// Interval: due after the configured days.
	ok := osInfoFile{Data: &osInfoData{FetchedAt: time.Now().Add(-4 * 24 * time.Hour).Unix()}}
	if !osInfoDue(ok, 3, time.Now()) || osInfoDue(ok, 14, time.Now()) {
		t.Fatal("interval")
	}
}

func TestHealthLimits(t *testing.T) {
	names := make([]string, 50)
	for i := range names {
		names[i] = strings.Repeat("u", 500)
	}
	f := sanitizeFacts(proto.Facts{Health: &proto.Health{FailedCount: 50, Failed: names, AutoUpdates: "<b>",
		LastUpgrade: &proto.Upgrade{Packages: -5, By: "evil"}}})
	h := f.Health
	if len(h.Failed) != maxFailedUnits || len(h.Failed[0]) != 128 || h.AutoUpdates != "" || h.LastUpgrade.Packages != 0 || h.LastUpgrade.By != "" {
		t.Fatalf("health not limited: %+v %+v", h, h.LastUpgrade)
	}
}
