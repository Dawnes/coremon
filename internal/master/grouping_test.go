package master

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"coremon/internal/feature/apps"
	"coremon/internal/feature/updates"
	"coremon/internal/proto"
)

func TestJobsGrouped(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	mk := func(id, agent, actor, action string, created int64, status string) *Job {
		return &Job{ID: id, AgentID: agent, Actor: actor, Type: proto.CmdAction, Feature: "updates", Action: action, Created: created, Status: status}
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	s.st.st.Jobs = []*Job{
		mk("j0", "a1", "scheduler", "check", 100, JobSuccess),
		mk("j1", "a2", "scheduler", "check", 100, JobSuccess),
		mk("j2", "a3", "scheduler", "check", 101, JobFailed),
		mk("j3", "a1", "admin", "install", 200, JobSuccess),
		mk("j4", "a1", "scheduler", "check", 300, JobSuccess),
		mk("j5", "a1", "scheduler", "check", 301, JobSuccess), // same system again: not one batch
	}
	desc := func(l []jobEntry) string {
		var p []string
		for _, e := range l {
			s := e.ID
			for _, b := range e.Batch[min(1, len(e.Batch)):] {
				s += "+" + b.ID
			}
			p = append(p, s)
		}
		return strings.Join(p, " ")
	}
	all, more := s.jobsGrouped("", false, "", 0, 10)
	if desc(all) != "j5 j4 j3 j2+j1+j0" || more {
		t.Fatalf("grouped: %s %v", desc(all), more)
	}
	p1, more := s.jobsGrouped("", false, "", 0, 2)
	if desc(p1) != "j5 j4" || !more {
		t.Fatalf("page 1: %s", desc(p1))
	}
	p2, more := s.jobsGrouped("", false, "j4", 300, 2)
	if desc(p2) != "j3 j2+j1+j0" || more {
		t.Fatalf("page 2: %s", desc(p2))
	}
	s.st.st.Jobs[1].Status = JobRunning
	act, _ := s.jobsGrouped("", true, "", 0, 10)
	if desc(act) != "j2+j1+j0" {
		t.Fatalf("active: %s", desc(act))
	}
	if u, _ := s.jobsGrouped("admin", false, "", 0, 10); desc(u) != "j3" {
		t.Fatalf("user filter: %s", desc(u))
	}
	// With a system selected: individual jobs.
	if l, _ := s.jobsPage("a1", "", false, "", 0, 10); len(l) != 4 {
		t.Fatalf("one system: %d jobs", len(l))
	}
}

func TestAppInventory(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	state := func(ver string) json.RawMessage {
		return appsState(t,
			apps.App{Name: "OpenSSL", Installed: true, Version: strings.SplitN(ver, "-", 2)[0], Full: ver, Package: "libssl3"},
			apps.App{Name: "Tomcat 10", Installed: true, Version: "10.1.34", Full: "10.1.34-0+deb12u1", Package: "tomcat10"},
			apps.App{Name: "Nginx"})
	}
	upd, _ := json.Marshal(updates.Snapshot{Packages: []updates.Package{{Name: "libssl3", Available: "3.0.17-1~deb12u3"}}})
	s.st.mu.Lock()
	s.st.st.Agents["a1"] = &Agent{ID: "a1", Name: "web-02", Status: StatusActive, Facts: proto.Facts{OSName: "Debian 12"}, State: map[string]json.RawMessage{"apps": state("3.0.15-1~deb12u1"), "updates": upd}}
	s.st.st.Agents["a2"] = &Agent{ID: "a2", Name: "web-01", Status: StatusReportOnly, State: map[string]json.RawMessage{"apps": state("3.0.15-1~deb12u1")}}
	s.st.st.Agents["a3"] = &Agent{ID: "a3", Name: "db", Status: StatusActive, State: map[string]json.RawMessage{"apps": state("3.0.17-1~deb12u3")}}
	s.st.st.Agents["a4"] = &Agent{ID: "a4", Name: "new", Status: StatusPending, State: map[string]json.RawMessage{"apps": state("1.0-1")}}
	s.st.mu.Unlock()
	w := httptest.NewRecorder()
	s.handleAppInventory(w, httptest.NewRequest("GET", "/api/ui/apps", nil))
	var out struct {
		Apps []invApp `json:"apps"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	if len(out.Apps) != 2 || out.Apps[0].Name != "OpenSSL" || out.Apps[1].Name != "Tomcat 10" {
		t.Fatalf("apps: %s", w.Body.String())
	}
	v := out.Apps[0].Versions
	if len(v) != 2 || v[0].Full != "3.0.17-1~deb12u3" || len(v[1].Systems) != 2 {
		t.Fatalf("OpenSSL versions: %+v", v)
	}
	old := v[1].Systems
	if old[0].Name != "web-01" || old[1].Name != "web-02" || old[1].Update != "3.0.17-1~deb12u3" || old[0].Update != "" || old[1].OS != "Debian 12" {
		t.Errorf("systems of 3.0.15: %+v", old)
	}
	if strings.Contains(w.Body.String(), `"new"`) {
		t.Error("pending system listed")
	}
	if !naturalLess("tomcat 9", "tomcat 10") || naturalLess("php 8.2", "php 7.4") {
		t.Error("natural order")
	}
}
