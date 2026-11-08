package master

// Master key rotation. The master has one key in use (its TLS key, which
// agents pin) and during a rotation a second, generated key. Agents have two
// key slots:
//
//  1. Generate: a new key is created; the master keeps serving the old one.
//  2. Distribute: every agent is told, over its connection verified with the
//     old key, to trust [old, new]. Agents report what they stored.
//  3. Switch: the master serves the new key, closes existing connections so
//     agents reconnect with it, and tells agents to keep only [new].
//  Cancel before the switch discards the new key; agents drop it again.

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"coremon/internal/proto"
)

// keyState is the master's view of its keys.
type keyState struct {
	cert, next   *tls.Certificate
	pin, nextPin string
	nextAt       int64
	conns        map[net.Conn]struct{} // agent-port connections, closed on a switch
}

func (s *Server) currentCert(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	return s.keys.cert, nil
}

func (s *Server) currentPin() string {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	return s.keys.pin
}

// desiredPins is what every agent should trust right now.
func (s *Server) desiredPins() [2]string {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	return [2]string{s.keys.pin, s.keys.nextPin}
}

func (s *Server) loadNextKey() {
	crt, key := certPaths(s.cfg.DataDir, "next")
	st, err := os.Stat(crt)
	if err != nil {
		return
	}
	cert, pin, err := loadCert(crt, key)
	if err != nil {
		log.Printf("key rotation: cannot load the new key: %v", err)
		return
	}
	s.keys.next, s.keys.nextPin, s.keys.nextAt = &cert, pin, st.ModTime().Unix()
}

// trackConn remembers agent-port connections (http.Server.ConnState).
func (s *Server) trackConn(c net.Conn, state http.ConnState) {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	switch state {
	case http.StateNew:
		s.keys.conns[c] = struct{}{}
	case http.StateClosed, http.StateHijacked:
		delete(s.keys.conns, c)
	}
}

func (s *Server) generateKey() error {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	if s.keys.next != nil {
		return errors.New("a new key was already generated; switch to it or cancel first")
	}
	crt, key := certPaths(s.cfg.DataDir, "next")
	if err := createCert(crt, key, s.cfg.SANs); err != nil {
		return err
	}
	cert, pin, err := loadCert(crt, key)
	if err != nil {
		return err
	}
	s.keys.next, s.keys.nextPin, s.keys.nextAt = &cert, pin, time.Now().Unix()
	return nil
}

func (s *Server) cancelKey() error {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	if s.keys.next == nil {
		return errors.New("no new key to cancel")
	}
	crt, key := certPaths(s.cfg.DataDir, "next")
	os.Remove(crt)
	os.Remove(key)
	s.keys.next, s.keys.nextPin, s.keys.nextAt = nil, "", 0
	return nil
}

// switchKey makes the new key current and deletes the old one.
func (s *Server) switchKey() (old, cur string, err error) {
	s.keyMu.Lock()
	if s.keys.next == nil {
		s.keyMu.Unlock()
		return "", "", errors.New("generate a new key first")
	}
	nc, nk := certPaths(s.cfg.DataDir, "next")
	cc, ck := certPaths(s.cfg.DataDir, "current")
	if err := os.Rename(nk, ck); err != nil {
		s.keyMu.Unlock()
		return "", "", err
	}
	if err := os.Rename(nc, cc); err != nil {
		s.keyMu.Unlock()
		return "", "", err
	}
	old = s.keys.pin
	s.keys.cert, s.keys.pin = s.keys.next, s.keys.nextPin
	s.keys.next, s.keys.nextPin, s.keys.nextAt = nil, "", 0
	cur = s.keys.pin
	conns := make([]net.Conn, 0, len(s.keys.conns))
	for c := range s.keys.conns {
		conns = append(conns, c)
	}
	s.keyMu.Unlock()
	// Existing connections were set up with the old key; close them so every
	// agent reconnects (and is then told to drop the old key).
	for _, c := range conns {
		c.Close()
	}
	return old, cur, nil
}

// syncPins tells an agent which keys to trust, until it reports exactly
// that. Caller holds s.st.mu.
func (s *Server) syncPins(a *Agent, actor string) *Job {
	if a.Protocol < 4 || a.Removing {
		return nil
	}
	want := s.desiredPins()
	if a.Pins == want || s.openJob(a.ID, proto.CmdSetPins, "") {
		return nil
	}
	p, _ := json.Marshal(proto.PinParams{Pins: want})
	return s.enqueue(a, &Job{Type: proto.CmdSetPins, Params: p, Actor: actor, Timeout: 60})
}

// ---- UI ----

type agentKeys struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Online     bool      `json:"online"`
	LastSeen   int64     `json:"last_seen"`
	Version    string    `json:"core_version"`
	Slots      bool      `json:"slots"` // agent has two key slots (0.2.5+)
	Pins       [2]string `json:"pins"`
	ReportedAt int64     `json:"reported_at"`
	HasNext    bool      `json:"has_next"`
}

func (s *Server) identityView() map[string]any {
	s.keyMu.Lock()
	cur, next, nextAt := s.keys.pin, s.keys.nextPin, s.keys.nextAt
	s.keyMu.Unlock()
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	now := time.Now().Unix()
	list := []agentKeys{}
	for _, a := range s.st.st.Agents {
		k := agentKeys{ID: a.ID, Name: a.Name, Status: a.Status, Online: s.online(a, now), LastSeen: a.LastSeen,
			Version: a.CoreVersion, Slots: a.Protocol >= 4, Pins: a.Pins, ReportedAt: a.PinsAt}
		k.HasNext = next != "" && (a.Pins[0] == next || a.Pins[1] == next)
		list = append(list, k)
	}
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
	return map[string]any{"current": cur, "next": next, "next_at": nextAt, "agents": list}
}

func (s *Server) handleIdentity(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.identityView())
}

func (s *Server) handleIdentityAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Force bool `json:"force"`
	}
	if !readJSON(w, r, &req, 1024) {
		return
	}
	var err error
	switch r.PathValue("action") {
	case "generate":
		if err = s.generateKey(); err == nil {
			s.audit.write(actor(r), clientIP(r), "identity.generate", "", "new key "+shortPin(s.desiredPins()[1]))
		}
	case "cancel":
		if err = s.cancelKey(); err == nil {
			s.audit.write(actor(r), clientIP(r), "identity.cancel", "", "new key discarded")
		}
	case "switch":
		missing := s.missingNextKey()
		if len(missing) > 0 && !req.Force {
			http.Error(w, "not all systems have the new key: "+strings.Join(missing, ", "), http.StatusConflict)
			return
		}
		var old, cur string
		if old, cur, err = s.switchKey(); err == nil {
			detail := fmt.Sprintf("%s → %s", shortPin(old), shortPin(cur))
			if len(missing) > 0 {
				detail += "; switched anyway, without: " + strings.Join(missing, ", ")
			}
			s.audit.write(actor(r), clientIP(r), "identity.switch", "", detail)
			log.Printf("key rotation: switched to %s", shortPin(cur))
		}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.st.mu.Lock()
	for _, a := range s.st.st.Agents {
		s.syncPins(a, actor(r))
	}
	s.st.mu.Unlock()
	writeJSON(w, s.identityView())
}

// missingNextKey lists systems that have not confirmed the new key.
func (s *Server) missingNextKey() []string {
	next := s.desiredPins()[1]
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	out := []string{}
	for _, a := range s.st.st.Agents {
		if a.Removing {
			continue
		}
		if next == "" || (a.Pins[0] != next && a.Pins[1] != next) {
			out = append(out, a.Name)
		}
	}
	sort.Strings(out)
	return out
}

func shortPin(p string) string {
	if len(p) > 12 {
		return p[:6] + "…" + p[len(p)-6:]
	}
	return p
}
