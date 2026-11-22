package probe

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"time"

	"coremon/internal/proto"
)

const (
	maxChain = 8   // certificates kept of a chain
	maxNames = 100 // names kept per certificate
)

// checkCert: connect, TLS handshake, then the certificate checks that are
// switched on. No HTTP request is sent. With Remote the connection goes to
// where the name points (looked up in DNS) instead of the device's address.
func checkCert(ctx context.Context, addr string, s Service, timeout time.Duration) Outcome {
	if s.Remote {
		addr = s.Host
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
	state := tc.ConnectionState()
	remote := tc.RemoteAddr().String()
	tc.Close()
	chain := state.PeerCertificates
	leaf, now := chain[0], time.Now()
	info := certInfo(state, remote)
	left := int(math.Floor(leaf.NotAfter.Sub(now).Hours() / 24))
	detail := "valid until " + leaf.NotAfter.Format("2006-01-02")
	if left >= 0 {
		detail += fmt.Sprintf(" (%d day%s)", left, map[bool]string{true: "", false: "s"}[left == 1])
	}
	if iss := issuerName(leaf); iss != "" {
		detail += " · " + iss
	}
	fail := func(step string, err error) Outcome {
		return Outcome{RTT: rtt, Detail: detail, Failed: step, Err: err, Cert: info}
	}
	if s.CertValid {
		// The chain and the name only: the time is its own step, so check at
		// a moment the server's certificate is valid.
		at := leaf.NotBefore.Add(leaf.NotAfter.Sub(leaf.NotBefore) / 2)
		if err := verifyCert(chain, s.Host, at); err != nil {
			return fail(StepCert, err)
		}
	}
	if s.CertTime {
		for i, cc := range chain {
			what := "certificate"
			if i > 0 {
				what = "intermediate certificate " + issuerName(chain[i-1])
			}
			if now.Before(cc.NotBefore) {
				return fail(StepCertTime, fmt.Errorf("%s not valid before %s", what, cc.NotBefore.Format("2006-01-02")))
			}
			if now.After(cc.NotAfter) {
				return fail(StepCertTime, fmt.Errorf("%s expired on %s", what, cc.NotAfter.Format("2006-01-02")))
			}
		}
	}
	if s.CertDays > 0 {
		if err := certDays(leaf, s.CertDays, now); err != nil {
			return fail(StepCertDays, err)
		}
	}
	return Outcome{RTT: rtt, Detail: detail, Cert: info}
}

// certInfo: the connection and the presented chain, for the "Show" view.
func certInfo(st tls.ConnectionState, addr string) *proto.CertInfo {
	info := &proto.CertInfo{Addr: addr, TLS: tls.VersionName(st.Version), Cipher: tls.CipherSuiteName(st.CipherSuite)}
	for i, c := range st.PeerCertificates {
		if i == maxChain {
			break
		}
		e := proto.CertEntry{Subject: c.Subject.String(), Issuer: c.Issuer.String(), NotBefore: c.NotBefore.Unix(), NotAfter: c.NotAfter.Unix(),
			Key: keyText(c), SigAlg: c.SignatureAlgorithm.String(), Serial: strings.ToUpper(c.SerialNumber.Text(16)), CA: c.IsCA}
		for _, n := range c.DNSNames {
			e.Names = append(e.Names, n)
		}
		for _, ip := range c.IPAddresses {
			e.Names = append(e.Names, ip.String())
		}
		if len(e.Names) > maxNames {
			e.Names = e.Names[:maxNames]
		}
		sum := sha256.Sum256(c.Raw)
		hexes := make([]string, len(sum))
		for j, b := range sum {
			hexes[j] = fmt.Sprintf("%02X", b)
		}
		e.SHA256 = strings.Join(hexes, ":")
		info.Chain = append(info.Chain, e)
	}
	return info
}

func keyText(c *x509.Certificate) string {
	switch k := c.PublicKey.(type) {
	case *rsa.PublicKey:
		return "RSA " + strconv.Itoa(k.N.BitLen())
	case *ecdsa.PublicKey:
		return "ECDSA " + k.Curve.Params().Name
	case ed25519.PublicKey:
		return "Ed25519"
	}
	return c.PublicKeyAlgorithm.String()
}

// ParseTarget reads what an admin types for a certificate check:
// "example.com", "example.com:8443" or "https://example.com:8443/path"
// (the path is ignored). Port 443 when none is given.
func ParseTarget(s string) (host string, port int, err error) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "://"); i >= 0 {
		if !strings.EqualFold(s[:i], "https") {
			return "", 0, errors.New("only https:// addresses can be checked")
		}
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	port = 443
	if h, p, e := net.SplitHostPort(s); e == nil {
		n, cerr := strconv.Atoi(p)
		if cerr != nil || n < 1 || n > 65535 {
			return "", 0, errors.New("the port must be 1-65535")
		}
		s, port = h, n
	} else if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		s = s[1 : len(s)-1]
	}
	host = strings.ToLower(s)
	if err := ValidHost(host); err != nil {
		return "", 0, errors.New("the web address must look like www.example.com or www.example.com:8443")
	}
	return host, port, nil
}
