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

func TestRemoteProbe(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	now := time.Now().Unix()
	s.st.mu.Lock()
	s.st.st.Agents["a_p"] = &Agent{ID: "a_p", Name: "probe-1", Status: StatusReportOnly, LastSeen: now, ProbeCap: true, CoreVersion: "0.3.9",
		Facts: proto.Facts{IPs: []string{"10.9.0.2"}}}
	s.st.st.Agents["a_old"] = &Agent{ID: "a_old", Name: "old", Status: StatusActive, LastSeen: now, CoreVersion: "0.3.8",
		Facts: proto.Facts{IPs: []string{"10.9.0.3"}}}
	s.st.mu.Unlock()
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	if code, _ := c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`); code != 200 {
		t.Fatal("login")
	}
	_, body := c.do("POST", "/api/ui/locals", `{"name":"cam","address":"10.9.0.50"}`)
	var dv agentView
	json.Unmarshal([]byte(body), &dv)
	id := dv.ID
	d := s.st.st.Locals[id]

	// Only agents that can probe; a report-only agent may.
	if code, b := c.do("POST", "/api/ui/locals/"+id, `{"probe":"a_old"}`); code != 409 || !strings.Contains(b, "0.3.9") {
		t.Errorf("old agent as probe: %d %s", code, b)
	}
	if code, b := c.do("POST", "/api/ui/locals/"+id, `{"probe":"a_p"}`); code != 200 || d.Probe != "a_p" {
		t.Fatalf("probe chosen: %d %s", code, b)
	}
	_, choices := c.do("GET", "/api/ui/probe-agents", "")
	if !strings.Contains(choices, `"id":"a_p"`) || !strings.Contains(choices, `"note":"agent 0.3.9 or later needed"`) {
		t.Errorf("choices: %s", choices)
	}

	// The same address is fine for another probe (another network), not for the same one.
	if code, _ := c.do("POST", "/api/ui/locals", `{"name":"cam-home","address":"10.9.0.50"}`); code != 200 {
		t.Error("same address checked by the master refused")
	}
	if code, _ := c.do("POST", "/api/ui/locals", `{"name":"cam2","address":"10.9.0.50","probe":"a_p"}`); code != 409 {
		t.Error("same address twice on one probe accepted")
	}

	// The master does not check it itself; Ping now is refused.
	if code, _ := c.do("POST", "/api/ui/locals/"+id+"/ping", ""); code != 409 {
		t.Error("ping now on a remote device")
	}

	// The list goes out as a job, built when sent.
	s.st.mu.Lock()
	a := s.st.st.Agents["a_p"]
	want := s.probeList("a_p")
	if len(want.Checks) != 1 || want.Checks[0].ID != id || want.Version == "" {
		t.Fatalf("list: %+v", want)
	}
	if !s.openJob("a_p", proto.CmdProbeList, "") {
		t.Fatal("no list sent")
	}
	cmds := s.takeCommands("a_p")
	s.st.mu.Unlock()
	if len(cmds) != 1 || cmds[0].Type != proto.CmdProbeList || !strings.Contains(string(cmds[0].Params), want.Version) {
		t.Fatalf("commands: %+v", cmds)
	}
	ack, _ := json.Marshal(proto.ProbeAck{Version: want.Version, Checks: 1})
	s.resultCore("a_p", proto.Result{CommandID: cmds[0].ID, OK: true, Data: ack}, "")
	if a.ProbeVer != want.Version {
		t.Fatalf("ack not stored: %q", a.ProbeVer)
	}

	// Results are stored like the master's own; old or repeated ones are skipped.
	s.st.mu.Lock()
	s.probeIntake(a, &proto.ProbeReport{Version: want.Version, Results: []proto.ProbeResult{
		{ID: id, AgoMs: 2000, OK: true, RTTUs: 4200},
		{ID: id, AgoMs: 2000, OK: true, RTTUs: 4200}, // the same again
		{ID: "l_other", OK: true},
	}}, time.Now())
	v := s.localDeviceView(d, false)
	s.st.mu.Unlock()
	if !d.Up || d.RTTUs != 4200 || len(d.Hist) != 1 || d.Hist[0].N != 1 || !v.Online || v.Local.Probe.State != "ok" {
		t.Fatalf("result: up %v rtt %d hist %+v view %+v", d.Up, d.RTTUs, d.Hist, v.Local.Probe)
	}

	// Result deliveries are listed on Remote Probes only with the setting on.
	results := func(at int64) (rows []probeRequest) {
		for _, p := range s.probeRequests(at, nil) {
			if p.Kind == "results" {
				rows = append(rows, p)
			}
		}
		return rows
	}
	s.st.mu.Lock()
	if r := results(time.Now().Unix()); len(r) != 0 {
		t.Fatalf("results listed while off: %+v", r)
	}
	s.st.st.Settings.ProbeResultsShown = true
	s.probeIntake(a, &proto.ProbeReport{Version: want.Version, Results: []proto.ProbeResult{{ID: id, AgoMs: 500, Error: "timeout"}}}, time.Now())
	r := results(time.Now().Unix())
	late := results(time.Now().Unix() + probeAnsweredKeep + 1)
	s.st.st.Settings.ProbeResultsShown = false
	s.st.mu.Unlock()
	if len(r) != 1 || r[0].State != "received" || r[0].Checks != 1 || r[0].Failed != 1 || len(r[0].Items) != 1 || r[0].Agent != a.Name {
		t.Fatalf("results rows: %+v", r)
	}
	if len(late) != 0 {
		t.Fatalf("results kept too long: %+v", late)
	}
	s.st.mu.Lock()
	s.probeIntake(a, &proto.ProbeReport{Version: want.Version, Results: []proto.ProbeResult{{ID: id, AgoMs: 100, OK: true, RTTUs: 4000}}}, time.Now())
	s.st.mu.Unlock()

	// Probe offline: unknown, not down.
	a.LastSeen = now - 3600
	s.st.mu.Lock()
	v = s.localDeviceView(d, false)
	s.st.mu.Unlock()
	if v.Online || v.Local.Checked || v.Local.Probe.State != "offline" {
		t.Errorf("offline probe: %+v", v.Local)
	}
	a.LastSeen = now

	// Clear: the agent drops its list; nothing is sent again by itself.
	if code, b := c.do("POST", "/api/ui/agents/a_p/probe/clear", ""); code != 200 {
		t.Fatalf("clear: %d %s", code, b)
	}
	s.st.mu.Lock()
	cmds = s.takeCommands("a_p")
	s.st.mu.Unlock()
	if len(cmds) != 1 || cmds[0].Type != proto.CmdProbeClear || a.ProbeHold == nil {
		t.Fatalf("clear commands: %+v", cmds)
	}
	ack, _ = json.Marshal(proto.ProbeAck{})
	s.resultCore("a_p", proto.Result{CommandID: cmds[0].ID, OK: true, Data: ack}, "")
	s.st.mu.Lock()
	s.probeSync(a, time.Now())
	open := s.openJob("a_p", proto.CmdProbeList, "")
	v = s.localDeviceView(d, false)
	s.st.mu.Unlock()
	if a.ProbeVer != "" || open || v.Local.Probe.State != "cleared" {
		t.Fatalf("after clear: ver %q, list sent %v, state %s", a.ProbeVer, open, v.Local.Probe.State)
	}

	// Resend: the list goes out again.
	if code, _ := c.do("POST", "/api/ui/agents/a_p/probe/resend", ""); code != 200 || a.ProbeHold != nil {
		t.Fatal("resend")
	}
	s.st.mu.Lock()
	open = s.openJob("a_p", proto.CmdProbeList, "")
	s.st.mu.Unlock()
	if !open {
		t.Error("resend queued no list")
	}

	// A change of the checks also ends a clear.
	c.do("POST", "/api/ui/agents/a_p/probe/clear", "")
	if code, b := c.do("POST", "/api/ui/locals/"+id+"/services", `{"preset":"ssh"}`); code != 200 {
		t.Fatalf("add service: %d %s", code, b)
	}
	if a.ProbeHold != nil {
		t.Error("a change of the checks did not end the clear")
	}

	// Overdue results show on Remote Probes.
	s.st.mu.Lock()
	for _, j := range s.st.st.Jobs {
		if !j.done() {
			j.Status, j.Finished = JobSuccess, now
		}
	}
	a.ProbeVer, a.ProbeAt = s.probeList("a_p").Version, now-3600
	d.LastCheck = now - 3600
	reqs := s.probeRequests(now, map[string]bool{})
	s.st.mu.Unlock()
	titles := []string{}
	for _, p := range reqs {
		if p.Kind == "check" && p.State == "overdue" {
			titles = append(titles, p.Title)
		}
	}
	if len(titles) != 2 {
		t.Errorf("overdue checks: %v", titles)
	}

	// At most 200 checks per probe.
	s.st.mu.Lock()
	err := s.probeAllowed("a_p", maxProbeChecks, map[string]bool{})
	onMasterErr := s.probeAllowed("a_p", 1, map[string]bool{"10.9.0.2": true})
	s.st.mu.Unlock()
	if err == nil || onMasterErr == nil {
		t.Errorf("limits: %v / %v", err, onMasterErr)
	}

	_, body = c.do("GET", "/api/ui/audit", "")
	for _, x := range []string{"agent.probe_cleared", "agent.probe_resent", "checked by Master → probe-1"} {
		if !strings.Contains(body, x) {
			t.Errorf("audit lacks %q", x)
		}
	}
}
