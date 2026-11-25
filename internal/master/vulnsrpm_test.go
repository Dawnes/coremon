package master

import (
	"testing"

	"coremon/internal/feature/apps"
	"coremon/internal/proto"
)

// Rocky Linux / AlmaLinux: advisories with several CVEs, RPM versions.
func TestVulnRPM(t *testing.T) {
	for _, c := range []struct {
		f    proto.Facts
		want string
	}{
		{proto.Facts{OSID: "rocky", OSVersion: "10.3"}, "Rocky Linux:10"},
		{proto.Facts{OSID: "almalinux", OSVersion: "9.6"}, "AlmaLinux:9"},
		{proto.Facts{OSID: "rhel", OSVersion: "10.1"}, ""},
		{proto.Facts{OSID: "rocky", OSVersion: "x"}, ""},
	} {
		if got := osvEcosystem(c.f); got != c.want {
			t.Errorf("%+v: %q, want %q", c.f, got, c.want)
		}
	}
	if !fromDistro("rocky", "Rocky Linux") || fromDistro("rocky", "EPEL") || !fromDistro("almalinux", "AlmaLinux") {
		t.Error("fromDistro")
	}
	if !osvWanted("RLSA-2026:102195") || !osvWanted("ALSA-2024:6783") || !osvWanted("DEBIAN-CVE-2025-1") || osvWanted("GHSA-xxxx-yyyy-zzzz") {
		t.Error("osvWanted")
	}

	var r1, r2 osvRecord
	r1.ID, r1.Summary, r1.Upstream = "RLSA-2026:1", "Moderate: openssh security update", []string{"CVE-2026-0002", "CVE-2026-0001"}
	r1.Affected = make([]struct {
		Package struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
		} `json:"package"`
		Ranges []struct {
			Type   string              `json:"type"`
			Events []map[string]string `json:"events"`
		} `json:"ranges"`
		EcoSpecific map[string]any `json:"ecosystem_specific"`
	}, 2)
	r1.Affected[0].Package.Ecosystem, r1.Affected[0].Package.Name = "Rocky Linux:10", "openssh"
	r1.Affected[0].Ranges = append(r1.Affected[0].Ranges, struct {
		Type   string              `json:"type"`
		Events []map[string]string `json:"events"`
	}{"ECOSYSTEM", []map[string]string{{"introduced": "0"}, {"fixed": "9.9p1-34.el10_3"}}})
	r1.Affected[1].Package.Ecosystem, r1.Affected[1].Package.Name = "Fedora:41", "openssh" // not ours
	rec1, ok := reduceRecord(r1)
	if !ok || rec1.CVE != "CVE-2026-0002" || len(rec1.More) != 1 || rec1.More[0] != "CVE-2026-0001" || rec1.Word != "moderate" || len(rec1.Aff) != 1 {
		t.Fatalf("advisory record: %+v", rec1)
	}
	r2 = r1
	r2.ID, r2.Summary, r2.Upstream = "RLSA-2026:2", "Important: openssh security update", []string{"CVE-2026-0002"}
	r2.Affected = r1.Affected[:1:1]
	r2.Affected[0].Ranges = r2.Affected[0].Ranges[:0:0]
	r2.Affected[0].Ranges = append(r2.Affected[0].Ranges, struct {
		Type   string              `json:"type"`
		Events []map[string]string `json:"events"`
	}{"ECOSYSTEM", []map[string]string{{"introduced": "0"}, {"fixed": "9.9p1-12.el10_1"}}})
	rec2, _ := reduceRecord(r2)

	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	eco := "Rocky Linux:10"
	ap := apps.App{Name: "SSH server", Installed: true, Package: "openssh-server", Source: "openssh", SourceVersion: "9.9p1-7.el10", Origin: "Rocky Linux"}
	s.vul.mu.Lock()
	s.vul.d.Records["RLSA-2026:1"], s.vul.d.Records["RLSA-2026:2"] = rec1, rec2
	s.vul.d.Queries[vulnKey(eco, "openssh", "9.9p1-7.el10")] = &vulnQuery{At: 1, IDs: []string{"RLSA-2026:1", "RLSA-2026:2"}}
	av := &appVulns{}
	var at int64
	list, ok := s.distroCVEs(ap, eco, "rocky", map[string]string{"openssh-server": "9.9p1-34.el10_3"}, av, &at)
	s.vul.mu.Unlock()
	if !ok || len(list) != 2 {
		t.Fatalf("CVEs: %v %+v", ok, list)
	}
	byID := map[string]cveView{}
	for _, c := range list {
		byID[c.ID] = c
	}
	// In both advisories: the earliest fix, the highest rating.
	if c := byID["CVE-2026-0002"]; c.Fixed != "9.9p1-12.el10_1" || c.Word != "important" || c.Level != 3 || !c.ByUpdate || c.Minor {
		t.Errorf("CVE-2026-0002: %+v", c)
	}
	if c := byID["CVE-2026-0001"]; c.Fixed != "9.9p1-34.el10_3" || c.Level != 2 || !c.ByUpdate {
		t.Errorf("CVE-2026-0001: %+v", c)
	}
	// High with a fix: red; Low never minor.
	if !byID["CVE-2026-0002"].red() {
		t.Error("important with a fix should count")
	}
	if minorWord("low") {
		t.Error("low is not minor")
	}
	// Already fixed in the installed version: no fix to show.
	ap.SourceVersion = "9.9p1-40.el10_3"
	s.vul.mu.Lock()
	s.vul.d.Queries[vulnKey(eco, "openssh", ap.SourceVersion)] = &vulnQuery{At: 1, IDs: []string{"RLSA-2026:1"}}
	list, _ = s.distroCVEs(ap, eco, "rocky", nil, &appVulns{}, &at)
	s.vul.mu.Unlock()
	for _, c := range list {
		if c.Fixed != "" {
			t.Errorf("fixed version older than installed: %+v", c)
		}
	}
	// Pruning keeps every CVE of an advisory.
	s.vul.mu.Lock()
	known := s.pruneLocked(map[string]bool{vulnKey(eco, "openssh", "9.9p1-7.el10"): true}, map[string]bool{eco: true})
	s.vul.mu.Unlock()
	if len(known) != 2 {
		t.Errorf("known CVEs after pruning: %v", known)
	}
}
