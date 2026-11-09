package master

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"coremon/internal/proto"
)

// rawGet sends a request with an arbitrary Host header, as an attacker could.
func rawGet(t *testing.T, srvURL, path, host string) (int, string) {
	t.Helper()
	conn, err := tls.Dial("tcp", strings.TrimPrefix(srvURL, "https://"), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", path, host)
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestInstallScriptHostInjection(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.2.7")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()

	for _, evil := range []string{"x$(reboot)", "x`id`", "a;b", "x'y", "x&y"} {
		code, body := rawGet(t, srv.URL, "/install.sh", evil)
		if code == 200 || strings.Contains(body, "MASTER=") {
			t.Errorf("Host %q must be refused, got %d: %.80s", evil, code, body)
		}
	}
	code, body := rawGet(t, srv.URL, "/install.sh", "192.168.5.108:8443")
	if code != 200 || !strings.Contains(body, "MASTER='https://192.168.5.108:") {
		t.Fatalf("normal host: %d %.200s", code, body)
	}
	code, body = rawGet(t, srv.URL, "/install.sh", "coremon.lan")
	if code != 200 || !strings.Contains(body, "MASTER='https://coremon.lan:") {
		t.Fatalf("DNS name: %d", code)
	}

	// The install command shown in the website is refused the same way.
	req := httptest.NewRequest("POST", "https://x/api/ui/tokens", strings.NewReader(`{}`))
	req.Host = "x$(reboot)"
	w := httptest.NewRecorder()
	s.handleTokenCreate(w, req)
	if w.Code != 400 || len(s.st.st.Tokens) != 0 {
		t.Errorf("token with a crafted host: %d, %d token(s) created", w.Code, len(s.st.st.Tokens))
	}
}

func TestSafeHost(t *testing.T) {
	cases := map[string]string{
		"192.168.5.108": "192.168.5.108", "192.168.5.108:443": "192.168.5.108", "[fd00::1]:8443": "fd00::1",
		"coremon": "coremon", "master.lan:8443": "master.lan",
		"x$(reboot)": "", "a b": "", "-x": "", "x'": "", "": "",
	}
	for in, want := range cases {
		if got := safeHost(in); got != want {
			t.Errorf("safeHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClientIPBehindLocalProxy(t *testing.T) {
	var got string
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = clientIP(r) })
	req := func(remote, xff string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = remote
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return r
	}
	viaLocalProxy(h).ServeHTTP(httptest.NewRecorder(), req("127.0.0.1:5000", "10.0.0.7"))
	if got != "10.0.0.7" {
		t.Errorf("behind the local proxy: %q, want the forwarded address", got)
	}
	viaLocalProxy(h).ServeHTTP(httptest.NewRecorder(), req("127.0.0.1:5000", "1.2.3.4, 10.0.0.8"))
	if got != "10.0.0.8" {
		t.Errorf("forged first entry must be ignored: %q", got)
	}
	// On the HTTPS listeners X-Forwarded-For is never trusted.
	h.ServeHTTP(httptest.NewRecorder(), req("192.168.5.20:5000", "10.0.0.7"))
	if got != "192.168.5.20" {
		t.Errorf("direct request: %q, want the real address", got)
	}
	viaLocalProxy(h).ServeHTTP(httptest.NewRecorder(), req("192.168.5.20:5000", "10.0.0.7"))
	if got != "192.168.5.20" {
		t.Errorf("non-loopback sender must not set the address: %q", got)
	}
}

func TestAgentReportLimits(t *testing.T) {
	ips := make([]string, 500)
	for i := range ips {
		ips[i] = "10.0.0.1"
	}
	f := sanitizeFacts(proto.Facts{Hostname: strings.Repeat("h", 5000), IPs: ips})
	if len(f.Hostname) != maxFactLen || len(f.IPs) != maxIPs {
		t.Errorf("facts not limited: %d chars, %d IPs", len(f.Hostname), len(f.IPs))
	}
	n := sanitizeFacts(proto.Facts{Gateway: "192.168.5.1", GatewayIf: strings.Repeat("e", 100), DNSVia: "<b>x</b>",
		DNS: append([]string{"1.1.1.1", "2001:db8::1", "<script>", "::ffff:1.2.3.4"}, strings.Split(strings.Repeat("9.9.9.9,", 20), ",")...)})
	if n.Gateway != "192.168.5.1" || len(n.GatewayIf) != 32 || len(n.DNS) != maxDNS || n.DNS[0] != "1.1.1.1" || n.DNS[1] != "9.9.9.9" || n.DNSVia != "" {
		t.Errorf("network facts not limited: %+v", n)
	}
	if g := sanitizeFacts(proto.Facts{Gateway: "not-an-ip", GatewayIf: "eth0"}); g.Gateway != "" || g.GatewayIf != "" {
		t.Errorf("bad gateway kept: %+v", g)
	}
	feats := make([]proto.FeatureInfo, 100)
	feats[0].Actions = make([]proto.ActionSpec, 500)
	feats = sanitizeFeatures(feats)
	if len(feats) != maxFeatures || len(feats[0].Actions) != maxActions {
		t.Errorf("features not limited: %d / %d", len(feats), len(feats[0].Actions))
	}

	// A request larger than the limit is refused outright.
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.2.7")
	big := `{"agent":"` + strings.Repeat("x", maxSealedBody) + `"}`
	w := httptest.NewRecorder()
	s.handleSealed(w, httptest.NewRequest("POST", "/api/agent/v5", strings.NewReader(big)))
	if w.Code != 400 {
		t.Errorf("oversized request: %d, want 400", w.Code)
	}
}

func TestLoginNameLengthInAudit(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.2.7")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"`+strings.Repeat("a", 3000)+`","password":"x"}`)
	e := s.audit.list("", "", 5)
	if len(e) == 0 || len(e[0].Actor) > 32 {
		t.Errorf("failed login logged a %d-character name", len(e[0].Actor))
	}
	os.Remove(filepath.Join(s.cfg.DataDir, "audit.jsonl"))
}
