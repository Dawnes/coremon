package master

import (
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coremon/internal/proto"
)

// A device behind a device that is down (or in maintenance) is unreachable,
// not down; the cause is the highest device of the run that is down.
func TestConnectedVia(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	now := time.Now().Unix()
	s.st.mu.Lock()
	s.st.st.Locals["l_core"] = &LocalDevice{ID: "l_core", Name: "core", Checks: Checks{Address: "10.0.0.1"}}
	s.st.st.Locals["l_sw"] = &LocalDevice{ID: "l_sw", Name: "switch", Checks: Checks{Address: "10.0.0.2"}}
	s.st.st.Locals["l_pc"] = &LocalDevice{ID: "l_pc", Name: "printer", Checks: Checks{Address: "10.0.0.3"}}
	s.st.st.Agents["a_web"] = &Agent{ID: "a_web", Name: "web", Status: StatusActive, LastSeen: now, Facts: proto.Facts{IPs: []string{"10.0.0.9"}}}
	s.st.st.Agents["a_new"] = &Agent{ID: "a_new", Name: "new", Status: StatusPending}
	s.st.mu.Unlock()
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)

	for path, body := range map[string]string{
		"/api/ui/locals/l_sw":  `{"via":"l_core"}`,
		"/api/ui/locals/l_pc":  `{"via":"l_sw"}`,
		"/api/ui/agents/a_web": `{"via":"l_sw"}`,
	} {
		if code, b := c.do("POST", path, body); code != 200 {
			t.Fatalf("%s: %d %s", path, code, b)
		}
	}
	// No loops, not itself, no pending or unknown device.
	for path, body := range map[string]string{
		"/api/ui/locals/l_core": `{"via":"l_pc"}`,
		"/api/ui/locals/l_sw":   `{"via":"l_sw"}`,
		"/api/ui/locals/l_pc":   `{"via":"a_new"}`,
		"/api/ui/agents/a_web":  `{"via":"l_nothere"}`,
	} {
		if code, _ := c.do("POST", path, body); code != 400 {
			t.Errorf("%s %s: %d, want 400", path, body, code)
		}
	}
	if code, b := c.do("POST", "/api/ui/locals", `{"name":"cam","address":"10.0.0.4","via":"a_web"}`); code != 200 || !strings.Contains(b, `"via":{"id":"a_web","name":"web"}`) {
		t.Errorf("add with via: %d %s", code, b)
	}

	set := func(id string, checked, up bool) {
		d := s.st.st.Locals[id]
		d.Checked, d.Up, d.Since = checked, up, now
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	sw, pc := s.st.st.Locals["l_sw"], s.st.st.Locals["l_pc"]
	set("l_core", true, true)
	set("l_sw", true, false)
	set("l_pc", true, false)
	// The switch is down, the core is up: the printer is unreachable behind the switch.
	if v := s.localDeviceView(pc, true); v.Unreach == nil || v.Unreach.ID != "l_sw" || len(v.ViaChain) != 2 || v.ViaChain[1].ID != "l_core" {
		t.Errorf("printer: %+v %+v", v.Unreach, v.ViaChain)
	}
	if v := s.localDeviceView(sw, false); v.Unreach != nil {
		t.Errorf("switch is down itself, not unreachable: %+v", v.Unreach)
	}
	// The core down too: the cause is the core, for both.
	set("l_core", true, false)
	if v := s.localDeviceView(pc, false); v.Unreach == nil || v.Unreach.ID != "l_core" {
		t.Errorf("printer behind core: %+v", v.Unreach)
	}
	set("l_core", true, true)
	// An offline agent behind the down switch.
	s.st.st.Agents["a_web"].LastSeen = now - 3600
	if v := s.view(s.st.st.Agents["a_web"], false); v.Unreach == nil || v.Unreach.ID != "l_sw" {
		t.Errorf("agent: %+v", v.Unreach)
	}
	// A device that still answers is up, whatever is above it.
	set("l_pc", true, true)
	if v := s.localDeviceView(pc, false); v.Unreach != nil {
		t.Errorf("printer up: %+v", v.Unreach)
	}
	set("l_pc", true, false)
	// The switch in maintenance: unreachable too; the switch up again: down.
	set("l_sw", true, true)
	if v := s.localDeviceView(pc, false); v.Unreach != nil {
		t.Errorf("switch up: %+v", v.Unreach)
	}
	sw.Maint = &Maintenance{Since: now}
	if v := s.localDeviceView(pc, false); v.Unreach == nil || v.Unreach.ID != "l_sw" {
		t.Errorf("switch in maintenance: %+v", v.Unreach)
	}
	// The history counts failures while unreachable.
	at := time.Now()
	s.record(&pc.Checks, 0, errors.New("no answer"), at)
	s.noteBehind(pc.ID, true, pc.Hist, at)
	if h := pc.Hist[len(pc.Hist)-1]; h.U != 1 || h.N != 1 {
		t.Errorf("history slot: %+v", h)
	}
	sw.Maint = nil
	s.record(&pc.Checks, 0, errors.New("no answer"), at)
	s.noteBehind(pc.ID, true, pc.Hist, at)
	if h := pc.Hist[len(pc.Hist)-1]; h.U != 1 || h.N != 2 {
		t.Errorf("history slot, switch up: %+v", h)
	}
	// A removed device carries no others.
	s.dropVia("l_sw")
	if pc.Via != "" || s.st.st.Agents["a_web"].Via != "" || sw.Via != "l_core" {
		t.Errorf("after removing the switch: %q %q", pc.Via, s.st.st.Agents["a_web"].Via)
	}
}
