package master

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserLock(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	login := func(name, pass string) (*uiClient, int, string) {
		c := newUIClient(t, srv.URL)
		code, body := c.do("POST", "/api/ui/login", `{"username":"`+name+`","password":"`+pass+`"}`)
		return c, code, body
	}
	admin, code, _ := login("admin", strings.TrimSpace(string(pw)))
	if code != 200 {
		t.Fatalf("admin login: %d", code)
	}
	ids := map[string]string{}
	for _, u := range [][2]string{{"bob", "viewer"}, {"dave", "admin"}} {
		code, body := admin.do("POST", "/api/ui/users", `{"name":"`+u[0]+`","role":"`+u[1]+`","password":"secret1"}`)
		if code != 200 {
			t.Fatalf("create %s: %d %s", u[0], code, body)
		}
		var v userView
		json.Unmarshal([]byte(body), &v)
		ids[u[0]] = v.ID
	}
	bob, _, _ := login("bob", "secret1")
	if code, _ := bob.do("GET", "/api/ui/overview", ""); code != 200 {
		t.Fatal("bob cannot work before the lock")
	}
	// Lock: signed out at once; sign-in says so only with the right password.
	if code, body := admin.do("POST", "/api/ui/users/"+ids["bob"], `{"locked":true}`); code != 200 || !strings.Contains(body, `"locked":true`) {
		t.Fatalf("lock bob: %d %s", code, body)
	}
	if code, _ := bob.do("GET", "/api/ui/overview", ""); code != 401 {
		t.Errorf("bob's session still works after the lock: %d", code)
	}
	if _, code, body := login("bob", "secret1"); code != 403 || body != "Your account is locked" {
		t.Errorf("locked sign-in: %d %q", code, body)
	}
	if _, code, body := login("bob", "wrong!"); code != 401 || body != "wrong user name or password" {
		t.Errorf("locked account, wrong password: %d %q", code, body)
	}
	if _, body := admin.do("GET", "/api/ui/users", ""); !strings.Contains(body, `"name":"bob","role":"viewer"`) || !strings.Contains(body, `"locked":true`) {
		t.Errorf("users list: %s", body)
	}
	found := false
	for _, e := range s.audit.list("", "", 100) {
		if e.Action == "user.locked" && e.Target == "bob" {
			found = true
		}
	}
	if !found {
		t.Error("no audit entry for the lock")
	}
	// Not on yourself; not together with other changes.
	adminID := ""
	s.st.mu.Lock()
	adminID = s.st.userByName("admin").ID
	s.st.mu.Unlock()
	if code, body := admin.do("POST", "/api/ui/users/"+adminID, `{"locked":true}`); code != 409 {
		t.Errorf("lock yourself: %d %s", code, body)
	}
	if code, _ := admin.do("POST", "/api/ui/users/"+ids["bob"], `{"locked":false,"role":"admin"}`); code != 400 {
		t.Errorf("lock combined with a role change: %d", code)
	}
	// Another administrator may lock the permanent admin; then dave is the
	// last administrator who can sign in and cannot demote himself.
	dave, _, _ := login("dave", "secret1")
	if code, body := dave.do("POST", "/api/ui/users/"+adminID, `{"locked":true}`); code != 200 {
		t.Fatalf("dave locks admin: %d %s", code, body)
	}
	if code, _ := admin.do("GET", "/api/ui/overview", ""); code != 401 {
		t.Error("admin's session survived the lock")
	}
	if code, body := dave.do("POST", "/api/ui/users/"+ids["dave"], `{"role":"viewer"}`); code != 409 || !strings.Contains(body, "last administrator") {
		t.Errorf("last active admin demotes himself: %d %s", code, body)
	}
	if code, _ := dave.do("POST", "/api/ui/users/"+ids["dave"]+"/delete", ""); code != 409 {
		t.Errorf("last active admin removed: %d", code)
	}
	// Unlock: works again; a locked account stays locked when its password
	// is reset by an administrator.
	dave.do("POST", "/api/ui/users/"+ids["bob"], `{"locked":false}`)
	if _, code, _ := login("bob", "secret1"); code != 200 {
		t.Errorf("bob after unlock: %d", code)
	}
	dave.do("POST", "/api/ui/users/"+ids["bob"], `{"locked":true}`)
	dave.do("POST", "/api/ui/users/"+ids["bob"], `{"password":"secret2"}`)
	if _, code, _ := login("bob", "secret2"); code != 403 {
		t.Errorf("password reset unlocked the account: %d", code)
	}
	// coremon-reset unlocks the account it resets.
	s.st.mu.Lock()
	if err := s.st.writeNow(); err != nil {
		t.Fatal(err)
	}
	s.st.mu.Unlock()
	if _, err := ResetPassword(s.cfg.DataDir, "admin"); err != nil {
		t.Fatal(err)
	}
	st, err := openStore(s.cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if u := st.userByName("admin"); u.Locked || !u.MustChange {
		t.Errorf("after coremon-reset: locked %v, must change %v", u.Locked, u.MustChange)
	}
}
