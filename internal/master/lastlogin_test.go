package master

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLastLogin(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	login := `{"username":"admin","password":"` + strings.TrimSpace(string(pw)) + `"}`

	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"wrong"}`)
	s.st.mu.Lock()
	if u := s.st.userByName("admin"); u.LastLogin != 0 {
		t.Fatal("a failed sign-in counted")
	}
	s.st.mu.Unlock()

	c.do("POST", "/api/ui/login", login)
	_, body := c.do("GET", "/api/ui/session", "")
	var sess struct {
		PrevLogin int64    `json:"prev_login"`
		User      userView `json:"user"`
	}
	json.Unmarshal([]byte(body), &sess)
	if sess.User.LastLogin == 0 || sess.User.LastIP != "127.0.0.1" || sess.PrevLogin != 0 {
		t.Fatalf("first sign-in: %s", body)
	}
	first := sess.User.LastLogin

	c2 := newUIClient(t, srv.URL)
	c2.do("POST", "/api/ui/login", login)
	_, body = c2.do("GET", "/api/ui/session", "")
	sess.PrevLogin = 0
	json.Unmarshal([]byte(body), &sess)
	if sess.PrevLogin != first || !strings.Contains(body, `"prev_ip":"127.0.0.1"`) {
		t.Fatalf("second sign-in must show the first as previous: %s", body)
	}

	// Users who signed in before this existed: filled once from the audit log.
	s.st.mu.Lock()
	u := s.st.userByName("admin")
	u.LastLogin, u.LastIP, u.PrevLogin, u.PrevIP = 0, "", 0, ""
	s.st.st.LoginsFilled = false
	s.st.mu.Unlock()
	s.audit.add(auditEntry{Actor: "admin", IP: "10.0.0.9", Action: "login"})
	s.fillLoginsFromAudit()
	s.st.mu.Lock()
	got := *u
	s.st.mu.Unlock()
	if got.LastIP != "10.0.0.9" || got.PrevIP != "127.0.0.1" || got.LastLogin == 0 {
		t.Errorf("filled from the audit log: last %s, previous %s", got.LastIP, got.PrevIP)
	}
}
