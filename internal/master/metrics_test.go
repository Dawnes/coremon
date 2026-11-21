package master

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coremon/internal/proto"
)

// Disk, memory and load: measured by the device's own agent, down after
// the set readings in a row past the limit; never checked from outside.
func TestAgentMetrics(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	now := time.Now()
	s.st.mu.Lock()
	s.st.st.Agents["a_web"] = &Agent{ID: "a_web", Name: "web", Status: StatusActive, LastSeen: now.Unix(), CoreVersion: "0.3.12",
		Facts: proto.Facts{IPs: []string{"10.1.0.5"}}}
	s.st.st.Locals["l_fw"] = &LocalDevice{ID: "l_fw", Name: "fw", Checks: Checks{Address: "10.1.0.1"}}
	s.st.mu.Unlock()
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	a := s.st.st.Agents["a_web"]

	if code, _ := c.do("POST", "/api/ui/locals/l_fw/services", `{"preset":"disk"}`); code != 400 {
		t.Error("disk on an agentless device")
	}
	for _, x := range []struct {
		body string
		want int
	}{
		{`{"preset":"disk"}`, 200},
		{`{"preset":"disk","name":"Root again"}`, 409}, // / is watched already
		{`{"preset":"disk","name":"Data","mount":"/var","limit_kind":"free"}`, 200},
		{`{"preset":"memory"}`, 200},
		{`{"preset":"memory","name":"Mem 2"}`, 409},
		{`{"preset":"load","limit":0}`, 400},
		{`{"preset":"load"}`, 200},
		{`{"preset":"disk","name":"X","mount":"var"}`, 400},
		{`{"preset":"disk","name":"Y","mount":"/y","readings":0}`, 400},
	} {
		if code, b := c.do("POST", "/api/ui/locals/a_web/services", x.body); code != x.want {
			t.Errorf("%s: %d %s, want %d", x.body, code, b, x.want)
		}
	}
	if a.Checks == nil || a.Checks.Address != "" || len(a.Checks.Services) != 4 {
		t.Fatalf("checks: %+v", a.Checks)
	}
	root, data, mem, load := a.Checks.Services[0], a.Checks.Services[1], a.Checks.Services[2], a.Checks.Services[3]
	if root.Mount != "/" || root.Limit != 90 || root.Readings != 3 || data.LimitKind != "free" || data.Limit != 10 || mem.Readings != 5 || load.Limit != 1.5 {
		t.Errorf("defaults: %+v %+v %+v %+v", root, data, mem, load)
	}
	// Not checked from outside: no schedule, no probe list, no Check now.
	s.st.mu.Lock()
	s.loc.mu.Lock()
	due := s.dueServices(s.checkDevs(), now.Add(time.Hour))
	s.loc.mu.Unlock()
	s.st.mu.Unlock()
	if len(due) != 0 {
		t.Errorf("scheduled: %+v", due)
	}
	if code, _ := c.do("POST", "/api/ui/locals/a_web/services/"+root.ID+"/check", ""); code != 409 {
		t.Error("check now on a measured service")
	}

	// Readings: / at 95% three times (down after 3), memory 50%, load 0.5 per core.
	const g = 1 << 30
	sample := func(ago int64, rootPct float64) proto.MetricSample {
		free := uint64(100 * g * (100 - rootPct) / 100)
		return proto.MetricSample{AgoMs: ago, CPUs: 4, Load1: 2, Load5: 1, Load15: 1, MemTotal: 16 * g, MemAvail: 8 * g,
			Disks: []proto.DiskUse{{Mount: "/", Size: 100 * g, Free: free, Avail: free}, {Mount: "/var", Size: 400 * g, Free: 5 * g, Avail: 5 * g}}}
	}
	s.st.mu.Lock()
	s.metricIntake(a, &proto.Metrics{Disks: []proto.DiskInfo{{Mount: "/", FS: "ext4", Size: 100 * g}, {Mount: "/var", FS: "xfs", Size: 400 * g}},
		Samples: []proto.MetricSample{sample(180000, 95), sample(120000, 95)}}, now)
	if !root.Up && root.Checked || root.Fails != 2 || root.Value != 95 {
		t.Errorf("after 2 readings: %+v", root)
	}
	s.metricIntake(a, &proto.Metrics{Samples: []proto.MetricSample{sample(60000, 95), sample(60000, 95)}}, now) // the same reading twice: once
	if !root.Checked || root.Up || root.Fails != 3 || !strings.Contains(root.LastError, "95% used") {
		t.Errorf("after 3 readings: %+v", root)
	}
	if !data.Checked || data.Up || !strings.Contains(data.LastError, "less than 10 GB") || data.Value != 5 {
		t.Errorf("/var free: %+v", data)
	}
	if !mem.Up || mem.Value != 50 || !load.Up || load.Value != 0.5 || !strings.Contains(load.Banner, "on 4 cores") {
		t.Errorf("memory/load: %+v %+v", mem, load)
	}
	v := s.view(a, true)
	s.st.mu.Unlock()
	if len(v.Disks) != 2 || v.Checks == nil || v.Checks.SvcDown != 2 || len(v.Checks.Services[0].Vals) == 0 || v.Checks.Services[0].Unit != "% used" {
		t.Errorf("view: %+v", v.Checks)
	}
	// Agent offline: unknown, not down.
	s.st.mu.Lock()
	a.LastSeen = now.Unix() - 3600
	v = s.view(a, true)
	s.st.mu.Unlock()
	if v.Checks.SvcDown != 0 || !v.Checks.Services[0].Stale {
		t.Errorf("offline: down %d", v.Checks.SvcDown)
	}
	// A check from outside gets the agent's address then.
	if code, b := c.do("POST", "/api/ui/locals/a_web/services", `{"preset":"ssh"}`); code != 200 || a.Checks.Address != "10.1.0.5" {
		t.Errorf("ssh after metrics: %d %s %q", code, b, a.Checks.Address)
	}
	// Changing the mount starts over; the limit does not.
	if code, _ := c.do("POST", "/api/ui/locals/a_web/services/"+root.ID, `{"limit":97}`); code != 200 || len(root.Vals) == 0 {
		t.Error("limit change cleared the history")
	}
	if code, _ := c.do("POST", "/api/ui/locals/a_web/services/"+root.ID, `{"mount":"/boot"}`); code != 200 || len(root.Vals) != 0 || root.Checked {
		t.Error("mount change kept the history")
	}
	// Saved and restored with the readings.
	s.st.mu.Lock()
	js, _ := s.checksJSON()
	s.st.mu.Unlock()
	var cd checksData
	json.Unmarshal(js, &cd)
	if r := cd.Devices["a_web"].Services[data.ID]; len(r.Vals) == 0 || r.Value != 5 {
		t.Errorf("checks.dat: %+v", r)
	}
	// An agent device may end up without any check: the outside one first,
	// then the measured ones, even when only measured ones are left.
	var ids []string
	for _, sv := range a.Checks.Services {
		if !sv.isMetric() {
			ids = append([]string{sv.ID}, ids...)
		} else {
			ids = append(ids, sv.ID)
		}
	}
	if code, b := c.do("POST", "/api/ui/locals/a_web/services/"+load.ID+"/pause", `{"paused":true}`); code != 200 {
		t.Errorf("pause a measured service: %d %s", code, b)
	}
	for _, id := range ids {
		if code, b := c.do("POST", "/api/ui/locals/a_web/services/"+id+"/delete", ""); code != 200 {
			t.Errorf("delete %s: %d %s", id, code, b)
		}
	}
	if len(a.Checks.Services) != 0 {
		t.Errorf("left: %d", len(a.Checks.Services))
	}
}
