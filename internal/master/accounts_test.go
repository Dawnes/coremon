package master

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"coremon/internal/proto"
)

func TestStateFileObfuscated(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.st.mu.Lock()
	s.st.st.Agents["a_1"] = &Agent{ID: "a_1", Name: "secret-web-01", Facts: proto.Facts{IPs: []string{"192.168.77.12"}}}
	s.st.writeNow()
	s.st.mu.Unlock()
	b, err := os.ReadFile(filepath.Join(s.cfg.DataDir, stateFile))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("secret-web-01")) || bytes.Contains(b, []byte("192.168.77.12")) || bytes.Contains(b, []byte("admin")) || bytes.Contains(b, []byte("\x1f\x8b")) {
		t.Fatal("state file is readable or plain gzip")
	}

	out, err := ShowState(s.cfg.DataDir)
	if err != nil || !bytes.Contains(out, []byte(`"name": "secret-web-01"`)) {
		t.Fatalf("show-state: %v %s", err, out)
	}
	// Damaged files are refused, not misread.
	b[len(b)-5] ^= 0xff
	if _, err := decodeState(b); err == nil {
		t.Fatal("damaged state accepted")
	}
}

func TestForcedChangeAndPermanentAdmin(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pwb, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	pw := strings.TrimSpace(string(pwb))
	s.st.mu.Lock()
	if !s.st.userByName("admin").MustChange && s.st.st.InitialUser == "" {
		t.Fatal("no initial user")
	}
	s.st.userByName("admin").MustChange = true // as after installation (testServer clears it)
	s.st.mu.Unlock()
	c := newUIClient(t, srv.URL)
	if code, _ := c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+pw+`"}`); code != 200 {
		t.Fatal("login")
	}
	// Generated password: everything but the password change is refused.
	if _, body := c.do("GET", "/api/ui/session", ""); !strings.Contains(body, `"must_change":true`) {
		t.Fatalf("session: %s", body)
	}
	if code, body := c.do("GET", "/api/ui/overview", ""); code != 403 || !strings.Contains(body, "password change required") {
		t.Fatalf("overview before the change: %d %s", code, body)
	}
	if code, _ := c.do("POST", "/api/ui/password", `{"current":"`+pw+`","new":"`+pw+`"}`); code != 400 {
		t.Fatal("keeping the generated password accepted")
	}
	if code, body := c.do("POST", "/api/ui/password", `{"current":"`+pw+`","new":"mine-12345"}`); code != 200 {
		t.Fatalf("change: %d %s", code, body)
	}
	if code, _ := c.do("GET", "/api/ui/overview", ""); code != 200 {
		t.Fatal("overview after the change")
	}

	// The admin account is permanent.
	c.do("POST", "/api/ui/users", `{"name":"boss","role":"admin","password":"abcde"}`)
	c2 := newUIClient(t, srv.URL)
	c2.do("POST", "/api/ui/login", `{"username":"boss","password":"abcde"}`)
	var adminID string
	s.st.mu.Lock()
	adminID = s.st.userByName("admin").ID
	s.st.mu.Unlock()
	if code, _ := c2.do("POST", "/api/ui/users/"+adminID+"/delete", `{}`); code != 409 {
		t.Error("admin account removed")
	}
	if code, _ := c2.do("POST", "/api/ui/users/"+adminID, `{"role":"viewer"}`); code != 409 {
		t.Error("admin account demoted")
	}

	// coremon-reset: existing users only; the new password must be changed.
	s.st.mu.Lock()
	s.st.writeNow()
	s.st.mu.Unlock()
	if _, err := ResetPassword(s.cfg.DataDir, "nobody"); err == nil || !strings.Contains(err.Error(), "admin") {
		t.Fatalf("reset of a missing user: %v (should list the administrators)", err)
	}
	newPW, err := ResetPassword(s.cfg.DataDir, "admin")
	if err != nil || len(newPW) != 16 {
		t.Fatalf("reset: %v %q", err, newPW)
	}
	st, _ := openStore(s.cfg.DataDir)
	if u := st.userByName("admin"); u == nil || !u.MustChange || !checkPassword(u.PasswordHash, newPW) {
		t.Fatal("reset not stored as a generated password")
	}
	al := &auditLog{path: filepath.Join(s.cfg.DataDir, "audit.jsonl")}
	if e := al.list("", "", 1); len(e) == 0 || e[0].Action != "password.reset" || strings.Contains(e[0].Detail, newPW) {
		t.Fatalf("reset audit: %+v", e)
	}

	// A missing admin account is restored at start-up, with a forced change.
	st.mu.Lock()
	st.st.Users = []*User{st.userByName("boss")}
	st.writeNow()
	st.mu.Unlock()
	os.Remove(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	s2, err := New(Config{DataDir: s.cfg.DataDir, SigningKeyPath: s.cfg.SigningKeyPath, Version: "0.3.0", IndexURL: "http://127.0.0.1:1/none"})
	if err != nil {
		t.Fatal(err)
	}
	u := s2.st.userByName("admin")
	restoredPW, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	if u == nil || u.Role != RoleAdmin || !u.MustChange || !checkPassword(u.PasswordHash, strings.TrimSpace(string(restoredPW))) {
		t.Fatalf("admin not restored: %+v", u)
	}
}
