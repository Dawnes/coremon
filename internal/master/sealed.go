package master

// The sealed agent API (agents 0.3.0+): every request is encrypted to the
// master's payload key and signed by the agent's own key; every reply is
// encrypted and signed by the master, bound to the request. HTTPS only
// carries it.

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"coremon/internal/proto"
	"coremon/internal/seal"
)

// maxSealedBody bounds a sealed request (output batches and results are the
// largest payloads).
const maxSealedBody = 12 << 20

type sealedMsg struct {
	Op   string          `json:"op"` // enroll | poll | output | result
	Body json.RawMessage `json:"body"`
}

// nonceCache rejects a request seen before (replay), for as long as its
// timestamp would still be accepted.
type nonceCache struct {
	mu   sync.Mutex
	seen map[string]int64
}

func (c *nonceCache) fresh(nonce string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now().Unix()
	if c.seen == nil {
		c.seen = map[string]int64{}
	}
	if len(c.seen) > 100000 || now%60 == 0 {
		for k, exp := range c.seen {
			if exp < now {
				delete(c.seen, k)
			}
		}
	}
	if _, dup := c.seen[nonce]; dup {
		return false
	}
	c.seen[nonce] = now + int64(2*seal.MaxSkew/time.Second)
	return true
}

func (s *Server) handleSealed(w http.ResponseWriter, r *http.Request) {
	// Admitted per address and in total, before anything is read: the sender
	// is not known yet.
	if r.ContentLength > maxSealedBody {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	release := s.intake.admit(w, r)
	if release == nil {
		return
	}
	defer release()
	var env seal.Request
	if !readJSON(w, r, &env, maxSealedBody) {
		return
	}
	var msg sealedMsg
	opened, err := seal.OpenRequest(&env, s.payloadKeys(), func(agentID string, plain []byte) (ed25519.PublicKey, error) {
		if err := json.Unmarshal(plain, &msg); err != nil {
			return nil, errors.New("bad request")
		}
		if agentID == "" {
			// Enrollment: the request proves possession of the key it brings.
			if msg.Op != "enroll" {
				return nil, errors.New("bad request")
			}
			var e struct {
				AgentKey string `json:"agent_key"`
			}
			json.Unmarshal(msg.Body, &e)
			return seal.ParseAgentKey(e.AgentKey)
		}
		s.st.mu.Lock()
		defer s.st.mu.Unlock()
		a := s.st.st.Agents[agentID]
		if a == nil || a.AgentKey == "" {
			// The same answer as for a wrong signature: nobody can find
			// out which agent IDs exist.
			return nil, seal.ErrBadSig
		}
		return seal.ParseAgentKey(a.AgentKey)
	})
	if err != nil {
		switch {
		case errors.Is(err, seal.ErrUnknownKey):
			http.Error(w, "unknown master key", http.StatusMisdirectedRequest)
		case errors.Is(err, seal.ErrStale):
			http.Error(w, "request timestamp out of range (check the clock)", http.StatusBadRequest)
		default:
			var ae *apiError
			if errors.As(err, &ae) {
				http.Error(w, ae.msg, ae.code)
			} else {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
			}
		}
		return
	}
	// Rate limit per verified agent (checked after the signature, so nobody
	// can slow down another agent by using its ID); enrollments per address.
	key := env.Agent
	if key == "" {
		key = "enroll:" + clientIP(r)
	}
	if !s.agentRate.allow(key, time.Now()) {
		http.Error(w, "too many requests", http.StatusTooManyRequests)
		return
	}
	if !s.nonces.fresh(env.Nonce) {
		http.Error(w, "replayed request", http.StatusUnauthorized)
		return
	}
	ip := clientIP(r)
	var reply any = struct{}{}
	switch msg.Op {
	case "enroll":
		var req proto.EnrollRequest
		var extra struct {
			AgentKey string `json:"agent_key"`
		}
		json.Unmarshal(msg.Body, &req)
		json.Unmarshal(msg.Body, &extra)
		resp, err := s.enrollCore(req, ip, extra.AgentKey)
		if err != nil {
			writeAPIError(w, err)
			return
		}
		reply = resp
	case "poll":
		var req proto.PollRequest
		if len(msg.Body) > maxPollBody || json.Unmarshal(msg.Body, &req) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		release() // a poll waits for commands; it must not hold an intake slot
		resp, err := s.pollCore(r.Context(), env.Agent, req)
		if err != nil {
			if r.Context().Err() == nil {
				writeAPIError(w, err)
			}
			return
		}
		reply = resp
	case "output":
		var req proto.OutputBatch
		if json.Unmarshal(msg.Body, &req) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		s.outputCore(env.Agent, req)
	case "result":
		var res proto.Result
		if json.Unmarshal(msg.Body, &res) != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		s.resultCore(env.Agent, res, ip)
	default:
		http.Error(w, "unknown operation", http.StatusBadRequest)
		return
	}
	plain, _ := json.Marshal(reply)
	rep, err := opened.SealReply(plain)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, rep)
}

// handlePublicCore serves any agent version in the store. Binaries are not
// secret; agents check the release signature and checksum.
func (s *Server) handlePublicCore(w http.ResponseWriter, r *http.Request) {
	goos, arch, ok := strings.Cut(r.PathValue("plat"), "-")
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.serveArtifact(w, r, s.repo.find("core", r.PathValue("version"), goos, arch))
}

var _ = io.EOF
