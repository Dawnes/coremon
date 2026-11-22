package probe

// Service checks. A service has a check method; each method runs its
// sub-probes in order and stops at the first one that fails, so a failed
// check always names what failed.
//   - tcp:   connect, then (optional) wait for the greeting line
//   - https: connect, TLS handshake (SNI = hostname), (optional) certificate
//            valid, (optional) certificate valid for at least N days, HTTP
//            status (HEAD, and GET when HEAD fails)
//   - dns:   query the DNS server on the device (UDP, TCP when truncated),
//            server status, records of the asked type, (optional) expected
//            value, (optional) response time
//   - cert:  connect, TLS handshake, (optional) valid chain: trusted and
//            issued for the name, (optional) valid in time, (optional) not
//            expiring within N days; no HTTP request
// The hostname is used for SNI and the Host header only: the connection
// always goes to the device's own address, nothing is looked up in DNS
// (a DNS check asks the device's own DNS server). The one exception: a
// cert check with Remote connects to wherever the name points.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"coremon/internal/proto"
)

const (
	MethodTCP   = "tcp"
	MethodHTTPS = "https"
	MethodDNS   = "dns"
	MethodCert  = "cert"

	StepCertTime = "cert_time" // cert: valid at this time (not expired, not before its start)
	StepConnect  = "connect"
	StepGreeting = "greeting"
	StepTLS      = "tls"
	StepCert     = "cert"
	StepCertDays = "cert_days"
	StepHTTP     = "http"
	StepQuery    = "query"   // dns: an answer came back
	StepRcode    = "rcode"   // dns: the server answered "no error"
	StepRecords  = "records" // dns: at least one record of the asked type
	StepExpect   = "expect"  // dns: an answer contains the expected value
	StepTime     = "time"    // dns: answered within the maximum time

	DefaultCodes    = "200-399"
	DefaultCertDays = 14
	CertCheckDays   = 10 // cert: down when it expires within this many days
	MaxCertDays     = 365
	DefaultDNSType  = "A"
	DefaultMaxMs    = 3000
	MaxMaxMs        = 60000
	maxCodeRanges   = 20
	maxPath         = 200
	maxExpect       = 200
)

// DNSTypes: the record types a DNS check can ask for.
var DNSTypes = []string{"A", "AAAA", "MX", "TXT", "CNAME", "NS"}

// Service is what a service check needs.
type Service struct {
	Method    string // "" means tcp
	Port      int
	Greeting  bool
	Host      string // https: the hostname; dns: the name to resolve
	Path      string
	Codes     string
	CertValid bool
	CertDays  int    // 0: not checked
	RType     string // dns: record type
	Expect    string // dns: an answer must contain this ("" = not checked)
	MaxMs     int    // dns: maximum response time in ms (0 = not checked)
	CertTime  bool   // cert: valid at this time
	Remote    bool   // cert: connect to where Host points (DNS), not to the device
}

// Outcome of one check.
type Outcome struct {
	RTT    time.Duration // time to connect
	Detail string        // the greeting line, or "HTTP 200 · certificate valid until …"
	Failed string        // the sub-probe that failed; "" when all passed
	Err    error
	Cert   *proto.CertInfo // cert: what the server presented (also when failed)
}

// Steps: the sub-probes a service runs, in order.
func Steps(s Service) []string {
	if s.Method == MethodDNS {
		st := []string{StepQuery, StepRcode, StepRecords}
		if s.Expect != "" {
			st = append(st, StepExpect)
		}
		if s.MaxMs > 0 {
			st = append(st, StepTime)
		}
		return st
	}
	if s.Method == MethodCert {
		st := []string{StepConnect, StepTLS}
		if s.CertValid {
			st = append(st, StepCert)
		}
		if s.CertTime {
			st = append(st, StepCertTime)
		}
		if s.CertDays > 0 {
			st = append(st, StepCertDays)
		}
		return st
	}
	if s.Method == MethodHTTPS {
		st := []string{StepConnect, StepTLS}
		if s.CertValid {
			st = append(st, StepCert)
		}
		if s.CertDays > 0 {
			st = append(st, StepCertDays)
		}
		return append(st, StepHTTP)
	}
	if s.Greeting {
		return []string{StepConnect, StepGreeting}
	}
	return []string{StepConnect}
}

// KnownStep: s is a sub-probe name (for results that come from an agent).
func KnownStep(s string) bool {
	switch s {
	case StepConnect, StepGreeting, StepTLS, StepCert, StepCertTime, StepCertDays, StepHTTP,
		StepQuery, StepRcode, StepRecords, StepExpect, StepTime:
		return true
	}
	return false
}

// Validate checks a service's settings.
func Validate(s Service) error {
	if s.Port < 1 || s.Port > 65535 {
		return errors.New("the port must be 1-65535")
	}
	switch s.Method {
	case "", MethodTCP:
		return nil
	case MethodDNS:
		return validDNS(s)
	case MethodCert:
		if s.Greeting {
			return errors.New("a certificate check does not wait for a greeting")
		}
		if err := ValidHost(s.Host); err != nil {
			return err
		}
		if s.CertDays < 0 || s.CertDays > MaxCertDays {
			return fmt.Errorf("certificate days must be 1-%d", MaxCertDays)
		}
		return nil
	case MethodHTTPS:
	default:
		return fmt.Errorf("unknown check method %q", s.Method)
	}
	if s.Greeting {
		return errors.New("an HTTPS check does not wait for a greeting")
	}
	if err := ValidHost(s.Host); err != nil {
		return err
	}
	if err := ValidPath(s.Path); err != nil {
		return err
	}
	if _, err := ParseCodes(s.Codes); err != nil {
		return err
	}
	if s.CertDays < 0 || s.CertDays > MaxCertDays {
		return fmt.Errorf("certificate days must be 1-%d", MaxCertDays)
	}
	return nil
}

// ValidHost: a hostname (FQDN) or an IP address.
func ValidHost(h string) error {
	if h == "" {
		return errors.New("a hostname is required for an HTTPS check")
	}
	if net.ParseIP(h) != nil {
		return nil
	}
	bad := errors.New("the hostname must look like www.example.com")
	if len(h) > 253 || strings.HasPrefix(h, ".") || strings.HasSuffix(h, ".") {
		return bad
	}
	for _, l := range strings.Split(h, ".") {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return bad
		}
		for _, c := range l {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return bad
			}
		}
	}
	return nil
}

// ValidPath: starts with "/", printable ASCII without spaces.
func ValidPath(p string) error {
	if p == "" || p[0] != '/' || len(p) > maxPath {
		return fmt.Errorf("the path must start with / (at most %d characters)", maxPath)
	}
	for _, c := range p {
		if c <= ' ' || c > '~' {
			return errors.New("the path may not contain spaces or special characters")
		}
	}
	return nil
}

// ParseCodes reads "200-399" or "200,204,301-302" (HTTP status 100-599).
func ParseCodes(s string) ([][2]int, error) {
	bad := errors.New(`status codes must look like 200-399 or 200,204,301-302 (100-599)`)
	var out [][2]int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(part, "-")
		a, err1 := strconv.Atoi(strings.TrimSpace(lo))
		b := a
		var err2 error
		if isRange {
			b, err2 = strconv.Atoi(strings.TrimSpace(hi))
		}
		if err1 != nil || err2 != nil || a < 100 || b > 599 || a > b {
			return nil, bad
		}
		out = append(out, [2]int{a, b})
	}
	if len(out) == 0 || len(out) > maxCodeRanges {
		return nil, bad
	}
	return out, nil
}

// CleanCodes: the codes written the same way every time ("200-399, 404" → "200-399,404").
func CleanCodes(s string) string {
	rs, err := ParseCodes(s)
	if err != nil {
		return s
	}
	parts := make([]string, len(rs))
	for i, r := range rs {
		if r[0] == r[1] {
			parts[i] = strconv.Itoa(r[0])
		} else {
			parts[i] = fmt.Sprintf("%d-%d", r[0], r[1])
		}
	}
	return strings.Join(parts, ",")
}

func codeOK(rs [][2]int, code int) bool {
	for _, r := range rs {
		if code >= r[0] && code <= r[1] {
			return true
		}
	}
	return false
}

// Check runs one service check against addr.
func Check(ctx context.Context, addr string, s Service, timeout time.Duration) Outcome {
	switch s.Method {
	case MethodHTTPS:
		return checkHTTPS(ctx, addr, s, timeout)
	case MethodDNS:
		return checkDNS(ctx, addr, s, timeout)
	case MethodCert:
		return checkCert(ctx, addr, s, timeout)
	}
	return checkTCP(ctx, addr, s, timeout)
}

func checkTCP(ctx context.Context, addr string, s Service, timeout time.Duration) Outcome {
	rtt, line, err := TCP(ctx, addr, s.Port, s.Greeting, timeout)
	if err != nil {
		var ge greetingError
		if errors.As(err, &ge) {
			return Outcome{RTT: rtt, Failed: StepGreeting, Err: err}
		}
		return Outcome{Failed: StepConnect, Err: err}
	}
	return Outcome{RTT: rtt, Detail: line}
}
