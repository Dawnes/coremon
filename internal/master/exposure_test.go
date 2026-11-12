package master

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"coremon/internal/proto"
	"coremon/internal/seal"
)

// Parallel wrong logins must not get past the limit of 5 per address.
func TestLoginLimitHoldsInParallel(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	hc := srv.Client()
	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[int]int{}
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest("POST", srv.URL+"/api/ui/login", strings.NewReader(`{"username":"admin","password":"wrong-guess"}`))
			req.Header.Set("X-Coremon", "1")
			resp, err := hc.Do(req)
			if err != nil {
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			mu.Lock()
			codes[resp.StatusCode]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if codes[401] != loginFails || codes[429] != 40-loginFails {
		t.Fatalf("40 parallel wrong logins: %v, want %d checked and the rest refused", codes, loginFails)
	}
	if n := len(s.audit.list("", "", 100)); n != loginFails {
		t.Errorf("%d audit entries, want %d", n, loginFails)
	}
	// A correct login gives its reserved attempt back.
	s2 := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	srv2 := httptest.NewTLSServer(s2.routes())
	defer srv2.Close()
	pw, _ := os.ReadFile(filepath.Join(s2.cfg.DataDir, "initial-password.txt"))
	for i := 0; i < 8; i++ {
		c := newUIClient(t, srv2.URL)
		if code, _ := c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`); code != 200 {
			t.Fatalf("correct login %d refused: %d", i+1, code)
		}
	}
	s2.sess.sweep()
	if len(s2.sess.fail) != 0 {
		t.Errorf("failed-login list not empty after only correct logins: %v", s2.sess.fail)
	}
}

// Large bodies to the agent endpoint are admitted a few at a time.
func TestIntakeLimits(t *testing.T) {
	var in intake
	if !in.try("1.1.1.1", true) || in.try("1.1.1.1", true) {
		t.Fatal("one large request per address")
	}
	if !in.try("2.2.2.2", true) || in.try("3.3.3.3", true) {
		t.Fatal("two large requests in total")
	}
	in.release("1.1.1.1", true)
	if !in.try("3.3.3.3", true) {
		t.Fatal("a released slot must be usable")
	}
	for i := 0; i < intakeSmallPerIP; i++ {
		if !in.try("9.9.9.9", false) {
			t.Fatalf("small request %d refused", i+1)
		}
	}
	if in.try("9.9.9.9", false) || !in.try("8.8.8.8", false) {
		t.Fatal("small requests are limited per address, not for others")
	}

	// Through the server: with the large slots taken, a large body gets 503
	// and is not read.
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.intake.largeTotal = 1
	s.intake.try("203.0.113.9", true) // someone else holds the only slot
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	big := bytes.Repeat([]byte("A"), intakeLargeFrom+10)
	s.intake.wait = 300 * time.Millisecond // normally 20 s
	if resp, err := srv.Client().Post(srv.URL+"/api/agent/v5", "application/json", bytes.NewReader(big)); err != nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("large body with no free slot: %v %v, want 503", err, resp)
	}
	// Small bodies still pass (and are refused for what they are).
	resp, err := srv.Client().Post(srv.URL+"/api/agent/v5", "application/json", strings.NewReader(`{}`))
	if err != nil || resp.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("small request while large slots are taken: %v %v", err, resp)
	}
	// Announced as too large: refused at once.
	req, _ := http.NewRequest("POST", srv.URL+"/api/agent/v5", bytes.NewReader(nil))
	req.ContentLength = maxSealedBody + 1
	req.Body = io.NopCloser(io.LimitReader(neverEnding('A'), maxSealedBody+1))
	if resp, err := srv.Client().Do(req); err == nil && resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized request: %d", resp.StatusCode)
	}
}

type neverEnding byte

func (b neverEnding) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(b)
	}
	return len(p), nil
}

// Registrations: wrong tokens are limited and summarised; with a valid
// token one system per address may wait for approval.
func TestRegistrationRules(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	_, k, _ := ed25519.GenerateKey(rand.Reader)
	key := seal.AgentKey(k.Public().(ed25519.PublicKey))
	enroll := func(ip, token, host string) error {
		_, err := s.enrollCore(proto.EnrollRequest{Token: token, Facts: proto.Facts{Hostname: host}}, ip, key)
		return err
	}
	code := func(err error) int {
		if ae, ok := err.(*apiError); ok {
			return ae.code
		}
		return 0
	}
	// Wrong token: 5 looked at, then refused; one audit line now, one summary later.
	for i := 0; i < 30; i++ {
		c := code(enroll("198.51.100.7", "nope", "evil\x1b[2J"))
		if (i < enrollFails && c != 403) || (i >= enrollFails && c != 429) {
			t.Fatalf("attempt %d: %d", i+1, c)
		}
	}
	e := s.audit.list("", "", 100)
	if len(e) != 1 || e[0].Action != "enroll.rejected" || e[0].Target != "evil[2J" {
		t.Fatalf("audit after 30 wrong attempts: %+v", e)
	}
	s.flushEnrolls(time.Now().Add(enrollWindow + time.Second))
	e = s.audit.list("", "", 100)
	if len(e) != 2 || !strings.Contains(e[0].Detail, "29 more rejected") {
		t.Fatalf("summary: %+v", e)
	}
	// Another address is not affected.
	if c := code(enroll("198.51.100.8", "nope", "x")); c != 403 {
		t.Fatalf("other address: %d", c)
	}

	// Valid tokens: one pending system per address.
	tok := func(v string) {
		s.st.mu.Lock()
		s.st.st.Tokens[sha256hex(v)] = &EnrollToken{Hash: sha256hex(v), Prefix: v, Expires: time.Now().Add(time.Hour).Unix()}
		s.st.mu.Unlock()
	}
	tok("tok-a")
	tok("tok-b")
	if err := enroll("203.0.113.5", "tok-a", "first"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ { // the waiting agent retries
		if c := code(enroll("203.0.113.5", "tok-b", "second")); c != 429 {
			t.Fatalf("second system while the first is pending: %d", c)
		}
	}
	s.st.mu.Lock()
	_, kept := s.st.st.Tokens[sha256hex("tok-b")]
	var first *Agent
	for _, a := range s.st.st.Agents {
		first = a
	}
	s.st.mu.Unlock()
	if !kept {
		t.Fatal("the waiting system's token was used up")
	}
	n := 0
	for _, x := range s.audit.list("", "", 100) {
		if x.Action == "enroll.waiting" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d 'waiting' audit lines for 3 retries, want 1", n)
	}
	// A different address may register meanwhile.
	tok("tok-c")
	if err := enroll("203.0.113.6", "tok-c", "elsewhere"); err != nil {
		t.Fatalf("other address: %v", err)
	}
	// After approval the second one gets in.
	s.st.mu.Lock()
	first.Status = StatusActive
	s.st.mu.Unlock()
	if err := enroll("203.0.113.5", "tok-b", "second"); err != nil {
		t.Fatalf("second system after approval: %v", err)
	}
}

// What a stranger can see.
func TestNothingForStrangers(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "9.8.7")
	apub, _, _ := ed25519.GenerateKey(rand.Reader)
	s.st.st.Agents["a_known"] = &Agent{ID: "a_known", AgentKey: seal.AgentKey(apub)}
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	get := func(p string) (int, string) {
		resp, err := srv.Client().Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	// No folder listings; files are still served.
	for _, p := range []string{"/legal/", "/legal"} {
		if code, body := get(p); code != 404 || strings.Contains(body, "LICENSE") {
			t.Errorf("%s: %d %q", p, code, body)
		}
	}
	for _, p := range []string{"/", "/app.js", "/style.css", "/legal/LICENSE.txt", "/favicon.svg"} {
		if code, _ := get(p); code != 200 {
			t.Errorf("%s: %d", p, code)
		}
	}
	// No version or date before signing in; both after.
	if _, body := get("/api/ui/session"); strings.Contains(body, "9.8.7") || strings.Contains(body, "version") || strings.Contains(body, "build_date") {
		t.Errorf("session tells a stranger the version: %s", body)
	}
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	if _, body := c.do("GET", "/api/ui/session", ""); !strings.Contains(body, `"version":"9.8.7"`) {
		t.Errorf("signed-in session has no version: %s", body)
	}
	// The same answer for an existing and a non-existing agent ID.
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	answer := func(id string) (int, string) {
		keyStr := s.desiredPins()[0]
		plain, _ := json.Marshal(map[string]any{"op": "poll", "body": map[string]any{}})
		req, _, err := seal.SealRequest(id, other, keyStr, []string{keyStr}, plain)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(req)
		resp, err := srv.Client().Post(srv.URL+"/api/agent/v5", "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	c1, b1 := answer("a_known")
	c2, b2 := answer("a_doesnotexist")
	if c1 != 401 || c1 != c2 || b1 != b2 {
		t.Errorf("existing agent: %d %q, unknown agent: %d %q", c1, b1, c2, b2)
	}
}
