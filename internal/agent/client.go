package agent

// HTTPS client for the sealed agent API.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
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

type client struct {
	base string
	hc   *http.Client

	agentID  string
	agentKey ed25519.PrivateKey

	mu      sync.Mutex
	pins    [2]string
	lastPin string
	prefer  int

	plain bool
}

func newSealedClient(base string, keys [2]string, agentKey ed25519.PrivateKey) *client {
	c := &client{base: base, pins: keys, agentKey: agentKey}
	c.hc = &http.Client{Transport: &http.Transport{
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true},
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConnsPerHost: 4,
		TLSHandshakeTimeout: 15 * time.Second,
	}}
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

func (c *client) post(ctx context.Context, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
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

func (c *client) postJSON(ctx context.Context, path string, in, out any, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	inner, err := json.Marshal(in)
	if err != nil {
		return err
	}
	if c.plain {
		b, err := c.post(ctx, path, inner)
		if err != nil || out == nil {
			return err
		}
		return json.Unmarshal(b, out)
	}
	op := strings.TrimPrefix(path, "/api/agent/")
	plain, err := json.Marshal(map[string]json.RawMessage{"op": json.RawMessage(`"` + op + `"`), "body": inner})
	if err != nil {
		return err
	}

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
		b, err := c.post(ctx, "/api/agent/v5", body)
		var he *httpError
		if errors.As(err, &he) && he.Status == http.StatusMisdirectedRequest {
			lastErr = err
			continue
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
