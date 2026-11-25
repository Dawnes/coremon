package master

import (
	"encoding/json"
	"testing"
	"time"

	"coremon/internal/feature/apps"
	"coremon/internal/feature/updates"
	"coremon/internal/proto"
)

func TestSUSEAlpineSupport(t *testing.T) {
	sles, _ := parseEOL([]byte(`[{"cycle":"16.0","releaseDate":"2025-11-04","eol":"2027-11-30","extendedSupport":"2030-11-30"},{"cycle":"15.7","releaseDate":"2025-06-17","eol":"2031-07-31"}]`))
	leap, _ := parseEOL([]byte(`[{"cycle":"16.0","releaseDate":"2025-10-01","eol":"2027-10-31"},{"cycle":"15.6","releaseDate":"2024-06-12","eol":"2026-04-30"}]`))
	alpine, err := parseEOL([]byte(`[{"cycle":"3.22","releaseDate":"2025-05-30","eol":"2027-05-01"},{"cycle":"3.18","releaseDate":"2023-05-09","eol":"2025-05-09"}]`))
	if err != nil {
		t.Fatal(err)
	}
	d := &osInfoData{EL: map[string][]osRelease{"sles": sles, "opensuse-leap": leap, "alpine": alpine}}
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		id, ver, status, phase, until string
	}{
		{"sles", "16.0", "supported", "until", "2027-11-30"}, // not the paid LTSS
		{"sles", "15.7", "supported", "until", "2031-07-31"},
		{"opensuse-leap", "15.6", "expired", "ended", ""},
		{"alpine", "3.22.2", "soon", "until", "2027-05-01"}, // within six months
		{"alpine", "3.18.9", "expired", "ended", ""},
		{"opensuse-tumbleweed", "20261120", "supported", "open", ""},
	} {
		o := supportStatus(d, c.id, c.ver, now)
		if o == nil || o.Status != c.status || o.Phase != c.phase || o.Until != c.until {
			t.Errorf("%s %s: %+v", c.id, c.ver, o)
		}
	}
	for _, c := range []struct {
		f    proto.Facts
		want string
	}{
		{proto.Facts{OSID: "alpine", OSVersion: "3.22.2"}, "Alpine:v3.22"},
		{proto.Facts{OSID: "opensuse-leap", OSVersion: "15.6"}, "openSUSE:Leap 15.6"},
		{proto.Facts{OSID: "opensuse-tumbleweed", OSVersion: "20261120"}, "openSUSE:Tumbleweed"},
		{proto.Facts{OSID: "sles", OSVersion: "16.0"}, "SUSE:Linux Enterprise Server 16.0"},
		{proto.Facts{OSID: "sles", OSVersion: "15.6"}, ""}, // per module: not yet
	} {
		if got := osvEcosystem(c.f); got != c.want {
			t.Errorf("%+v: %q, want %q", c.f, got, c.want)
		}
	}
	if !fromDistro("opensuse-leap", "SUSE") || !fromDistro("opensuse-leap", "openSUSE") || fromDistro("sles", "openSUSE") || !fromDistro("alpine", "Alpine") {
		t.Error("fromDistro")
	}
	if !osvWanted("SUSE-SU-2025:0585-1") || !osvWanted("openSUSE-SU-2024:13842-1") || !osvWanted("ALPINE-CVE-2024-6387") {
		t.Error("osvWanted")
	}
	if probePkg("openSUSE:Leap 15.6") != "openssh" || probePkg("Alpine:v3.22") != "openssl" {
		t.Error("probePkg")
	}
	if verCmp("Alpine:v3.22")("9.7_p1-r4", "9.7_p1-r10") >= 0 || verCmp("openSUSE:Leap 15.6")("9.6p1-150600.6.3.1", "9.6p1-150600.6.15.2") >= 0 {
		t.Error("verCmp")
	}
}

// Alpine: apk marks no security updates; the master counts the waiting
// updates that fix a CVE it knows for the device.
func TestSecurityFromCVE(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	eco := "Alpine:v3.22"
	snap := updates.Snapshot{Backend: "apk", SecurityFromCVE: true, Total: 2, Packages: []updates.Package{
		{Name: "openssh-server", Current: "9.9_p1-r2", Available: "9.9_p2-r0"},
		{Name: "busybox", Current: "1.37.0-r18", Available: "1.37.0-r19"},
	}}
	raw, _ := json.Marshal(snap)
	a := &Agent{ID: "a_alp", Name: "alp", Status: StatusActive, CoreVersion: "0.3.6",
		Facts: proto.Facts{OSID: "alpine", OSVersion: "3.22.2"},
		State: map[string]json.RawMessage{"updates": raw, "apps": appsState(t,
			apps.App{Name: "SSH server", Installed: true, Package: "openssh-server", Source: "openssh", SourceVersion: "9.9_p1-r2", Origin: "Alpine"})},
		StateAt: map[string]int64{}}
	s.st.mu.Lock()
	s.st.st.Agents[a.ID] = a
	s.st.mu.Unlock()
	s.vul.mu.Lock()
	s.vul.d.Ecos[eco] = true
	s.vul.d.Records["ALPINE-CVE-2025-1"] = &vulnRec{CVE: "CVE-2025-1", Aff: []vulnAff{{Eco: eco, Pkg: "openssh", Events: [][2]string{{"introduced", "0"}, {"fixed", "9.9_p2-r0"}}}}}
	s.vul.d.Queries[vulnKey(eco, "openssh", "9.9_p1-r2")] = &vulnQuery{At: 1, IDs: []string{"ALPINE-CVE-2025-1"}}
	s.vul.mu.Unlock()

	s.st.mu.Lock()
	v := s.view(a, false)
	s.st.mu.Unlock()
	if v.Updates == nil || v.Updates.Security != 1 || !v.Updates.SecurityFromCVE || v.Vulns.UpdateFixes != nil {
		t.Fatalf("overview: %+v %+v", v.Updates, v.Vulns)
	}
	s.st.mu.Lock()
	full := s.view(a, true)
	s.st.mu.Unlock()
	if f := full.Vulns.UpdateFixes["openssh-server"]; len(f) != 1 || full.Updates.Security != 1 {
		t.Errorf("device page: %+v %+v", full.Updates, full.Vulns.UpdateFixes)
	}
	// Accepted: no longer a security update.
	s.st.mu.Lock()
	s.st.st.AcceptedCVEs = map[string]*AcceptedCVE{"CVE-2025-1": {By: "admin"}}
	v = s.view(a, false)
	s.st.st.AcceptedCVEs = nil
	s.st.mu.Unlock()
	if v.Updates.Security != 0 {
		t.Errorf("accepted CVE still counts: %+v", v.Updates)
	}
}
