package master

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// servedPin connects and returns the fingerprint of the certificate served.
func servedPin(t *testing.T, addr string) string {
	t.Helper()
	c, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	sum := sha256.Sum256(c.ConnectionState().PeerCertificates[0].RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(sum[:])
}

func TestCertRotation(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.st.st.Settings.MasterURL = "https://coremon.lan:8443"
	// A real server with the master's own TLS setup (httptest would put in
	// its own certificate).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hs := &http.Server{Handler: s.routes(), TLSConfig: &tls.Config{GetCertificate: s.currentCert}}
	go hs.ServeTLS(ln, "", "")
	defer hs.Close()
	addr := ln.Addr().String()
	srv := struct{ URL string }{"https://" + addr}
	old := s.currentPin()
	if servedPin(t, addr) != old {
		t.Fatal("not serving the current certificate")
	}

	// Through the website, as an administrator.
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)
	if code, body := c.do("POST", "/api/ui/identity/cert", `{}`); code != 200 {
		t.Fatalf("rotate: %d %s", code, body)
	}
	pin := s.currentPin()
	if pin == old || servedPin(t, addr) != pin {
		t.Fatal("the new certificate is not served right away")
	}
	from, until := certDates(s.keys.cert)
	if d := until.Sub(from); d < 364*24*time.Hour || d > 367*24*time.Hour {
		t.Fatalf("validity %s, want 1 year", d)
	}
	leaf := s.keys.cert.Leaf
	if leaf == nil {
		leaf, _ = parseLeaf(s.keys.cert)
	}
	if !contains(leaf.DNSNames, "coremon.lan") || !contains(leaf.DNSNames, "localhost") {
		t.Fatalf("names: %v", leaf.DNSNames)
	}
	// Stored: a restarted master uses the new one.
	if _, p, err := loadOrCreateCert(s.cfg.DataDir, nil); err != nil || p != pin {
		t.Fatalf("stored certificate: %v", err)
	}
	if e := s.audit.list("", "", 5); len(e) == 0 || e[0].Action != "identity.cert" || !strings.Contains(e[0].Detail, shortPin(pin)) {
		t.Fatalf("audit: %+v", e)
	}
	// Read-only users cannot.
	c.do("POST", "/api/ui/users", `{"name":"ro","role":"viewer","password":"abcde"}`)
	ro := newUIClient(t, srv.URL)
	ro.do("POST", "/api/ui/login", `{"username":"ro","password":"abcde"}`)
	if code, _ := ro.do("POST", "/api/ui/identity/cert", `{}`); code != 403 {
		t.Fatalf("read-only user replaced the certificate: %d", code)
	}

	// Automatic renewal: not yet, then within 30 days of the end.
	s.renewCertIfDue(time.Now())
	if s.currentPin() != pin {
		t.Fatal("renewed too early")
	}
	s.renewCertIfDue(until.Add(-11 * 24 * time.Hour))
	if s.currentPin() != pin {
		t.Fatal("renewed 11 days before expiry (should be 10)")
	}
	s.renewCertIfDue(until.Add(-9 * 24 * time.Hour))
	if s.currentPin() == pin {
		t.Fatal("not renewed 9 days before expiry")
	}
	for host, want := range map[string]string{"web-01": "coremon web-01", "coremon": "coremon", "Coremon": "coremon", "coremon-master": "coremon coremon-master", "": "coremon"} {
		if got := certCN(host); got != want {
			t.Errorf("certCN(%q) = %q, want %q", host, got, want)
		}
	}
	if e := s.audit.list("", "", 1); e[0].Actor != "scheduler" {
		t.Fatalf("renewal audit: %+v", e[0])
	}
}

// A browser resumes its TLS session (session ticket) and reuses open
// connections; neither may keep it on the old certificate.
func TestCertRotationReachesBrowsers(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	hs := s.newHTTPSServer(s.routes(), "")
	go hs.ServeTLS(ln, "", "")
	defer hs.Close()
	url := "https://" + ln.Addr().String() + "/"
	// Like a browser: a TLS session cache and keep-alive connections.
	browser := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true,
		ClientSessionCache: tls.NewLRUClientSessionCache(8)}}}
	pinOf := func(st *tls.ConnectionState) string {
		sum := sha256.Sum256(st.PeerCertificates[0].RawSubjectPublicKeyInfo)
		return base64.StdEncoding.EncodeToString(sum[:])
	}
	get := func() *tls.ConnectionState {
		t.Helper()
		resp, err := browser.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.TLS
	}
	old := s.currentPin()
	if pinOf(get()) != old {
		t.Fatal("not serving the current certificate")
	}
	// A second connection resumes the session (as browsers do).
	browser.Transport.(*http.Transport).CloseIdleConnections()
	if st := get(); !st.DidResume {
		t.Log("note: no session resumption with this Go version")
	}
	if err := s.rotateCert("test", ""); err != nil {
		t.Fatal(err)
	}
	// Without closing anything on the client side: the open connection and
	// the session ticket must both lead to the new certificate.
	time.Sleep(100 * time.Millisecond)
	st := get()
	if pinOf(st) != s.currentPin() || pinOf(st) == old {
		t.Fatalf("after a new certificate the browser still gets the old one (resumed: %v)", st.DidResume)
	}
}

// An interrupted rotation (key renamed, certificate not yet) is completed at
// the next start instead of leaving a mismatched pair.
func TestCertRecovery(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := loadOrCreateCert(dir, nil); err != nil {
		t.Fatal(err)
	}
	crt, key := certPaths(dir, "current")
	if err := createCert(crt+".new", key+".new", nil); err != nil {
		t.Fatal(err)
	}
	_, want, _ := loadCert(crt+".new", key+".new")
	os.Rename(key+".new", key) // crash here
	if _, pin, err := loadOrCreateCert(dir, nil); err != nil || pin != want {
		t.Fatalf("after recovery: %v (pin match %v)", err, pin == want)
	}
	if _, err := os.Stat(crt + ".new"); !os.IsNotExist(err) {
		t.Fatal("leftover file")
	}
}
