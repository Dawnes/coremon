package agent

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"coremon/internal/seal"
)

// client talks to the master.
//
// Sealed method (0.3.0+): HTTPS only carries the traffic and is not
// verified. Every request is encrypted to the master's payload key and
// signed with the agent's own key; only replies signed by a key in one of
// the two slots are accepted.
//
// Old method (used once, for the hand-over): HTTPS pinned to the master's
// certificate plus the shared secret.
type client struct {
	base string
	hc   *http.Client

	// sealed method
	agentID  string
	agentKey ed25519.PrivateKey

	// old method (hand-over only)
	auth string

	mu      sync.Mutex
	pins    [2]string // sealed: master payload keys; old: HTTPS key fingerprints
	lastPin string    // sealed: key that signed the last verified reply
	prefer  int       // slot that answered last
	legacy  bool
}

func transport(verify func(tls.ConnectionState) error) *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, // see verify / the sealed layer
			VerifyConnection:   verify,
		},
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConnsPerHost: 4,
		TLSHandshakeTimeout: 15 * time.Second,
	}
}

// newSealedClient trusts the master payload keys in keys.
func newSealedClient(base string, keys [2]string, agentKey ed25519.PrivateKey) *client {
	c := &client{base: base, pins: keys, agentKey: agentKey}
	c.hc = &http.Client{Transport: transport(nil)}
	return c
}

// newLegacyClient pins the master's HTTPS certificate (old method).
func newLegacyClient(base string, tlsPins [2]string) *client {
	c := &client{base: base, pins: tlsPins, legacy: true}
	c.hc = &http.Client{Transport: transport(func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return errors.New("no server certificate")
		}
		sum := sha256.Sum256(cs.PeerCertificates[0].RawSubjectPublicKeyInfo)
		got := base64.StdEncoding.EncodeToString(sum[:])
		c.mu.Lock()
		defer c.mu.Unlock()
		if got != c.pins[0] && got != c.pins[1] {
			return errors.New("master certificate does not match a pinned key")
		}
		return nil
	})}
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

func (c *client) post(ctx context.Context, path string, body []byte, auth bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if auth && c.auth != "" {
		req.Header.Set("Authorization", "Bearer "+c.auth)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, &httpError{Status: resp.StatusCode, Body: string(bytes.TrimSpace(b))}
	}
	return io.ReadAll(io.LimitReader(resp.Body, 24<<20))
}

// postJSON sends one API call: sealed, or (hand-over only) the old way.
func (c *client) postJSON(ctx context.Context, path string, in, out any, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if c.legacy {
		body, err := json.Marshal(in)
		if err != nil {
			return err
		}
		b, err := c.post(ctx, path, body, true)
		if err != nil || out == nil {
			return err
		}
		return json.Unmarshal(b, out)
	}
	op := strings.TrimPrefix(path, "/api/agent/")
	inner, err := json.Marshal(in)
	if err != nil {
		return err
	}
	plain, err := json.Marshal(map[string]json.RawMessage{"op": json.RawMessage(`"` + op + `"`), "body": inner})
	if err != nil {
		return err
	}
	// Seal to the slot that answered last; if the master no longer has
	// that key (after a switch), try the other slot.
	c.mu.Lock()
	keys, first := c.pins, c.prefer
	agentID := c.agentID
	c.mu.Unlock()
	var lastErr error
	for _, i := range []int{first, 1 - first} {
		if keys[i] == "" {
			continue
		}
		req, pend, err := seal.SealRequest(agentID, c.agentKey, keys[i], keys[:], plain)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := json.Marshal(req)
		b, err := c.post(ctx, "/api/agent/v5", body, false)
		var he *httpError
		if errors.As(err, &he) && he.Status == http.StatusMisdirectedRequest {
			lastErr = err
			continue // master does not have this key: try the other slot
		}
		if err != nil {
			return err
		}
		var rep seal.Reply
		if err := json.Unmarshal(b, &rep); err != nil {
			return fmt.Errorf("bad reply: %w", err)
		}
		reply, err := pend.OpenReply(&rep)
		if err != nil {
			return fmt.Errorf("refused reply: %w", err)
		}
		c.mu.Lock()
		c.lastPin, c.prefer = keys[i], i
		c.mu.Unlock()
		if out == nil {
			return nil
		}
		return json.Unmarshal(reply, out)
	}
	if lastErr == nil {
		lastErr = errors.New("no master key to use")
	}
	return lastErr
}

// download streams a public file from the master into w, up to max bytes.
// Its integrity is checked by the caller (release signature and checksum).
func (c *client) download(ctx context.Context, path string, w io.Writer, max int64) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
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
