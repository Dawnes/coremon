package master

// Proxmox VE and Proxmox Backup Server are not in the Debian security data
// (they come from Proxmox's own package source) and Proxmox has no
// machine-readable advisory feed. Their CVEs come from NVD instead: all CVEs
// of the product's CPE, with the affected version ranges, are fetched in one
// question per product; which apply to an installed version is worked out
// here. Only the product name is sent, never a version or a host name.

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"coremon/internal/feature/apps"
)

type nvdProduct struct {
	ID   string // key in vulnData.Products
	App  string // the application row (apps.go)
	CPE  string // vendor:product prefix
	Name string
}

var nvdProducts = []nvdProduct{
	{"pve", "Proxmox VE", "cpe:2.3:a:proxmox:virtual_environment", "Proxmox VE"},
	{"pbs", "Proxmox Backup Server", "cpe:2.3:a:proxmox:backup_server", "Proxmox Backup Server"},
}

const (
	nvdLink         = "https://nvd.nist.gov/vuln/detail/"
	nvdProductRetry = time.Hour
	maxProductCVEs  = 2000
)

func productOf(app string) *nvdProduct {
	for i := range nvdProducts {
		if nvdProducts[i].App == app {
			return &nvdProducts[i]
		}
	}
	return nil
}

// prodData: every CVE NVD lists for a product.
type prodData struct {
	At   int64     `json:"at"`
	CVEs []prodCVE `json:"cves"`
}

type prodCVE struct {
	ID     string      `json:"id"`
	Score  float64     `json:"s,omitempty"`
	Scored bool        `json:"sc,omitempty"`
	M      []prodMatch `json:"m"`
}

// prodMatch: one affected CPE: an exact version, or a range of versions.
type prodMatch struct {
	Ver string `json:"v,omitempty"` // the CPE's version ("*" = any, then the range applies)
	SI  string `json:"si,omitempty"`
	SE  string `json:"se,omitempty"`
	EI  string `json:"ei,omitempty"`
	EE  string `json:"ee,omitempty"` // the first version that is not affected: the fix
}

// affects: whether the installed version ver is affected, and the version
// that fixes it (if known).
func (m prodMatch) affects(ver string) (bool, string) {
	switch m.Ver {
	case "-":
		return false, ""
	case "*", "":
	default:
		return debVerCmp(ver, m.Ver) == 0, ""
	}
	if m.SI != "" && debVerCmp(ver, m.SI) < 0 || m.SE != "" && debVerCmp(ver, m.SE) <= 0 ||
		m.EI != "" && debVerCmp(ver, m.EI) > 0 || m.EE != "" && debVerCmp(ver, m.EE) >= 0 {
		return false, ""
	}
	return true, m.EE
}

// wantedProductsLocked: the products installed somewhere. Caller holds s.st.mu.
func (s *Server) wantedProductsLocked() map[string]bool {
	out := map[string]bool{}
	for _, a := range s.st.st.Agents {
		if a.Status == StatusPending || s.belowMin(a) {
			continue
		}
		if st := appsOf(a); st != nil {
			for _, ap := range st.Apps {
				if p := productOf(ap.Name); p != nil && ap.Installed {
					out[p.ID] = true
				}
			}
		}
	}
	return out
}

// nextProduct: a product whose CVE list is missing or older than the
// vulnerability check interval.
func (s *Server) nextProduct(now time.Time) string {
	s.st.mu.Lock()
	want := s.wantedProductsLocked()
	days := s.st.st.Settings.VulnDays
	s.st.mu.Unlock()
	if days <= 0 {
		days = defaultVulnDay
	}
	s.vul.mu.Lock()
	defer s.vul.mu.Unlock()
	for _, p := range nvdProducts {
		if !want[p.ID] || now.Sub(s.vul.prodTry[p.ID]) < nvdProductRetry {
			continue
		}
		if d := s.vul.d.Products[p.ID]; d == nil || now.Sub(time.Unix(d.At, 0)) >= time.Duration(days)*24*time.Hour {
			return p.ID
		}
	}
	return ""
}

type nvdMetric struct {
	Type string `json:"type"`
	Data struct {
		Score float64 `json:"baseScore"`
	} `json:"cvssData"`
}

// nvdScore: the primary CVSS base score, newest CVSS version first.
func nvdScore(metrics map[string][]nvdMetric) (float64, bool) {
	for _, k := range []string{"cvssMetricV31", "cvssMetricV30", "cvssMetricV40", "cvssMetricV2"} {
		list := metrics[k]
		for _, m := range list {
			if m.Type == "Primary" {
				return m.Data.Score, true
			}
		}
		if len(list) > 0 {
			return list[0].Data.Score, true
		}
	}
	return 0, false
}

func (s *Server) lookupProduct(id, key string) error {
	var p *nvdProduct
	for i := range nvdProducts {
		if nvdProducts[i].ID == id {
			p = &nvdProducts[i]
		}
	}
	if p == nil {
		return nil
	}
	s.vul.mu.Lock()
	if s.vul.prodTry == nil {
		s.vul.prodTry = map[string]time.Time{}
	}
	s.vul.prodTry[id] = time.Now()
	s.vul.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	hdr := map[string]string{}
	if key != "" {
		hdr["apiKey"] = key
	}
	var list []prodCVE
	for start, page := 0, 0; ; page++ {
		var resp struct {
			Total int `json:"totalResults"`
			Vulns []struct {
				CVE struct {
					ID      string                 `json:"id"`
					Status  string                 `json:"vulnStatus"`
					Metrics map[string][]nvdMetric `json:"metrics"`
					Configs []struct {
						Nodes []struct {
							Match []struct {
								Vulnerable bool   `json:"vulnerable"`
								Criteria   string `json:"criteria"`
								SI         string `json:"versionStartIncluding"`
								SE         string `json:"versionStartExcluding"`
								EI         string `json:"versionEndIncluding"`
								EE         string `json:"versionEndExcluding"`
							} `json:"cpeMatch"`
						} `json:"nodes"`
					} `json:"configurations"`
				} `json:"cve"`
			} `json:"vulnerabilities"`
		}
		u := s.base(s.cfg.NVDURL, nvdBase) + "?virtualMatchString=" + url.QueryEscape(p.CPE) + fmt.Sprintf("&startIndex=%d", start)
		if _, err := s.vulnGet(ctx, "GET", u, nil, hdr, 32<<20, &resp); err != nil {
			return fmt.Errorf("NVD (%s): %w", p.Name, err)
		}
		for _, v := range resp.Vulns {
			c := v.CVE
			if !cveRe.MatchString(c.ID) || c.Status == "Rejected" {
				continue
			}
			pc := prodCVE{ID: c.ID}
			pc.Score, pc.Scored = nvdScore(c.Metrics)
			if pc.Score < 0 || pc.Score > 10 {
				pc.Score, pc.Scored = 0, false
			}
			for _, cf := range c.Configs {
				for _, n := range cf.Nodes {
					for _, m := range n.Match {
						rest, ok := strings.CutPrefix(m.Criteria, p.CPE+":")
						if !m.Vulnerable || !ok || len(pc.M) >= 64 {
							continue
						}
						ver, _, _ := strings.Cut(rest, ":")
						pm := prodMatch{Ver: clip(ver, 64), SI: clip(m.SI, 64), SE: clip(m.SE, 64), EI: clip(m.EI, 64), EE: clip(m.EE, 64)}
						pc.M = append(pc.M, pm)
					}
				}
			}
			if len(pc.M) > 0 && len(list) < maxProductCVEs {
				list = append(list, pc)
			}
		}
		start += len(resp.Vulns)
		if len(resp.Vulns) == 0 || start >= resp.Total || page >= 5 {
			break
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	now := time.Now().Unix()
	s.vul.mu.Lock()
	defer s.vul.mu.Unlock()
	if s.vul.d.Products == nil {
		s.vul.d.Products = map[string]*prodData{}
	}
	s.vul.d.Products[id] = &prodData{At: now, CVEs: list}
	for _, c := range list {
		m := s.vul.d.CVEs[c.ID]
		if m == nil {
			m = &cveMeta{}
			s.vul.d.CVEs[c.ID] = m
		}
		switch {
		case c.Scored && !m.Scored:
			m.Score, m.Scored, m.Src = c.Score, true, "nvd"
		case !c.Scored:
			m.NVDAt = now // NVD has no score yet: asked again later
		}
	}
	s.saveVulnsLocked()
	return nil
}

// productCVEs: the CVEs of a product that apply to the installed version.
// avail: the version an update would install ("" when none). Caller holds s.vul.mu.
func (s *Server) productCVEs(pd *prodData, ver, avail string) []cveView {
	d := &s.vul.d
	var out []cveView
	for _, pc := range pd.CVEs {
		hit, fixed := false, ""
		for _, m := range pc.M {
			if ok, f := m.affects(ver); ok {
				hit = true
				if f != "" && (fixed == "" || debVerCmp(f, fixed) < 0) {
					fixed = f
				}
			}
		}
		if !hit {
			continue
		}
		c := cveView{ID: pc.ID, Link: nvdLink + url.PathEscape(pc.ID), KEV: d.KEV[pc.ID], Fixed: fixed}
		if m := d.CVEs[pc.ID]; m != nil {
			c.EPSS, c.Pct = m.EPSS, m.Pct
			if m.Scored {
				c.Score, c.Scored, c.Src = m.Score, true, m.Src
				c.Level = scoreLevel(m.Score)
			}
		}
		if fixed != "" && avail != "" && debVerCmp(apps.Readable(avail), fixed) >= 0 {
			c.ByUpdate = true
		}
		out = append(out, c)
	}
	return out
}
