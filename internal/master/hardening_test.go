package master

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coremon/internal/feature/updates"
	"coremon/internal/proto"
)

// Website inputs, sent straight to the master (no website in between).
func TestInputHardening(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.st.st.Agents["a_1"] = &Agent{ID: "a_1", Name: "web-01", Status: StatusPending}
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	if code, _ := c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`); code != 200 {
		t.Fatal("login")
	}
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	bad := []string{
		"web<script>", `a"b`, "a'b", "a`b", "a$b", `a\b`, "a;b", "a|b", "a&b", "a>b",
		"line\nbreak", "tab\there", "esc\x1b[31m", "nul\x00", "del\x7f", "c1\u0085",
		"zero\u200bwidth", "bidi\u202eevil", "sep\u2028x", "priv\ue000", "bom\ufeff",
	}
	long := map[string]int{"system": maxSystemName, "group": maxGroupName, "note": maxNoteLen}
	fields := map[string]func(string) (int, string){
		"system": func(v string) (int, string) {
			return c.do("POST", "/api/ui/agents/a_1", `{"name":`+q(v)+`}`)
		},
		"group": func(v string) (int, string) { return c.do("POST", "/api/ui/groups", `{"name":`+q(v)+`}`) },
		"note":  func(v string) (int, string) { return c.do("POST", "/api/ui/tokens", `{"note":`+q(v)+`,"ttl_hours":1}`) },
	}
	for field, send := range fields {
		for _, v := range bad {
			if code, body := send(v); code != 400 || !strings.Contains(body, "not allowed") {
				t.Errorf("%s %q accepted: %d %s", field, v, code, body)
			}
		}
		if code, body := send(strings.Repeat("é", long[field]+1)); code != 400 || !strings.Contains(body, "too long") {
			t.Errorf("%s too long accepted: %d %s", field, code, body)
		}
	}
	// Normal names, also with accents and at the limit (in characters).
	if code, body := c.do("POST", "/api/ui/agents/a_1", `{"name":"Büro-server 2 (oud)"}`); code != 200 {
		t.Errorf("normal name refused: %d %s", code, body)
	}
	if code, body := c.do("POST", "/api/ui/groups", `{"name":`+q(strings.Repeat("é", maxGroupName))+`}`); code != 200 {
		t.Errorf("name at the limit refused: %d %s", code, body)
	}
	if code, _ := c.do("POST", "/api/ui/agents/a_1/approve", `{"name":"x;rm -rf /"}`); code != 400 {
		t.Error("approve with a bad name accepted")
	}

	// Passwords: 5-40 characters wherever one is set.
	for _, n := range []int{4, 41} {
		pwv := strings.Repeat("p", n)
		if code, _ := c.do("POST", "/api/ui/users", `{"name":"u`+fmt.Sprint(n)+`","role":"viewer","password":"`+pwv+`"}`); code != 400 {
			t.Errorf("new user with a %d-character password accepted", n)
		}
		if code, _ := c.do("POST", "/api/ui/password", `{"current":"x","new":"`+pwv+`"}`); code != 400 {
			t.Errorf("own password of %d characters accepted", n)
		}
	}
	if code, _ := c.do("POST", "/api/ui/users", `{"name":"forty","role":"viewer","password":`+q(strings.Repeat("€", 40))+`}`); code != 200 {
		t.Error("40-character password refused")
	}
	if code, _ := c.do("POST", "/api/ui/users", `{"name":"ok","role":"viewer","password":"abcde"}`); code != 200 {
		t.Fatal("create user")
	}
	var uid string
	s.st.mu.Lock()
	uid = s.st.userByName("ok").ID
	s.st.mu.Unlock()
	if code, _ := c.do("POST", "/api/ui/users/"+uid, `{"password":`+q(strings.Repeat("x", 41))+`}`); code != 400 {
		t.Error("admin reset to 41 characters accepted")
	}

	// Package names: the master checks them too.
	s.st.mu.Lock()
	s.st.st.Agents["a_1"].Status = StatusActive
	s.st.st.Agents["a_1"].Features = []proto.FeatureInfo{{Name: "updates", Actions: []proto.ActionSpec{{Name: "install", Mutating: true}}}}
	s.st.mu.Unlock()
	for _, p := range []string{"curl;reboot", "-o=evil", "$(id)", "Curl", "a b"} {
		if code, _ := c.do("POST", "/api/ui/actions", `{"agent_ids":["a_1"],"feature":"updates","action":"install","params":{"packages":[`+q(p)+`]}}`); code != 400 {
			t.Errorf("package %q accepted", p)
		}
	}
	if code, _ := c.do("POST", "/api/ui/actions", `{"agent_ids":["a_1"],"feature":"up dates","action":"install","params":{}}`); code != 400 {
		t.Error("bad feature name accepted")
	}
	if code, body := c.do("POST", "/api/ui/actions", `{"agent_ids":["a_1"],"feature":"updates","action":"install","params":{"packages":["libssl3:amd64","g++"]}}`); code != 200 || !strings.Contains(body, `"jobs":["j_`) {
		t.Errorf("valid packages refused: %d %s", code, body)
	}
}

// A rogue agent: dirty facts, bad results, future times, floods.
func TestRogueAgent(t *testing.T) {
	f := sanitizeFacts(proto.Facts{
		Hostname: "evil\x1b[2J<script>alert(1)</script>\u202e", OSName: "Debian\nGNU", IPs: []string{"10.0.0.1", "<b>", "999.1.1.1"},
		Gateway: "192.168.5.1", GatewayIf: "eth0;reboot",
		Health: &proto.Health{At: time.Now().Add(time.Hour).Unix(), Failed: []string{"x.service\x1b]0;pwn\x07", "\u200b"},
			LastUpgrade: &proto.Upgrade{Time: time.Now().Add(24 * time.Hour).Unix(), By: "user:<root>"}},
	})
	if f.Hostname != "evil[2Jscriptalert(1)/script" || f.OSName != "DebianGNU" || len(f.IPs) != 1 || f.GatewayIf != "eth0reboot" {
		t.Errorf("facts not cleaned: %q %q %v %q", f.Hostname, f.OSName, f.IPs, f.GatewayIf)
	}
	if f.Health.At > time.Now().Unix() || f.Health.LastUpgrade.Time > time.Now().Unix() || f.Health.LastUpgrade.By != "user:root" {
		t.Errorf("health: %+v %+v", f.Health, f.Health.LastUpgrade)
	}
	if len(f.Health.Failed) != 1 || f.Health.Failed[0] != "x.service]0;pwn" && f.Health.Failed[0] != "x.service]0pwn" {
		t.Errorf("failed units: %q", f.Health.Failed)
	}

	// Update results.
	good := updates.Snapshot{Backend: "apt", CheckedAt: time.Now().Add(48 * time.Hour).Unix(), Total: 99, Security: -1,
		Packages: []updates.Package{{Name: "libssl3", Available: "3.0.2\x1b[0m", Origin: "Debian<b>", Security: true}}}
	raw, _ := json.Marshal(good)
	out, err := checkState("updates", raw)
	if err != nil {
		t.Fatal(err)
	}
	var got updates.Snapshot
	json.Unmarshal(out, &got)
	if got.CheckedAt > time.Now().Unix() || got.Total != 1 || got.Security != 1 || got.Packages[0].Available != "3.0.2[0m" || got.Packages[0].Origin != "Debianb" {
		t.Errorf("state not cleaned: %+v", got)
	}
	for name, s := range map[string]updates.Snapshot{
		"bad name":      {Packages: []updates.Package{{Name: "evil;reboot"}}},
		"bad removal":   {Removals: []string{"$(id)"}},
		"too many pkgs": {Packages: make([]updates.Package, maxStatePkgs+1)},
	} {
		raw, _ := json.Marshal(s)
		if _, err := checkState("updates", raw); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := checkState("other", json.RawMessage(`"`+strings.Repeat("x", maxStateBytes)+`"`)); err == nil {
		t.Error("oversized state accepted")
	}

	// A refused result fails the job and keeps the previous state.
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	a := &Agent{ID: "a_1", Name: "web-01", Status: StatusActive, State: map[string]json.RawMessage{"updates": json.RawMessage(`{"total":1}`)}, StateAt: map[string]int64{}}
	s.st.st.Agents["a_1"] = a
	s.st.mu.Lock()
	j := s.enqueue(a, &Job{Type: proto.CmdAction, Feature: "updates", Action: "check"})
	j.Status = JobRunning
	s.st.mu.Unlock()
	bad, _ := json.Marshal(map[string]any{"state": updates.Snapshot{Packages: []updates.Package{{Name: "x;y"}}}})
	s.resultCore("a_1", proto.Result{CommandID: j.ID, OK: true, Data: bad, Error: "x\x1b[31m"}, "127.0.0.1")
	if j.Status != JobFailed || !strings.Contains(j.Error, "refused") || string(a.State["updates"]) != `{"total":1}` {
		t.Errorf("refused result: %s %q %s", j.Status, j.Error, a.State["updates"])
	}

	// Output: control characters removed.
	s.st.mu.Lock()
	j2 := s.enqueue(a, &Job{Type: proto.CmdAction, Feature: "updates", Action: "install"})
	j2.Status = JobRunning
	s.st.mu.Unlock()
	s.outputCore("a_1", proto.OutputBatch{Items: []proto.OutputItem{{CommandID: j2.ID, Data: "ok\x1b[2J\x07\nnext 'line' <b>\n"}}})
	b, _ := os.ReadFile(s.st.jobLogPath(j2.ID))
	if string(b) != "ok[2J\nnext 'line' <b>\n" {
		t.Errorf("job output: %q", b)
	}

	// One open heartbeat per agent: a second one ends the first.
	s.st.st.Settings.HeartbeatSec = 30
	first := make(chan time.Duration, 1)
	go func() {
		t0 := time.Now()
		s.pollCore(context.Background(), "a_1", proto.PollRequest{})
		first <- time.Since(t0)
	}()
	time.Sleep(300 * time.Millisecond)
	go s.pollCore(context.Background(), "a_1", proto.PollRequest{})
	select {
	case d := <-first:
		if d > 5*time.Second {
			t.Errorf("first heartbeat took %s", d)
		}
	case <-time.After(5 * time.Second):
		t.Error("the first heartbeat was not ended by the second")
	}

	// Request rate: a burst passes, a flood is refused, and it recovers.
	var l rateLimiter
	now := time.Now()
	n := 0
	for i := 0; i < 200; i++ {
		if l.allow("a_1", now) {
			n++
		}
	}
	if n != agentBurst {
		t.Errorf("burst allowed %d, want %d", n, agentBurst)
	}
	if l.allow("a_1", now) || !l.allow("a_1", now.Add(200*time.Millisecond)) || !l.allow("a_2", now) {
		t.Error("rate limit: flood not refused, no recovery, or other agents affected")
	}
}

func TestTextRules(t *testing.T) {
	if got := cleanText("  Debian GNU/Linux 13 (trixie)  ", 256); got != "Debian GNU/Linux 13 (trixie)" {
		t.Errorf("normal text changed: %q", got)
	}
	if got := cleanText(strings.Repeat("é", 10), 4); got != "éééé" {
		t.Errorf("cut by characters: %q", got)
	}
	if got := cleanText("bad\xffutf8", 64); got != "badutf8" {
		t.Errorf("invalid UTF-8: %q", got)
	}
	if got := cleanLog("a\r\nb\tc\x00\u202e"); got != "a\nb\tc" {
		t.Errorf("log: %q", got)
	}
	if _, err := checkName("x", "Büro 2 (test) #1 @home/lab:8 ~ok", 64); err != nil {
		t.Errorf("normal punctuation refused: %v", err)
	}
}
