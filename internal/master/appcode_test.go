package master

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The application code is only for signed-in users; the sign-in page is public.
func TestAppCodeSignedInOnly(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	c := newUIClient(t, srv.URL)

	name := s.appName()
	if !regexp.MustCompile(`^app-[0-9a-f]{12}\.js$`).MatchString(name) {
		t.Fatalf("app name %q", name)
	}
	if code, body := c.do("GET", "/", ""); code != 200 || !strings.Contains(body, "login.js") || strings.Contains(body, "app.js") {
		t.Fatalf("index: %d %s", code, body)
	}
	if code, body := c.do("GET", "/login.js", ""); code != 200 || strings.Contains(body, "/api/ui/users") || strings.Contains(body, name) {
		t.Errorf("login.js: %d, must not hold application code or its name", code)
	}
	for _, p := range []string{"/app.js", "/" + name} {
		if code, _ := c.do("GET", p, ""); code != 404 {
			t.Errorf("%s before sign-in: %d", p, code)
		}
	}
	_, body := c.do("GET", "/api/ui/session", "")
	if strings.Contains(body, name) || strings.Contains(body, `"app"`) {
		t.Errorf("session tells a stranger the app name: %s", body)
	}

	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	if code, _ := c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`); code != 200 {
		t.Fatal("login")
	}
	// Also with the forced password change still open.
	s.st.mu.Lock()
	s.st.userByName("admin").MustChange = true
	s.st.mu.Unlock()
	_, body = c.do("GET", "/api/ui/session", "")
	var sess struct {
		App        string `json:"app"`
		MustChange bool   `json:"must_change"`
	}
	json.Unmarshal([]byte(body), &sess)
	if sess.App != name || !sess.MustChange {
		t.Fatalf("session after sign-in: %s", body)
	}
	code, js := c.do("GET", "/"+name, "")
	if code != 200 || !strings.Contains(js, "function AppList") {
		t.Fatalf("app code after sign-in: %d", code)
	}
	if resp, err := c.hc.Get(srv.URL + "/" + name); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
		if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
			t.Errorf("app code may be kept by the browser: %q", cc)
		}
	}
	if code, _ := c.do("GET", "/app.js", ""); code != 404 {
		t.Errorf("/app.js after sign-in: %d", code)
	}

	c.do("POST", "/api/ui/logout", "")
	if code, _ := c.do("GET", "/"+name, ""); code != 404 {
		t.Errorf("app code after sign-out: %d", code)
	}
}
