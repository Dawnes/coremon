package master

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coremon/internal/proto"
	"coremon/internal/seal"
)

func TestPayloadKeyRotation(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)

	old := s.desiredPins()[0]
	httpsPin := s.currentPin()
	now := time.Now().Unix()
	s.st.mu.Lock()
	a := &Agent{ID: "a_1", Name: "web-01", Status: StatusActive, Protocol: 5, LastSeen: now, Pins: [2]string{old, ""}}
	b := &Agent{ID: "a_2", Name: "old-vm", Status: StatusActive, Protocol: 5, LastSeen: now}
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

	if code, body := c.do("POST", "/api/ui/identity/generate", `{}`); code != 200 {
		t.Fatalf("generate: %d %s", code, body)
	}
	next := s.desiredPins()[1]
	if next == "" || next == old || s.desiredPins()[0] != old {
		t.Fatal("generate must add a second payload key and keep the current one")
	}
	if s.currentPin() != httpsPin {
		t.Fatal("the HTTPS certificate must not change")
	}
	if len(s.payloadKeys()) != 2 {
		t.Fatal("during a rotation requests may be sealed to either key")
	}
	j := pinJob("a_1")
	if j == nil || !strings.Contains(string(j.Params), old) || !strings.Contains(string(j.Params), next) {
		t.Fatalf("agent not told to trust both keys: %+v", j)
	}
	if pinJob("a_2") == nil {
		t.Error("every agent must get the new key (all agents are assumed current)")
	}
	if code, body := c.do("POST", "/api/ui/identity/switch", `{}`); code != 409 || !strings.Contains(body, "web-01") || !strings.Contains(body, "old-vm") {
		t.Fatalf("switch with missing systems: %d %s", code, body)
	}
	s.st.mu.Lock()
	a.Pins = [2]string{old, next}
	j.Status = JobSuccess
	s.st.mu.Unlock()
	if code, _ := c.do("POST", "/api/ui/identity/switch", `{"force":true}`); code != 200 {
		t.Fatal("forced switch failed")
	}
	if s.desiredPins() != [2]string{next, ""} || len(s.payloadKeys()) != 1 {
		t.Fatalf("after switch: %v", s.desiredPins())
	}
	if k, err := readPayloadKey(s.payloadPath("current")); err != nil || k.Public().String() != next {
		t.Fatal("the new key must be stored as the current key")
	}
	if _, err := os.Stat(s.payloadPath("next")); err == nil {
		t.Fatal("the new-key file must be gone after the switch")
	}
	if j = pinJob("a_1"); j == nil || strings.Contains(string(j.Params), old) {
		t.Fatalf("agent not told to drop the old key: %+v", j)
	}
	if s.currentPin() != httpsPin {
		t.Fatal("the HTTPS certificate must not change on a payload key switch")
	}
	c.do("POST", "/api/ui/identity/generate", `{}`)
	if code, _ := c.do("POST", "/api/ui/identity/cancel", `{}`); code != 200 || s.desiredPins() != [2]string{next, ""} {
		t.Fatal("cancel must keep the current key")
	}
}

func TestReadOnlyCannotRotate(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	admin := newUIClient(t, srv.URL)
	admin.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	admin.do("POST", "/api/ui/users", `{"name":"ro","role":"viewer","password":"abcde"}`)
	ro := newUIClient(t, srv.URL)
	ro.do("POST", "/api/ui/login", `{"username":"ro","password":"abcde"}`)
	cur := s.desiredPins()[0]
	s.st.mu.Lock()
	s.st.st.Agents["a_1"] = &Agent{ID: "a_1", Name: "web-01", Status: StatusActive, Protocol: 5, Pins: [2]string{cur, "x"}, PinsAt: 1}
	s.st.mu.Unlock()
	fp, tlsPin := seal.Fingerprint(cur), s.currentPin()
	code, body := ro.do("GET", "/api/ui/identity", "")
	if code != 200 || !strings.Contains(body, `"slots":["current","unknown"]`) {
		t.Errorf("read-only users see the key status per system: %d %s", code, body)
	}
	if strings.Contains(body, fp) || strings.Contains(body, tlsPin) || strings.Contains(body, fp[:5]) {
		t.Errorf("read-only user received a fingerprint: %s", body)
	}
	if _, body := admin.do("GET", "/api/ui/identity", ""); !strings.Contains(body, fp) || !strings.Contains(body, tlsPin) {
		t.Errorf("administrators see the fingerprints: %s", body)
	}
	if code, _ := ro.do("POST", "/api/ui/identity/generate", `{}`); code != 403 {
		t.Errorf("read-only user generated a key: %d", code)
	}
}
