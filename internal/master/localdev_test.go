package master

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalDevices(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.st.st.Agents["a_1"] = &Agent{ID: "a_1", Name: "web-01", Status: StatusActive}
	s.st.st.Agents["a_1"].Facts.IPs = []string{"192.168.5.41", "fe80::1", "fd00::5"}
	answer := map[string]bool{"192.168.5.50": true}
	s.pinger = func(ctx context.Context, ip net.IP, timeout time.Duration) (time.Duration, error) {
		if answer[ip.String()] {
			return 3 * time.Millisecond, nil
		}
		return 0, errors.New("no answer within 2s")
	}
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	if code, _ := c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`); code != 200 {
		t.Fatal("login")
	}

	// IP addresses only, and each address once across both types.
	for addr, want := range map[string]int{
		"printer.lan": 400, "192.168.5.0/24": 400, "127.0.0.1": 400, "fe80::1": 400, "224.0.0.1": 400, "": 400,
		"192.168.5.41": 409, "fd00::5": 409, "192.168.5.50": 200,
	} {
		code, body := c.do("POST", "/api/ui/locals", `{"name":"printer","address":"`+addr+`"}`)
		if code != want {
			t.Errorf("address %q: %d %s, want %d", addr, code, body, want)
		}
		if want == 409 && !strings.Contains(body, "web-01") {
			t.Errorf("duplicate %q should name the device using it: %s", addr, body)
		}
	}
	if code, _ := c.do("POST", "/api/ui/locals", `{"name":"again","address":"192.168.5.50"}`); code != 409 {
		t.Error("the same address added twice")
	}
	if code, _ := c.do("POST", "/api/ui/locals", `{"name":"x;y","address":"10.0.0.9"}`); code != 400 {
		t.Error("bad name accepted")
	}
	var id string
	for k := range s.st.st.Locals {
		id = k
	}
	if id == "" || !strings.HasPrefix(id, "l_") {
		t.Fatalf("no local device stored: %v", s.st.st.Locals)
	}

	// Ping now: up; the overview lists it with the agent devices.
	if code, body := c.do("POST", "/api/ui/locals/"+id+"/ping", ""); code != 200 || !strings.Contains(body, `"up":true`) {
		t.Fatalf("ping now: %d %s", code, body)
	}
	_, body := c.do("GET", "/api/ui/overview", "")
	var ov struct {
		Agents []agentView `json:"agents"`
	}
	json.Unmarshal([]byte(body), &ov)
	if len(ov.Agents) != 2 {
		t.Fatalf("overview: %s", body)
	}
	for _, v := range ov.Agents {
		if v.ID == id && (v.Monitor != "local" || !v.Online || v.Local == nil || v.Local.RTTUs != 3000) {
			t.Errorf("local view: %+v", v)
		}
		if v.ID == "a_1" && v.Monitor != "agent" {
			t.Errorf("agent view: %+v", v)
		}
	}

	// Agent actions never apply to a local device.
	if _, body := c.do("POST", "/api/ui/actions", `{"agent_ids":["`+id+`"],"feature":"updates","action":"check"}`); !strings.Contains(body, "agentless, monitored by the master") {
		t.Errorf("action on a local device: %s", body)
	}

	// Down only after the set number of failed pings in a row.
	answer["192.168.5.50"] = false
	for i := 1; i <= 3; i++ {
		s.pingOne(id)
		d := s.st.st.Locals[id]
		if wantUp := i < 3; d.Up != wantUp {
			t.Errorf("after %d failed ping(s): up=%v", i, d.Up)
		}
	}
	if d := s.st.st.Locals[id]; len(d.Hist) != 1 || d.Hist[0].N != 4 || d.Hist[0].OK != 1 {
		t.Errorf("history: %+v", d.Hist)
	}

	// An agent device reporting the same address: both show the conflict.
	s.st.mu.Lock()
	s.st.st.Agents["a_1"].Facts.IPs = append(s.st.st.Agents["a_1"].Facts.IPs, "192.168.5.50")
	s.st.mu.Unlock()
	_, body = c.do("GET", "/api/ui/agents/"+id, "")
	if !strings.Contains(body, `"ip_conflicts":[{"ip":"192.168.5.50","id":"a_1","name":"web-01"}]`) {
		t.Errorf("conflict on the local device: %s", body)
	}
	_, body = c.do("GET", "/api/ui/agents/a_1", "")
	if !strings.Contains(body, `"ip_conflicts":[{"ip":"192.168.5.50"`) {
		t.Errorf("conflict on the agent device: %s", body)
	}

	// Settings: bounds checked; edit and delete work and are audited.
	if code, _ := c.do("POST", "/api/ui/local", `{"interval_sec":5,"timeout_sec":2,"down_after":3}`); code != 400 {
		t.Error("interval below the minimum accepted")
	}
	if code, _ := c.do("POST", "/api/ui/local", `{"interval_sec":30,"timeout_sec":1,"down_after":2}`); code != 200 || s.st.st.Local.IntervalSec != 30 {
		t.Error("valid local settings refused")
	}
	if code, _ := c.do("POST", "/api/ui/locals/"+id, `{"address":"192.168.5.41"}`); code != 409 {
		t.Error("changing to a used address accepted")
	}
	if code, _ := c.do("POST", "/api/ui/locals/"+id, `{"name":"Printer hall","address":"10.1.1.1"}`); code != 200 || s.st.st.Locals[id].Checked {
		t.Error("edit, or a new address did not start over")
	}
	if code, _ := c.do("POST", "/api/ui/locals/"+id+"/delete", ""); code != 200 || len(s.st.st.Locals) != 0 {
		t.Error("delete")
	}
	_, body = c.do("GET", "/api/ui/audit", "")
	for _, a := range []string{"device.local_added", "device.local_changed", "device.local_removed", "settings.local"} {
		if !strings.Contains(body, a) {
			t.Errorf("audit log lacks %s", a)
		}
	}
}

func TestEchoMessages(t *testing.T) {
	m := echoRequest(false, 0x1234, 7)
	if m[0] != 8 || icmpChecksum(m) != 0 {
		t.Errorf("echo request: % x", m)
	}
	reply := append([]byte{}, m...)
	reply[0] = 0
	if !isEchoReply(reply, false, 0x1234, 7, true) || isEchoReply(reply, false, 0x1234, 8, true) || isEchoReply(m, false, 0x1234, 7, true) {
		t.Error("reply matching")
	}
	if !isEchoReply(reply, false, 0, 7, false) {
		t.Error("ping socket: the kernel picks the id")
	}
}
