package master

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"coremon/internal/feature/apps"
	"coremon/internal/proto"
)

// Proxmox CVEs come from NVD by product: only those whose version range
// covers the installed version count, with the range end as the fix.
func TestProxmoxNVD(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	asked := ""
	nvd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.RawQuery
		w.Write([]byte(`{"totalResults":4,"vulnerabilities":[
 {"cve":{"id":"CVE-2024-0001","vulnStatus":"Analyzed","metrics":{"cvssMetricV31":[{"type":"Primary","cvssData":{"baseScore":8.8}}]},
  "configurations":[{"nodes":[{"cpeMatch":[{"vulnerable":true,"criteria":"cpe:2.3:a:proxmox:virtual_environment:*:*:*:*:*:*:*:*","versionStartIncluding":"8.0","versionEndExcluding":"8.2.7"}]}]}]}},
 {"cve":{"id":"CVE-2023-0002","vulnStatus":"Analyzed","metrics":{},
  "configurations":[{"nodes":[{"cpeMatch":[{"vulnerable":true,"criteria":"cpe:2.3:a:proxmox:virtual_environment:7.4:*:*:*:*:*:*:*"}]}]}]}},
 {"cve":{"id":"CVE-2024-0003","vulnStatus":"Analyzed","metrics":{"cvssMetricV31":[{"type":"Primary","cvssData":{"baseScore":5.0}}]},
  "configurations":[{"nodes":[{"cpeMatch":[{"vulnerable":true,"criteria":"cpe:2.3:a:proxmox:virtual_environment:*:*:*:*:*:*:*:*","versionEndIncluding":"8.2.4"}]}]}]}},
 {"cve":{"id":"CVE-2024-0004","vulnStatus":"Rejected","metrics":{},
  "configurations":[{"nodes":[{"cpeMatch":[{"vulnerable":true,"criteria":"cpe:2.3:a:proxmox:virtual_environment:*:*:*:*:*:*:*:*"}]}]}]}}]}`))
	}))
	defer nvd.Close()
	s.cfg.NVDURL = nvd.URL

	st, _ := json.Marshal(apps.State{ScannedAt: time.Now().Unix(), Apps: []apps.App{
		{Name: "Proxmox VE", Installed: true, Version: "8.2.4", Full: "8.2.4", Package: "pve-manager", Origin: "Proxmox"}}})
	upd, _ := json.Marshal(map[string]any{"packages": []map[string]string{{"name": "pve-manager", "available": "8.2.8"}}})
	s.st.mu.Lock()
	a := &Agent{ID: "a_1", Name: "pve", Status: StatusActive, Facts: proto.Facts{OSID: "debian", OSVersion: "12"},
		State: map[string]json.RawMessage{"apps": st, "updates": upd}}
	s.st.st.Agents["a_1"] = a
	s.st.mu.Unlock()

	if v := s.vulnsFor(a, true); v.Apps["Proxmox VE"].Status != "pending" {
		t.Errorf("before lookup: %+v", v.Apps["Proxmox VE"])
	}
	if p := s.nextProduct(time.Now()); p != "pve" {
		t.Fatalf("next product %q", p)
	}
	if err := s.lookupProduct("pve", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asked, "virtualMatchString=cpe%3A2.3%3Aa%3Aproxmox%3Avirtual_environment") || strings.Contains(asked, "8.2.4") {
		t.Errorf("asked %s", asked)
	}
	if p := s.nextProduct(time.Now()); p != "" {
		t.Errorf("asked again: %q", p)
	}
	av := s.vulnsFor(a, true).Apps["Proxmox VE"]
	if av.Status != "found" || av.Fixable != 1 || av.Open != 1 || len(av.CVEs) != 2 {
		t.Fatalf("found: %+v", av)
	}
	c := av.CVEs[0]
	if c.ID != "CVE-2024-0001" || c.Fixed != "8.2.7" || !c.ByUpdate || c.Score != 8.8 || !strings.HasPrefix(c.Link, nvdLink) {
		t.Errorf("first: %+v", c)
	}
	if c := av.CVEs[1]; c.ID != "CVE-2024-0003" || c.Fixed != "" {
		t.Errorf("second: %+v", c)
	}
	// Kept when the OSV data is pruned.
	s.vul.mu.Lock()
	s.pruneLocked(map[string]bool{}, map[string]bool{})
	_, kept := s.vul.d.CVEs["CVE-2024-0001"]
	s.vul.mu.Unlock()
	if !kept {
		t.Error("product CVE pruned")
	}
}
