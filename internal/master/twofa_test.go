package master

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTwoFA(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pwb, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	pw := strings.TrimSpace(string(pwb))
	login := func(name, pass string) (*uiClient, int, string) {
		c := newUIClient(t, srv.URL)
		code, body := c.do("POST", "/api/ui/login", `{"username":"`+name+`","password":"`+pass+`"}`)
		return c, code, body
	}
	admin, _, _ := login("admin", pw)
	other, _, _ := login("admin", pw) // a second browser
	admin.do("POST", "/api/ui/users", `{"name":"bob","role":"viewer","password":"secret1"}`)
	bob, _, _ := login("bob", "secret1")
	now := func() int64 { return time.Now().Unix() / 30 }

	// Turn on: QR code and key, then one code from the app.
	code, body := bob.do("POST", "/api/ui/account/2fa/start", `{}`)
	var st struct {
		Secret string   `json:"secret"`
		URI    string   `json:"uri"`
		QR     []string `json:"qr"`
	}
	json.Unmarshal([]byte(body), &st)
	if code != 200 || len(st.Secret) != 32 || !strings.Contains(st.URI, "Coremon:bob") || len(st.QR) < 21 {
		t.Fatalf("start: %d %s", code, body)
	}
	if code, _ := bob.do("POST", "/api/ui/account/2fa/enable", `{"code":"000000"}`); code != 401 {
		t.Fatalf("wrong code accepted when turning on: %d", code)
	}
	bobOther, _, _ := login("bob", "secret1")
	// Remember the step used: the 30-second window may roll over during the test.
	used := now()
	if code, body := bob.do("POST", "/api/ui/account/2fa/enable", `{"code":"`+totpCode(st.Secret, used)+`"}`); code != 200 {
		t.Fatalf("enable: %d %s", code, body)
	}
	if code, _ := bobOther.do("GET", "/api/ui/overview", ""); code != 401 {
		t.Error("other session survived turning 2FA on")
	}
	if code, _ := bob.do("GET", "/api/ui/overview", ""); code != 200 {
		t.Error("own session signed out")
	}
	if _, body := bob.do("GET", "/api/ui/session", ""); !strings.Contains(body, `"two_fa":true`) {
		t.Errorf("session: %s", body)
	}

	// Sign-in: no session after the password alone; the code is asked only
	// after a correct password.
	c2, code, body := login("bob", "secret1")
	var tk struct {
		CodeRequired bool   `json:"code_required"`
		Ticket       string `json:"ticket"`
	}
	json.Unmarshal([]byte(body), &tk)
	if code != 200 || !tk.CodeRequired || tk.Ticket == "" {
		t.Fatalf("password step: %d %s", code, body)
	}
	if code, _ := c2.do("GET", "/api/ui/overview", ""); code != 401 {
		t.Fatal("signed in without the code")
	}
	if _, code, body := login("bob", "wrong!"); code != 401 || strings.Contains(body, "ticket") {
		t.Fatalf("wrong password must not reach the code step: %d %s", code, body)
	}
	if code, _ := c2.do("POST", "/api/ui/login/code", `{"ticket":"`+tk.Ticket+`","code":"123456"}`); code != 401 {
		t.Fatal("wrong code accepted")
	}
	// The code used to turn 2FA on cannot be used again; the next one works.
	if code, _ := c2.do("POST", "/api/ui/login/code", `{"ticket":"`+tk.Ticket+`","code":"`+totpCode(st.Secret, used)+`"}`); code != 401 {
		t.Fatal("used code accepted again")
	}
	if code, body := c2.do("POST", "/api/ui/login/code", `{"ticket":"`+tk.Ticket+`","code":"`+totpCode(st.Secret, used+1)+`"}`); code != 200 {
		t.Fatalf("code step: %d %s", code, body)
	}
	if code, _ := c2.do("GET", "/api/ui/overview", ""); code != 200 {
		t.Fatal("not signed in after the code")
	}
	if code, _ := c2.do("POST", "/api/ui/login/code", `{"ticket":"`+tk.Ticket+`","code":"`+totpCode(st.Secret, used+1)+`"}`); code != 401 {
		t.Error("ticket reusable after sign-in")
	}

	// Per account: 10 wrong codes block the code step (from any address).
	for i := 0; i < 12; i++ {
		s.sess.sweep()
		s.sess.mu.Lock()
		s.sess.fail = map[string][]int64{} // other addresses: only the account limit counts
		s.sess.mu.Unlock()
		c, _, body := login("bob", "secret1")
		json.Unmarshal([]byte(body), &tk)
		code, _ := c.do("POST", "/api/ui/login/code", `{"ticket":"`+tk.Ticket+`","code":"999999"}`)
		if (i < 10 && code != 401) || (i >= 10 && code != 429) {
			t.Fatalf("wrong code %d: %d", i+1, code)
		}
	}
	found := false
	for _, e := range s.audit.list("", "", 200) {
		if e.Action == "login.code_blocked" {
			found = true
		}
	}
	if !found {
		t.Error("no audit entry for the blocked code step")
	}
	s.tfa.clearFails(func() string { s.st.mu.Lock(); defer s.st.mu.Unlock(); return s.st.userByName("bob").ID }())

	// Turning off yourself needs a code; administrators turn it off for others.
	if code, _ := bob.do("POST", "/api/ui/account/2fa/disable", `{"code":"111111"}`); code != 401 {
		t.Error("turned off without a valid code")
	}
	s.st.mu.Lock()
	bobID := s.st.userByName("bob").ID
	adminID := s.st.userByName("admin").ID
	s.st.mu.Unlock()
	if code, body := admin.do("POST", "/api/ui/users/"+bobID, `{"two_fa":true}`); code != 400 {
		t.Errorf("admin turned 2FA on for someone else: %d %s", code, body)
	}
	if code, body := admin.do("POST", "/api/ui/users/"+bobID, `{"two_fa":false}`); code != 200 || !strings.Contains(body, `"two_fa":false`) {
		t.Fatalf("admin turns off: %d %s", code, body)
	}
	if _, code, body := login("bob", "secret1"); code != 200 || strings.Contains(body, "code_required") {
		t.Errorf("after the admin turned it off: %d %s", code, body)
	}
	if code, _ := admin.do("POST", "/api/ui/users/"+adminID, `{"two_fa":false}`); code != 409 {
		t.Error("admin used the admin route for own 2FA")
	}
	// Own turn-off with a code.
	_, body = admin.do("POST", "/api/ui/account/2fa/start", `{}`)
	json.Unmarshal([]byte(body), &st)
	admin.do("POST", "/api/ui/account/2fa/enable", `{"code":"`+totpCode(st.Secret, now())+`"}`)
	if code, _ := other.do("GET", "/api/ui/overview", ""); code != 401 {
		t.Error("admin's other browser still signed in")
	}
	if code, body := admin.do("POST", "/api/ui/account/2fa/disable", `{"code":"`+totpCode(st.Secret, now()+1)+`"}`); code != 200 {
		t.Fatalf("own turn-off: %d %s", code, body)
	}

	// coremon-reset turns 2FA off for the account it resets.
	_, body = admin.do("POST", "/api/ui/account/2fa/start", `{}`)
	json.Unmarshal([]byte(body), &st)
	admin.do("POST", "/api/ui/account/2fa/enable", `{"code":"`+totpCode(st.Secret, now()+1)+`"}`)
	s.st.mu.Lock()
	on := s.st.userByName("admin").TOTPSecret != ""
	s.st.writeNow()
	s.st.mu.Unlock()
	if !on {
		t.Fatal("admin 2FA not on")
	}
	if _, err := ResetPassword(s.cfg.DataDir, "admin"); err != nil {
		t.Fatal(err)
	}
	stt, _ := openStore(s.cfg.DataDir)
	if u := stt.userByName("admin"); u.TOTPSecret != "" {
		t.Error("coremon-reset left 2FA on")
	}
}
