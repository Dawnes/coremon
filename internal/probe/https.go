package probe

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// checkHTTPS: connect, TLS handshake, certificate checks, HTTP status.
func checkHTTPS(ctx context.Context, addr string, s Service, timeout time.Duration) Outcome {
	codes, err := ParseCodes(s.Codes)
	if err != nil {
		return Outcome{Failed: StepHTTP, Err: err}
	}
	start := time.Now()
	c, err := dial(ctx, addr, s.Port, timeout)
	if err != nil {
		return Outcome{Failed: StepConnect, Err: err}
	}
	rtt := time.Since(start)
	tc, err := handshake(c, s.Host, timeout)
	if err != nil {
		c.Close()
		return Outcome{RTT: rtt, Failed: StepTLS, Err: err}
	}
	defer tc.Close()
	state := tc.ConnectionState()
	leaf := state.PeerCertificates[0]
	now := time.Now()
	certInfo := "certificate valid until " + leaf.NotAfter.Format("2006-01-02")
	if iss := issuerName(leaf); iss != "" {
		certInfo += " · " + iss
	}
	if s.CertValid {
		if err := verifyCert(state.PeerCertificates, s.Host, now); err != nil {
			return Outcome{RTT: rtt, Detail: certInfo, Failed: StepCert, Err: err}
		}
	}
	if s.CertDays > 0 {
		if err := certDays(leaf, s.CertDays, now); err != nil {
			return Outcome{RTT: rtt, Detail: certInfo, Failed: StepCertDays, Err: err}
		}
	}

	// HEAD first; when that fails (no answer, or a status that is not
	// accepted, as some servers answer HEAD badly) one GET on a new connection.
	code, herr := ask(tc, "HEAD", s, timeout)
	method := "HEAD"
	if herr != nil || !codeOK(codes, code) {
		c2, err := dial(ctx, addr, s.Port, timeout)
		if err == nil {
			var tc2 *tls.Conn
			if tc2, err = handshake(c2, s.Host, timeout); err == nil {
				code, err = ask(tc2, "GET", s, timeout)
				tc2.Close()
			} else {
				c2.Close()
			}
		}
		if err != nil {
			if herr == nil {
				err = fmt.Errorf("HTTP %d to HEAD; GET failed: %v", code, err)
			}
			return Outcome{RTT: rtt, Detail: certInfo, Failed: StepHTTP, Err: err}
		}
		method = "GET"
	}
	detail := fmt.Sprintf("HTTP %d", code)
	if method == "GET" {
		detail += " (GET)"
	}
	detail += " · " + certInfo
	if !codeOK(codes, code) {
		return Outcome{RTT: rtt, Detail: detail, Failed: StepHTTP, Err: fmt.Errorf("HTTP %d, expected %s", code, CleanCodes(s.Codes))}
	}
	return Outcome{RTT: rtt, Detail: detail}
}

// handshake does the TLS handshake; the certificate is checked separately,
// so a self-signed certificate does not stop the check here.
func handshake(c net.Conn, host string, timeout time.Duration) (*tls.Conn, error) {
	cfg := &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS10}
	if net.ParseIP(host) == nil {
		cfg.ServerName = host
	}
	tc := tls.Client(c, cfg)
	tc.SetDeadline(time.Now().Add(timeout))
	if err := tc.Handshake(); err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return nil, fmt.Errorf("no TLS handshake within %s", timeout)
		}
		return nil, fmt.Errorf("TLS handshake failed: %v", UnwrapOp(err))
	}
	if len(tc.ConnectionState().PeerCertificates) == 0 {
		tc.Close()
		return nil, errors.New("TLS handshake failed: no certificate")
	}
	return tc, nil
}

// verifyCert: trusted by the system's roots, valid for host, not expired.
func verifyCert(chain []*x509.Certificate, host string, now time.Time) error {
	leaf := chain[0]
	inter := x509.NewCertPool()
	for _, c := range chain[1:] {
		inter.AddCert(c)
	}
	_, err := leaf.Verify(x509.VerifyOptions{DNSName: host, Intermediates: inter, CurrentTime: now})
	if err == nil {
		return nil
	}
	var inv x509.CertificateInvalidError
	var hn x509.HostnameError
	var ua x509.UnknownAuthorityError
	switch {
	case errors.As(err, &inv) && inv.Reason == x509.Expired && now.After(leaf.NotAfter):
		return fmt.Errorf("certificate expired on %s", leaf.NotAfter.Format("2006-01-02"))
	case errors.As(err, &inv) && inv.Reason == x509.Expired:
		return fmt.Errorf("certificate not valid before %s", leaf.NotBefore.Format("2006-01-02"))
	case errors.As(err, &hn):
		return fmt.Errorf("certificate is not valid for %s", host)
	case errors.As(err, &ua):
		if iss := issuerName(leaf); iss != "" {
			return fmt.Errorf("certificate not trusted (issued by %s)", iss)
		}
		return errors.New("certificate not trusted")
	}
	return fmt.Errorf("certificate not valid: %v", err)
}

// certDays: still valid for at least days days (dates only, trusted or not).
func certDays(leaf *x509.Certificate, days int, now time.Time) error {
	if now.Before(leaf.NotBefore) {
		return fmt.Errorf("certificate not valid before %s", leaf.NotBefore.Format("2006-01-02"))
	}
	left := leaf.NotAfter.Sub(now)
	if left <= 0 {
		return fmt.Errorf("certificate expired on %s", leaf.NotAfter.Format("2006-01-02"))
	}
	if left < time.Duration(days)*24*time.Hour {
		n := int(math.Floor(left.Hours() / 24))
		word := "days"
		if n == 1 {
			word = "day"
		}
		return fmt.Errorf("certificate expires in %d %s (needs %d)", n, word, days)
	}
	return nil
}

func issuerName(c *x509.Certificate) string {
	if c.Issuer.CommonName != "" {
		return c.Issuer.CommonName
	}
	if len(c.Issuer.Organization) > 0 {
		return c.Issuer.Organization[0]
	}
	return ""
}

// ask sends one request and reads only the status line and headers.
func ask(c *tls.Conn, method string, s Service, timeout time.Duration) (int, error) {
	host := s.Host
	if s.Port != 443 {
		host = net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	c.SetDeadline(time.Now().Add(timeout))
	req := fmt.Sprintf("%s %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: Coremon\r\nAccept: */*\r\nConnection: close\r\n\r\n", method, s.Path, host)
	if _, err := c.Write([]byte(req)); err != nil {
		return 0, fmt.Errorf("request not sent: %v", UnwrapOp(err))
	}
	resp, err := http.ReadResponse(bufio.NewReaderSize(c, 4096), &http.Request{Method: method})
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return 0, fmt.Errorf("no HTTP answer within %s", timeout)
		}
		return 0, fmt.Errorf("no HTTP answer: %v", UnwrapOp(err))
	}
	// The body is not read: the connection is closed right after.
	return resp.StatusCode, nil
}
