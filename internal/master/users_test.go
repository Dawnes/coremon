package master

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type uiClient struct {
	t   *testing.T
	hc  *http.Client
	url string
}

func (c *uiClient) do(method, path, body string) (int, string) {
	req, _ := http.NewRequest(method, c.url+path, strings.NewReader(body))
	req.Header.Set("X-Coremon", "1")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b := new(strings.Builder)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp.StatusCode, strings.TrimSpace(b.String())
}

func newUIClient(t *testing.T, url string) *uiClient {
	jar, _ := cookiejar.New(nil)
	return &uiClient{t: t, url: url, hc: &http.Client{Jar: jar, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}}
}

func TestUsersAndRoles(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.2.4")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))

	admin := newUIClient(t, srv.URL)
	if code, body := admin.do("POST", "/api/ui/login", `{"username":"admin","password":"nope"}`); code != 401 || body != "wrong user name or password" {
		t.Fatalf("wrong password: %d %q", code, body)
	}
	if code, body := admin.do("POST", "/api/ui/login", `{"username":"nobody","password":"x"}`); code != 401 || body != "wrong user name or password" {
		t.Fatalf("unknown user must look the same: %d %q", code, body)
	}
	if code, _ := admin.do("POST", "/api/ui/login", `{"username":"Admin","password":"`+strings.TrimSpace(string(pw))+`"}`); code != 200 {
		t.Fatalf("admin login: %d", code)
	}
	if code, body := admin.do("POST", "/api/ui/users", `{"name":"viewer1","role":"viewer","password":"abcde"}`); code != 200 {
		t.Fatalf("create viewer: %d %s", code, body)
	}

	ro := newUIClient(t, srv.URL)
	if code, _ := ro.do("POST", "/api/ui/login", `{"username":"viewer1","password":"abcde"}`); code != 200 {
		t.Fatal("viewer login failed")
	}
	_, sess := ro.do("GET", "/api/ui/session", "")
	var se struct {
		CanChange bool `json:"can_change"`
		User      struct{ Role string }
	}
	json.Unmarshal([]byte(sess), &se)
	if se.CanChange || se.User.Role != "viewer" {
		t.Fatalf("viewer session: %s", sess)
	}
	// Read-only: sees everything...
	for _, p := range []string{"/api/ui/overview", "/api/ui/settings", "/api/ui/audit", "/api/ui/releases", "/api/ui/jobs"} {
		if code, _ := ro.do("GET", p, ""); code != 200 {
			t.Errorf("viewer GET %s: %d", p, code)
		}
	}
	// ...but changes nothing and sees no enrollment tokens.
	for _, c := range [][2]string{
		{"GET", "/api/ui/tokens"}, {"POST", "/api/ui/tokens"}, {"POST", "/api/ui/settings"}, {"POST", "/api/ui/actions"},
		{"POST", "/api/ui/groups"}, {"POST", "/api/ui/layout"}, {"POST", "/api/ui/master/update"}, {"GET", "/api/ui/users"},
		{"POST", "/api/ui/users"}, {"POST", "/api/ui/releases/check"},
	} {
		if code, body := ro.do(c[0], c[1], "{}"); code != 403 || body != "your account is read-only" {
			t.Errorf("viewer %s %s: %d %q, want 403", c[0], c[1], code, body)
		}
	}
	// A viewer may change its own password.
	if code, body := ro.do("POST", "/api/ui/password", `{"current":"abcde","new":"fghij"}`); code != 200 {
		t.Errorf("viewer own password: %d %s", code, body)
	}

	// The last administrator cannot be removed or demoted.
	_, list := admin.do("GET", "/api/ui/users", "")
	var users []userView
	json.Unmarshal([]byte(list), &users)
	var adminID, viewerID string
	for _, u := range users {
		if u.Name == "admin" {
			adminID = u.ID
		} else {
			viewerID = u.ID
		}
	}
	if code, _ := admin.do("POST", "/api/ui/users/"+adminID, `{"role":"viewer"}`); code != 409 {
		t.Errorf("demoting the last admin: %d, want 409", code)
	}
	if code, _ := admin.do("POST", "/api/ui/users/"+adminID+"/delete", ""); code != 409 {
		t.Errorf("removing yourself: %d, want 409", code)
	}
	// Changing a role signs that user out.
	if code, _ := admin.do("POST", "/api/ui/users/"+viewerID, `{"role":"admin"}`); code != 200 {
		t.Fatal("promote viewer")
	}
	if code, _ := ro.do("GET", "/api/ui/overview", ""); code != 401 {
		t.Errorf("user whose role changed must be signed out, got %d", code)
	}
	// Audit entries carry user names and can be filtered by user.
	if got := s.audit.list("", "viewer1", 100); len(got) == 0 || got[0].Actor != "viewer1" {
		t.Errorf("audit for viewer1: %+v", got)
	}
}

func TestMigrateSingleAdminPassword(t *testing.T) {
	dir := t.TempDir()
	h := hashPassword("oldpass")
	os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"password_hash":"`+h+`"}`), 0o600)
	st, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.st.Users) != 1 || st.st.Users[0].Name != "admin" || st.st.Users[0].Role != RoleAdmin || !checkPassword(st.st.Users[0].PasswordHash, "oldpass") {
		t.Fatalf("migration: %+v", st.st.Users)
	}
	if st.st.PasswordHash != "" {
		t.Fatal("old password field must be cleared")
	}
}
