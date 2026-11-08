package master

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"coremon/internal/agent"
	"coremon/internal/proto"
	"coremon/internal/seal"
)

// liveMaster runs the master's routes over HTTPS with its own certificate.
func liveMaster(t *testing.T) (*Server, string) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	s.st.st.Settings.HeartbeatSec = minHeartbeat
	srv := httptest.NewUnstartedServer(s.routes())
	// The master's own HTTPS certificate (agents of the old method pin it).
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{*s.keys.cert}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return s, srv.URL
}

func runAgent(t *testing.T, cfg map[string]any, identity map[string]string) (dir string, stop func()) {
	dir = t.TempDir()
	b, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(dir, "agent.json"), b, 0o600)
	if identity != nil {
		b, _ = json.Marshal(identity)
		os.WriteFile(filepath.Join(dir, "identity.json"), b, 0o600)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		agent.Run(ctx, agent.Options{ConfigPath: filepath.Join(dir, "agent.json"), StateDir: dir, Version: "0.3.0"})
		close(done)
	}()
	return dir, func() { cancel(); <-done }
}

func waitFor(t *testing.T, what string, d time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", what)
}

func TestSealedAgentLifecycle(t *testing.T) {
	s, url := liveMaster(t)

	// 1. A new agent enrolls with the sealed method (no shared secret).
	s.st.mu.Lock()
	s.st.st.Tokens[sha256hex("abc123")] = &EnrollToken{Hash: sha256hex("abc123"), Prefix: "abc123", Expires: time.Now().Add(time.Hour).Unix()}
	s.st.mu.Unlock()
	_, stopNew := runAgent(t, map[string]any{"master_url": url, "master_keys": s.desiredPins(), "token": "abc123"}, nil)
	defer stopNew()
	var newID string
	waitFor(t, "sealed enrollment", 20*time.Second, func() bool {
		s.st.mu.Lock()
		defer s.st.mu.Unlock()
		for id, a := range s.st.st.Agents {
			if a.Protocol >= 5 && a.AgentKey != "" && a.SecretHash == "" {
				newID = id
				return true
			}
		}
		return false
	})

	// 2. An agent of the old method hands over: it trusts the HTTPS key
	//    and has a shared secret; afterwards it uses the payload key and the
	//    secret is retired.
	s.st.mu.Lock()
	s.st.st.Agents["a_old"] = &Agent{ID: "a_old", Name: "old", Status: StatusActive, SecretHash: sha256hex("s3cret"), LastSeen: time.Now().Unix(),
		State: map[string]json.RawMessage{}, StateAt: map[string]int64{}}
	s.st.mu.Unlock()
	oldDir, stopOld := runAgent(t, map[string]any{"master_url": url, "master_pins": []string{s.currentPin(), ""}},
		map[string]string{"agent_id": "a_old", "secret": "s3cret", "signing_key": s.signingKeyB64})
	defer stopOld()
	waitFor(t, "hand-over of the old agent", 30*time.Second, func() bool {
		s.st.mu.Lock()
		defer s.st.mu.Unlock()
		a := s.st.st.Agents["a_old"]
		return a.Protocol >= 5 && a.AgentKey != "" && a.SecretHash == ""
	})
	var cfg struct {
		MasterKeys [2]string `json:"master_keys"`
		MasterPins [2]string `json:"master_pins"`
	}
	b, _ := os.ReadFile(filepath.Join(oldDir, "agent.json"))
	json.Unmarshal(b, &cfg)
	if cfg.MasterKeys != s.desiredPins() || cfg.MasterPins != [2]string{} {
		t.Fatalf("old agent config after hand-over: %s", b)
	}
	// The retired secret no longer works with the old method.
	req, _ := http.NewRequest("POST", url+"/api/agent/poll", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer a_old:s3cret")
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	if resp, err := hc.Do(req); err != nil || resp.StatusCode != 401 {
		t.Fatalf("old secret after hand-over: %v %v", err, resp.StatusCode)
	}

	// 3. Key rotation with both agents running.
	if err := s.generateKey(); err != nil {
		t.Fatal(err)
	}
	next := s.desiredPins()[1]
	s.st.mu.Lock()
	for _, a := range s.st.st.Agents {
		s.syncPins(a, "test")
		s.wake(a.ID)
	}
	s.st.mu.Unlock()
	waitFor(t, "both agents trust the new key", 30*time.Second, func() bool { return len(s.missingNextKey()) == 0 })
	if _, _, err := s.switchKey(); err != nil {
		t.Fatal(err)
	}
	s.st.mu.Lock()
	for _, a := range s.st.st.Agents {
		s.syncPins(a, "test")
		s.wake(a.ID)
	}
	s.st.mu.Unlock()
	waitFor(t, "agents keep only the new key", 40*time.Second, func() bool {
		s.st.mu.Lock()
		defer s.st.mu.Unlock()
		for _, a := range s.st.st.Agents {
			if a.Pins != [2]string{next, ""} {
				return false
			}
		}
		return true
	})
	_ = newID
}

func TestSealedReplayAndForgery(t *testing.T) {
	s, url := liveMaster(t)
	apub, apriv, _ := ed25519.GenerateKey(rand.Reader)
	s.st.mu.Lock()
	s.st.st.Agents["a_1"] = &Agent{ID: "a_1", Name: "web-01", Status: StatusActive, AgentKey: seal.AgentKey(apub),
		State: map[string]json.RawMessage{}, StateAt: map[string]int64{}}
	s.st.mu.Unlock()
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	post := func(r *seal.Request) int {
		b, _ := json.Marshal(r)
		resp, err := hc.Post(url+"/api/agent/v5", "application/json", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	key := s.desiredPins()[0]
	plain, _ := json.Marshal(map[string]any{"op": "result", "body": proto.Result{CommandID: "j_x", OK: true}})
	req, _, _ := seal.SealRequest("a_1", apriv, key, []string{key}, plain)
	if code := post(req); code != 200 {
		t.Fatalf("genuine request: %d", code)
	}
	if code := post(req); code != 401 {
		t.Fatalf("replayed request must be refused, got %d", code)
	}
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	forged, _, _ := seal.SealRequest("a_1", other, key, []string{key}, plain)
	if code := post(forged); code != 401 {
		t.Fatalf("request signed by another key must be refused, got %d", code)
	}
	evil, _ := seal.GenerateKeyPair()
	wrong, _, _ := seal.SealRequest("a_1", apriv, evil.Public().String(), nil, plain)
	if code := post(wrong); code != http.StatusMisdirectedRequest {
		t.Fatalf("request sealed to an unknown master key: %d", code)
	}
}
