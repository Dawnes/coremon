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

	"coremon/internal/proto"
)

// Virtual devices pass on the state above them; Upstream is managed only in
// its settings, is pinged by the master and is never a cause.
func TestVirtualAndUpstream(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	now := time.Now().Unix()
	answer := map[string]bool{"9.9.9.9": true, "1.1.1.1": true}
	s.pinger = func(ctx context.Context, ip net.IP, timeout time.Duration) (time.Duration, error) {
		if answer[ip.String()] {
			return 14 * time.Millisecond, nil
		}
		return 0, errors.New("no answer")
	}
	s.st.mu.Lock()
	s.st.st.Locals["l_rt"] = &LocalDevice{ID: "l_rt", Name: "router", Checks: Checks{Address: "192.168.5.1"}}
	s.st.st.Locals["l_pc"] = &LocalDevice{ID: "l_pc", Name: "printer", Checks: Checks{Address: "192.168.5.3"}}
	s.st.st.Agents["a_1"] = &Agent{ID: "a_1", Name: "web", Status: StatusActive, LastSeen: now, Facts: proto.Facts{IPs: []string{"192.168.5.9"}, Gateway: "192.168.5.1"}}
	s.st.st.Agents["a_2"] = &Agent{ID: "a_2", Name: "db", Status: StatusActive, LastSeen: now, Facts: proto.Facts{IPs: []string{"192.168.5.10"}, Gateway: "192.168.5.1"}}
	s.st.mu.Unlock()
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)

	// A virtual switch behind the router, the printer behind the switch.
	code, b := c.do("POST", "/api/ui/virtuals", `{"name":"switch","via":"l_rt"}`)
	var v VirtualDevice
	json.Unmarshal([]byte(b), &v)
	if code != 200 || !strings.HasPrefix(v.ID, "v_") || v.Via != "l_rt" {
		t.Fatalf("create: %d %s", code, b)
	}
	if code, _ := c.do("POST", "/api/ui/virtuals", `{"name":""}`); code != 400 {
		t.Error("no name accepted")
	}
	if code, b := c.do("POST", "/api/ui/locals/l_pc", `{"via":"`+v.ID+`"}`); code != 200 {
		t.Fatalf("via virtual: %d %s", code, b)
	}
	if code, _ := c.do("POST", "/api/ui/virtuals/"+v.ID, `{"via":"l_pc"}`); code != 400 {
		t.Error("loop accepted")
	}

	s.st.mu.Lock()
	rt, pc := s.st.st.Locals["l_rt"], s.st.st.Locals["l_pc"]
	rt.Checked, rt.Up = true, false
	pc.Checked, pc.Up = true, false
	// The router is down: the printer is unreachable behind the router, not the switch.
	if x := s.localDeviceView(pc, true); x.Unreach == nil || x.Unreach.ID != "l_rt" {
		t.Errorf("printer behind virtual: %+v", x.Unreach)
	}
	rt.Up = true
	if x := s.localDeviceView(pc, true); x.Unreach != nil {
		t.Errorf("router up, printer itself down: %+v", x.Unreach)
	}
	if g := s.gateway(); g == nil || g.ID != "l_rt" || g.Count != 2 || g.Done {
		t.Errorf("gateway: %+v", g)
	}
	s.st.mu.Unlock()

	// Upstream: not a device until switched on; then pinged.
	if code, _ := c.do("POST", "/api/ui/upstream", `{"enabled":true,"name":"Internet","addrs":["9.9.9.9","86.54.11.100","nope"],"show_tile":true}`); code != 400 {
		t.Error("bad address accepted")
	}
	if code, b := c.do("POST", "/api/ui/upstream", `{"enabled":true,"name":"Internet","addrs":["9.9.9.9","86.54.11.100","1.1.1.1"],"show_tile":true}`); code != 200 {
		t.Fatalf("enable: %d %s", code, b)
	}
	s.upstreamRound()
	s.st.mu.Lock()
	u := s.upstreamView()
	s.st.mu.Unlock()
	if u.State != "partial" || u.Answer != 2 || u.Total != 3 || u.RTTus != 14000 || u.Name != "Internet" {
		t.Errorf("upstream: %+v", u)
	}
	if code, b := c.do("POST", "/api/ui/locals/l_rt", `{"via":"`+upstreamID+`"}`); code != 200 {
		t.Fatalf("router via upstream: %d %s", code, b)
	}
	if code, _ := c.do("POST", "/api/ui/virtuals/"+upstreamID+"/delete", ""); code != 409 {
		t.Error("upstream deleted as a normal device")
	}
	answer = map[string]bool{}
	s.upstreamRound()
	s.st.mu.Lock()
	if u := s.upstreamView(); u.State != "down" || u.Since == 0 {
		t.Errorf("upstream down: %+v", u)
	}
	rt.Up = false
	// Upstream down is never a cause: the router itself is simply down.
	if x := s.localDeviceView(rt, true); x.Unreach != nil {
		t.Errorf("router behind upstream: %+v", x.Unreach)
	}
	if g := s.gateway(); g == nil || !g.Done {
		t.Errorf("gateway done: %+v", g)
	}
	s.st.mu.Unlock()

	// Switching off removes the device and every "Connected via" to it.
	if code, b := c.do("POST", "/api/ui/upstream", `{"enabled":false,"name":"Internet","addrs":["9.9.9.9","",""],"show_tile":false}`); code != 200 {
		t.Fatalf("disable: %d %s", code, b)
	}
	s.st.mu.Lock()
	if s.st.st.Virtuals[upstreamID] != nil || rt.Via != "" || !s.st.st.Upstream.HideTile {
		t.Errorf("after off: %v via=%q", s.st.st.Virtuals, rt.Via)
	}
	s.st.mu.Unlock()

	// Removing the switch frees the printer.
	if code, _ := c.do("POST", "/api/ui/virtuals/"+v.ID+"/delete", ""); code != 200 || pc.Via != "" {
		t.Errorf("delete: %d via=%q", code, pc.Via)
	}
}
