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

func TestParseCodes(t *testing.T) {
	for in, want := range map[string]string{"200-399": "200-399", " 200 , 204,301 - 302": "200,204,301-302", "404": "404"} {
		if _, err := ParseCodes(in); err != nil || CleanCodes(in) != want {
			t.Errorf("%q: %q %v", in, CleanCodes(in), err)
		}
	}
	for _, bad := range []string{"", "abc", "99", "600", "300-200", "200-", ",", "1-2-3"} {
		if _, err := ParseCodes(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := Service{Method: MethodHTTPS, Port: 443, Host: "www.example.com", Path: "/health?x=1", Codes: "200-399", CertValid: true, CertDays: 14}
	if err := Validate(ok); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]Service{
		"no host":  {Method: MethodHTTPS, Port: 443, Path: "/", Codes: "200"},
		"bad host": {Method: MethodHTTPS, Port: 443, Host: "a b.com", Path: "/", Codes: "200"},
		"bad path": {Method: MethodHTTPS, Port: 443, Host: "x.com", Path: "health", Codes: "200"},
		"space":    {Method: MethodHTTPS, Port: 443, Host: "x.com", Path: "/a b", Codes: "200"},
		"days":     {Method: MethodHTTPS, Port: 443, Host: "x.com", Path: "/", Codes: "200", CertDays: 400},
		"greeting": {Method: MethodHTTPS, Port: 443, Host: "x.com", Path: "/", Codes: "200", Greeting: true},
		"method":   {Method: "ftp", Port: 21},
		"port":     {Port: 0},
	} {
		if Validate(s) == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := Validate(Service{Method: MethodHTTPS, Port: 8006, Host: "10.0.0.5", Path: "/", Codes: "200"}); err != nil {
		t.Errorf("IP as host: %v", err)
	}
	if got := strings.Join(Steps(ok), ","); got != "connect,tls,cert,cert_days,http" {
		t.Errorf("steps %s", got)
	}
	if got := strings.Join(Steps(Service{Port: 22, Greeting: true}), ","); got != "connect,greeting" {
		t.Errorf("tcp steps %s", got)
	}
}

func TestCheckHTTPS(t *testing.T) {
	var hosts []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hosts = append(hosts, r.Host)
		switch {
		case r.URL.Path == "/nohead" && r.Method == http.MethodHead:
			w.WriteHeader(http.StatusMethodNotAllowed)
		case r.URL.Path == "/missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.Write([]byte("hello"))
		}
	}))
	defer srv.Close()
	host, ps, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(ps)
	run := func(s Service) Outcome {
		s.Method, s.Port = MethodHTTPS, port
		if s.Host == "" {
			s.Host = "example.com"
		}
		if s.Path == "" {
			s.Path = "/"
		}
		if s.Codes == "" {
			s.Codes = DefaultCodes
		}
		return Check(context.Background(), host, s, 3*time.Second)
	}
	if o := run(Service{}); o.Err != nil || !strings.HasPrefix(o.Detail, "HTTP 200 · certificate valid until") {
		t.Fatalf("plain: %+v", o)
	}
	if hosts[len(hosts)-1] != "example.com:"+ps {
		t.Errorf("Host header %q", hosts[len(hosts)-1])
	}
	// The test certificate is self-signed: not trusted by the system.
	if o := run(Service{CertValid: true}); o.Failed != StepCert || !strings.Contains(o.Err.Error(), "not trusted") {
		t.Errorf("cert: %+v", o)
	}
	if o := run(Service{CertDays: 30}); o.Err != nil {
		t.Errorf("days: %+v", o)
	}
	if o := run(Service{Path: "/nohead"}); o.Err != nil || !strings.HasPrefix(o.Detail, "HTTP 200 (GET)") {
		t.Errorf("GET fallback: %+v", o)
	}
	if o := run(Service{Path: "/missing"}); o.Failed != StepHTTP || o.Err.Error() != "HTTP 404, expected 200-399" {
		t.Errorf("404: %+v", o)
	}
	if o := run(Service{Path: "/missing", Codes: "200-399,404"}); o.Err != nil {
		t.Errorf("404 allowed: %+v", o)
	}
	srv.Close()
	if o := run(Service{}); o.Failed != StepConnect {
		t.Errorf("closed: %+v", o)
	}
}

func TestCertDays(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	leaf := srv.Certificate()
	srv.Close()
	if err := certDays(leaf, 14, leaf.NotAfter.Add(-9*24*time.Hour-time.Hour)); err == nil || err.Error() != "certificate expires in 9 days (needs 14)" {
		t.Errorf("9 days: %v", err)
	}
	if err := certDays(leaf, 14, leaf.NotAfter.Add(time.Hour)); err == nil || !strings.HasPrefix(err.Error(), "certificate expired on") {
		t.Errorf("expired: %v", err)
	}
	if err := certDays(leaf, 14, leaf.NotAfter.Add(-15*24*time.Hour)); err != nil {
		t.Errorf("15 days: %v", err)
	}
}

func TestCheckTCPSteps(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			time.AfterFunc(2*time.Second, func() { c.Close() })
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	if o := Check(context.Background(), "127.0.0.1", Service{Port: port}, time.Second); o.Err != nil {
		t.Errorf("connect: %+v", o)
	}
	if o := Check(context.Background(), "127.0.0.1", Service{Port: port, Greeting: true}, 300*time.Millisecond); o.Failed != StepGreeting {
		t.Errorf("greeting: %+v", o)
	}
}
