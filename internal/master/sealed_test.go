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
		agent.Run(ctx, agent.Options{ConfigPath: filepath.Join(dir, "agent.json"), StateDir: dir, Version: "0.3.1"})
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
			if a.Protocol >= 5 && a.AgentKey != "" {
				newID = id
				return true
			}
		}
		return false
	})

	// 2. Ping: answered at once, also while pending and in report only.
	ping := func() pingResult {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/ui/agents/"+newID+"/ping", nil)
		r.SetPathValue("id", newID)
		w := httptest.NewRecorder()
		s.handlePing(w, r)
		var p pingResult
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &p) != nil {
			t.Fatalf("ping: %d %s", w.Code, w.Body)
		}
		return p
	}
	waitFor(t, "agent version reported", 10*time.Second, func() bool {
		s.st.mu.Lock()
		defer s.st.mu.Unlock()
		return s.st.st.Agents[newID].CoreVersion == "0.3.1"
	})
	for _, status := range []string{StatusPending, StatusReportOnly} {
		s.st.mu.Lock()
		s.st.st.Agents[newID].Status = status
		s.st.mu.Unlock()
		p := ping()
		if !p.Answered || p.Version != "0.3.1" || !p.KeyCurrent || !p.SlotsOK || p.ClockMs > 5000 || p.ClockMs < -5000 {
			t.Fatalf("ping while %s: %+v", status, p)
		}
	}
	s.st.mu.Lock()
	s.st.st.Agents[newID].Status = StatusActive
	s.st.mu.Unlock()
	jobID := ping().JobID
	s.st.mu.Lock()
	j := s.st.jobByID(jobID)
	ok := j != nil && j.Status == JobSuccess && j.Type == proto.CmdPing
	s.st.mu.Unlock()
	if !ok {
		t.Fatalf("ping job: %+v", j)
	}

	// The old method's endpoints are gone.
	hc := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	for _, p := range []string{"/api/agent/poll", "/api/agent/enroll", "/api/agent/migrate"} {
		resp, err := hc.Post(url+p, "application/json", strings.NewReader(`{}`))
		if err != nil || resp.StatusCode == 200 || resp.StatusCode == 204 {
			t.Fatalf("%s still answers: %v", p, err)
		}
		resp.Body.Close()
	}
	// An agent with a config from before 0.3.0 refuses to start.
	odir := t.TempDir()
	os.WriteFile(filepath.Join(odir, "agent.json"), []byte(`{"master_url":"`+url+`","master_pins":["`+s.currentPin()+`",""]}`), 0o600)
	if _, err := agent.Run(context.Background(), agent.Options{ConfigPath: filepath.Join(odir, "agent.json"), StateDir: odir, Version: "0.3.1"}); err == nil || !strings.Contains(err.Error(), "install command") {
		t.Fatalf("old config: %v", err)
	}

	// 3. Key rotation with the agent running.
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
