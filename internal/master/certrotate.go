package master

// A new self-signed HTTPS certificate: on request (Settings > Identity) and
// automatically 30 days before the current one expires. Agents do not rely
// on it; browsers ask once to accept the new one.

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/url"
	"os"
	"time"
)

const (
	certValidity  = 1                   // years
	certRenewLead = 30 * 24 * time.Hour // renew this long before expiry
	certRetry     = time.Hour           // after a failed automatic renewal
)

func parseLeaf(c *tls.Certificate) (*x509.Certificate, error) {
	if c == nil || len(c.Certificate) == 0 {
		return nil, fmt.Errorf("no certificate")
	}
	return x509.ParseCertificate(c.Certificate[0])
}

// certDates returns when a certificate was made and when it expires.
func certDates(c *tls.Certificate) (from, until time.Time) {
	leaf, err := parseLeaf(c)
	if err != nil {
		return
	}
	return leaf.NotBefore, leaf.NotAfter
}

// certNames: the names the certificate is made for besides the host name and
// IP addresses: -san flags and a Master URL set by hand.
func (s *Server) certNames() []string {
	names := append([]string{}, s.cfg.SANs...)
	s.st.mu.Lock()
	mu := s.st.st.Settings.MasterURL
	s.st.mu.Unlock()
	if u, err := url.Parse(mu); err == nil && u.Hostname() != "" {
		names = append(names, u.Hostname())
	}
	return names
}

// rotateCert replaces the HTTPS certificate with a new one and uses it at
// once (new connections). The files are swapped so that an interrupted
// rotation is completed at the next start (see recoverCert).
func (s *Server) rotateCert(actor, ip string) error {
	s.certMu.Lock()
	defer s.certMu.Unlock()
	crt, key := certPaths(s.cfg.DataDir, "current")
	if err := createCert(crt+".new", key+".new", s.certNames()); err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}
	cert, pin, err := loadCert(crt+".new", key+".new")
	if err != nil {
		os.Remove(crt + ".new")
		os.Remove(key + ".new")
		return fmt.Errorf("load new certificate: %w", err)
	}
	if err := os.Rename(key+".new", key); err != nil {
		return err
	}
	if err := os.Rename(crt+".new", crt); err != nil {
		return err
	}
	s.keyMu.Lock()
	old := s.keys.tlsPin
	s.keys.cert, s.keys.tlsPin = &cert, pin
	s.keyMu.Unlock()
	// Browsers would otherwise keep the old certificate: they resume their
	// TLS session (no certificate is sent then) and reuse open connections.
	s.resetTLSSessions()
	_, until := certDates(&cert)
	s.audit.write(actor, ip, "identity.cert", "", fmt.Sprintf("new HTTPS certificate %s → %s, valid until %s",
		shortPin(old), shortPin(pin), until.Format("02-01-2006")))
	log.Printf("HTTPS certificate replaced (%s), valid until %s", actor, until.Format("2006-01-02"))
	return nil
}

func newTicketKey() (k [32]byte) {
	rand.Read(k[:])
	return k
}

// applyTicketKeys builds the TLS settings handed out per connection, with
// these session ticket keys (tickets made with other keys are rejected, so
// the browser does a full handshake). Caller holds s.tlsMu.
func (s *Server) applyTicketKeys(keys [][32]byte) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: s.currentCert, NextProtos: []string{"h2", "http/1.1"}}
	cfg.SetSessionTicketKeys(keys)
	s.ticketKeys, s.ticketKeyAt, s.tlsCfg = keys, time.Now(), cfg
}

// tlsNow returns the TLS settings for a new connection.
func (s *Server) tlsNow() *tls.Config {
	s.tlsMu.Lock()
	defer s.tlsMu.Unlock()
	return s.tlsCfg
}

// resetTLSSessions makes every browser do a full handshake (and so get the
// current certificate): all session tickets become invalid, and idle
// keep-alive connections are closed.
func (s *Server) resetTLSSessions() {
	s.tlsMu.Lock()
	defer s.tlsMu.Unlock()
	s.applyTicketKeys([][32]byte{newTicketKey()})
	for _, srv := range s.https {
		srv.SetKeepAlivesEnabled(false) // closes idle connections
		srv.SetKeepAlivesEnabled(true)
	}
}

// renewTicketKeys makes a new session ticket key once a day; the previous one
// stays valid one more day, so sessions are resumed but not forever.
func (s *Server) renewTicketKeys(now time.Time) {
	s.tlsMu.Lock()
	defer s.tlsMu.Unlock()
	if len(s.ticketKeys) == 0 || now.Sub(s.ticketKeyAt) < 24*time.Hour {
		return
	}
	s.applyTicketKeys([][32]byte{newTicketKey(), s.ticketKeys[0]})
}

// recoverCert completes a rotation that was interrupted between the two
// file renames, so the key and certificate always match.
func recoverCert(dir string) {
	crt, key := certPaths(dir, "current")
	if _, _, err := loadCert(crt, key); err == nil {
		os.Remove(crt + ".new")
		os.Remove(key + ".new")
		return
	}
	if _, _, err := loadCert(crt+".new", key); err == nil { // key already renamed
		os.Rename(crt+".new", crt)
		return
	}
	if _, _, err := loadCert(crt+".new", key+".new"); err == nil {
		os.Rename(key+".new", key)
		os.Rename(crt+".new", crt)
	}
}

// renewCertIfDue renews the certificate automatically when it expires within
// certRenewLead; after a failure it waits certRetry.
func (s *Server) renewCertIfDue(now time.Time) {
	s.keyMu.Lock()
	cert := s.keys.cert
	s.keyMu.Unlock()
	_, until := certDates(cert)
	if until.IsZero() || until.Sub(now) > certRenewLead || now.Sub(s.certTried) < certRetry {
		return
	}
	s.certTried = now
	if err := s.rotateCert("scheduler", ""); err != nil {
		log.Printf("automatic HTTPS certificate renewal failed: %v (retrying in %s)", err, certRetry)
	}
}
