package probe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCertCheck(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	host, p, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(p)
	ctx := context.Background()
	base := Service{Method: MethodCert, Port: port, Host: "example.com"}

	// The test server's certificate is self-signed: the chain fails.
	s := base
	s.CertValid, s.CertTime, s.CertDays = true, true, CertCheckDays
	if err := Validate(s); err != nil {
		t.Fatal(err)
	}
	if st := Steps(s); strings.Join(st, ",") != "connect,tls,cert,cert_time,cert_days" {
		t.Errorf("steps %v", st)
	}
	o := Check(ctx, host, s, 3*time.Second)
	if o.Err == nil || o.Failed != StepCert || !strings.Contains(o.Err.Error(), "not trusted") {
		t.Errorf("self-signed: %+v", o)
	}
	if o.Cert == nil || len(o.Cert.Chain) == 0 || o.Cert.TLS == "" || !strings.Contains(o.Cert.Chain[0].SHA256, ":") {
		t.Fatalf("certificate details missing on a failed check: %+v", o.Cert)
	}
	names := strings.Join(o.Cert.Chain[0].Names, ",")
	if !strings.Contains(names, "example.com") || !strings.Contains(names, "127.0.0.1") {
		t.Errorf("names: %s", names)
	}

	// Chain not required: valid in time passes (the test certificate runs for years).
	s.CertValid = false
	if o := Check(ctx, host, s, 3*time.Second); o.Err != nil || !strings.HasPrefix(o.Detail, "valid until") {
		t.Errorf("time only: %+v", o)
	}
	// Both off: a handshake with a certificate is enough.
	s.CertTime, s.CertDays = false, 0
	if st := Steps(s); len(st) != 2 {
		t.Errorf("steps without checks: %v", st)
	}
	if o := Check(ctx, host, s, 3*time.Second); o.Err != nil {
		t.Errorf("no checks: %+v", o)
	}
	// Nothing listening: the connect step fails.
	s.Port = 1
	if o := Check(ctx, host, s, 2*time.Second); o.Failed != StepConnect {
		t.Errorf("closed port: %+v", o)
	}
}

func TestParseTarget(t *testing.T) {
	for in, want := range map[string]string{
		"example.com":                       "example.com:443",
		" WWW.Example.com:8443 ":            "www.example.com:8443",
		"https://example.com/some/path?x=1": "example.com:443",
		"https://example.com:8006/":         "example.com:8006",
		"192.0.2.7:636":                     "192.0.2.7:636",
		"[2001:db8::1]:8443":                "2001:db8::1:8443",
	} {
		h, p, err := ParseTarget(in)
		if err != nil || h+":"+strconv.Itoa(p) != want {
			t.Errorf("%q: %s %d %v, want %s", in, h, p, err, want)
		}
	}
	for _, bad := range []string{"", "http://example.com", "example.com:0", "exa mple.com", "example.com:99999"} {
		if _, _, err := ParseTarget(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
