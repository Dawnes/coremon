package master

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coremon/internal/proto"
)

func TestKeyRotation(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.2.14")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)

	old := s.currentPin()
	now := time.Now().Unix()
	s.st.mu.Lock()
	a := &Agent{ID: "a_1", Name: "web-01", Status: StatusActive, Protocol: 4, LastSeen: now, Pins: [2]string{old, ""}}
	b := &Agent{ID: "a_2", Name: "old-vm", Status: StatusActive, Protocol: 3, LastSeen: now}
	s.st.st.Agents[a.ID], s.st.st.Agents[b.ID] = a, b
	s.st.mu.Unlock()
	pinJob := func(id string) *Job {
		s.st.mu.Lock()
		defer s.st.mu.Unlock()
		for i := len(s.st.st.Jobs) - 1; i >= 0; i-- {
			if j := s.st.st.Jobs[i]; j.AgentID == id && j.Type == proto.CmdSetPins && !j.done() {
				return j
			}
		}
		return nil
	}

	// 1. Generate: the master keeps serving the old key; agents are told
	//    to trust both.
	if code, body := c.do("POST", "/api/ui/identity/generate", `{}`); code != 200 {
		t.Fatalf("generate: %d %s", code, body)
	}
	next := s.desiredPins()[1]
	if next == "" || next == old || s.currentPin() != old {
		t.Fatalf("after generate: current %s next %s", s.currentPin(), next)
	}
	j := pinJob("a_1")
	if j == nil || !strings.Contains(string(j.Params), old) || !strings.Contains(string(j.Params), next) {
		t.Fatalf("agent not told to trust both keys: %+v", j)
	}
	if pinJob("a_2") != nil {
		t.Error("an agent without two slots must not get a key command")
	}

	// 2. Switching while systems lack the new key is refused unless forced.
	if code, body := c.do("POST", "/api/ui/identity/switch", `{}`); code != 409 || !strings.Contains(body, "web-01") || !strings.Contains(body, "old-vm") {
		t.Fatalf("switch with missing systems: %d %s", code, body)
	}
	// web-01 confirms; old-vm (no second slot) stays missing.
	s.st.mu.Lock()
	a.Pins = [2]string{old, next}
	j.Status = JobSuccess
	s.st.mu.Unlock()
	if code, body := c.do("POST", "/api/ui/identity/switch", `{}`); code != 409 || strings.Contains(body, "web-01") {
		t.Fatalf("web-01 is ready, only old-vm missing: %d %s", code, body)
	}

	// 3. Forced switch: new key in use, old key gone from disk, agents told
	//    to keep only the new key.
	if code, body := c.do("POST", "/api/ui/identity/switch", `{"force":true}`); code != 200 {
		t.Fatalf("forced switch: %d %s", code, body)
	}
	if s.currentPin() != next || s.desiredPins()[1] != "" {
		t.Fatalf("after switch: current %s, next %s", s.currentPin(), s.desiredPins()[1])
	}
	crt, key := certPaths(s.cfg.DataDir, "current")
	if _, p, err := loadCert(crt, key); err != nil || p != next {
		t.Fatalf("the new key must be stored as the current key: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.cfg.DataDir, "tls-next.key")); err == nil {
		t.Fatal("the new-key file must be gone after the switch")
	}
	j = pinJob("a_1")
	if j == nil || strings.Contains(string(j.Params), old) || !strings.Contains(string(j.Params), next) {
		t.Fatalf("agent not told to drop the old key: %+v", j)
	}
	e := s.audit.list("", "", 5)
	if len(e) == 0 || e[0].Action != "identity.switch" || !strings.Contains(e[0].Detail, "old-vm") {
		t.Errorf("audit of a forced switch: %+v", e)
	}

	// 4. Generate + cancel leaves the current key alone.
	c.do("POST", "/api/ui/identity/generate", `{}`)
	if code, _ := c.do("POST", "/api/ui/identity/cancel", `{}`); code != 200 || s.desiredPins()[1] != "" || s.currentPin() != next {
		t.Fatalf("cancel: current %s next %s", s.currentPin(), s.desiredPins()[1])
	}

	// The install script carries both keys during a rotation.
	c.do("POST", "/api/ui/identity/generate", `{}`)
	code, script := rawGet(t, srv.URL, "/install.sh", "192.168.5.108")
	if code != 200 || !strings.Contains(script, "NEXT='"+s.desiredPins()[1]+"'") || !strings.Contains(script, `"master_pins": ["$PIN", "$NEXT"]`) {
		t.Fatalf("install script during rotation: %d", code)
	}
}

func TestReadOnlyCannotRotate(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.2.14")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	admin := newUIClient(t, srv.URL)
	admin.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	admin.do("POST", "/api/ui/users", `{"name":"ro","role":"viewer","password":"abcde"}`)
	ro := newUIClient(t, srv.URL)
	ro.do("POST", "/api/ui/login", `{"username":"ro","password":"abcde"}`)
	if code, _ := ro.do("GET", "/api/ui/identity", ""); code != 200 {
		t.Error("read-only users may see the keys")
	}
	if code, _ := ro.do("POST", "/api/ui/identity/generate", `{}`); code != 403 {
		t.Errorf("read-only user generated a key: %d", code)
	}
}
