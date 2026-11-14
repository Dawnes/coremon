package master

// The Application List: every monitored application found in the
// environment, per exact version, with the systems that run it.

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"coremon/internal/feature/updates"
)

type invSystem struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	GroupID string    `json:"group_id,omitempty"`
	OS      string    `json:"os"`
	Online  bool      `json:"online"`
	Status  string    `json:"status"`
	Update  string    `json:"update,omitempty"` // version of the waiting update
	Vulns   *appVulns `json:"vulns,omitempty"`
	Missing string    `json:"vuln_status,omitempty"` // why there is no CVE result (system level)
}

type invVersion struct {
	Version string      `json:"version"`
	Full    string      `json:"full"`
	Systems []invSystem `json:"systems"`
}

type invApp struct {
	Name     string        `json:"name"`
	Versions []*invVersion `json:"versions"`
}

func (s *Server) handleAppInventory(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	byApp := map[string]map[string]*invVersion{}
	now := time.Now().Unix()
	for _, a := range s.st.st.Agents {
		if a.Status == StatusPending || s.belowMin(a) {
			continue
		}
		st := appsOf(a)
		if st == nil {
			continue
		}
		upd := map[string]string{}
		if raw, ok := a.State["updates"]; ok {
			var u updates.Snapshot
			if json.Unmarshal(raw, &u) == nil {
				for _, p := range u.Packages {
					upd[p.Name] = p.Available
				}
			}
		}
		vv := s.vulnsFor(a, true)
		for _, ap := range st.Apps {
			if !ap.Installed {
				continue
			}
			if byApp[ap.Name] == nil {
				byApp[ap.Name] = map[string]*invVersion{}
			}
			v := byApp[ap.Name][ap.Full]
			if v == nil {
				v = &invVersion{Version: ap.Version, Full: ap.Full}
				byApp[ap.Name][ap.Full] = v
			}
			sys := invSystem{ID: a.ID, Name: a.Name, GroupID: a.GroupID, OS: releaseName(a.Facts), Online: s.online(a, now), Status: a.Status, Update: upd[ap.Package]}
			if vv != nil {
				if x := vv.Apps[ap.Name]; x != nil {
					c := *x
					c.CVEs = nil
					sys.Vulns = &c
				} else if vv.Status != "checked" {
					sys.Missing = vv.Status
				}
			}
			v.Systems = append(v.Systems, sys)
		}
	}
	groups := []Group{}
	for _, g := range s.st.st.Groups {
		groups = append(groups, *g)
	}
	s.st.mu.Unlock()
	sort.Slice(groups, func(i, j int) bool {
		return naturalLess(strings.ToLower(groups[i].Name), strings.ToLower(groups[j].Name))
	})
	out := []*invApp{}
	for name, vers := range byApp {
		app := &invApp{Name: name}
		for _, v := range vers {
			sort.Slice(v.Systems, func(i, j int) bool { return strings.ToLower(v.Systems[i].Name) < strings.ToLower(v.Systems[j].Name) })
			app.Versions = append(app.Versions, v)
		}
		sort.Slice(app.Versions, func(i, j int) bool { return debVerCmp(app.Versions[i].Full, app.Versions[j].Full) > 0 })
		out = append(out, app)
	}
	sort.Slice(out, func(i, j int) bool { return naturalLess(strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)) })
	writeJSON(w, map[string]any{"apps": out, "groups": groups})
}

// naturalLess sorts "php 7.4" before "php 8.2" and "tomcat 9" before "tomcat 10".
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		da, db := leadingDigits(a), leadingDigits(b)
		if da != "" && db != "" {
			if c := numCmp(da, db); c != 0 {
				return c < 0
			}
			a, b = a[len(da):], b[len(db):]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}
