package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// client talks to the master over TLS pinned to the master's public key:
// the key must match one of the two slots.
type client struct {
	base string
	hc   *http.Client
	auth string

	mu      sync.Mutex
	pins    [2]string
	lastPin string // key of the most recently verified connection
}

func newClient(base string, pins [2]string) *client {
	c := &client{base: base, pins: pins}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// The certificate chain is not checked against a CA; instead the
			// server's public key must match a pinned key.
			InsecureSkipVerify: true,
			VerifyConnection: func(cs tls.ConnectionState) error {
				if len(cs.PeerCertificates) == 0 {
					return errors.New("no server certificate")
				}
				sum := sha256.Sum256(cs.PeerCertificates[0].RawSubjectPublicKeyInfo)
				got := base64.StdEncoding.EncodeToString(sum[:])
				c.mu.Lock()
				defer c.mu.Unlock()
				if got == "" || (got != c.pins[0] && got != c.pins[1]) {
					return errors.New("master certificate does not match a pinned key")
				}
				c.lastPin = got
				return nil
			},
		},
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConnsPerHost: 4,
		TLSHandshakeTimeout: 15 * time.Second,
	}
	c.hc = &http.Client{Transport: tr}
	return c
}

func (c *client) getPins() (pins [2]string, last string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pins, c.lastPin
}

func (c *client) setPins(p [2]string) {
	c.mu.Lock()
	c.pins = p
	c.mu.Unlock()
}

type httpError struct {
	Status int
	Body   string
}

func (e *httpError) Error() string { return fmt.Sprintf("master returned %d: %s", e.Status, e.Body) }

func (c *client) postJSON(ctx context.Context, path string, in, out any, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.auth != "" {
		req.Header.Set("Authorization", "Bearer "+c.auth)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return &httpError{Status: resp.StatusCode, Body: string(bytes.TrimSpace(b))}
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out)
}

// download streams a file from the master into w, up to max bytes.
func (c *client) download(ctx context.Context, path string, w io.Writer, max int64) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.auth)
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &httpError{Status: resp.StatusCode, Body: resp.Status}
	}
	n, err := io.Copy(w, io.LimitReader(resp.Body, max+1))
	if err != nil {
		return err
	}
	if n > max {
		return errors.New("download too large")
	}
	return nil
}
