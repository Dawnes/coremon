package master

// Master keys.
//
// The HTTPS certificate (tls.crt/tls.key) only carries traffic; it is not
// rotated here and agents 0.3.0+ do not rely on it. Agents trust the
// master's payload key (payload.key), in two slots, which allows rotation:
//
//  1. Generate: a new payload key; the master keeps answering with the old.
//  2. Distribute: every agent is told, in a reply signed with a key it
//     trusts, to trust [current, new]. Agents report what they stored.
//  3. Switch: the master answers with the new key and deletes the old one;
//     agents are told to keep only [new].
//  Cancel before the switch discards the new key; agents drop it again.

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"coremon/internal/proto"
	"coremon/internal/seal"
)

// keyState holds the HTTPS certificate and the payload keys.
type keyState struct {
	cert    *tls.Certificate // HTTPS
	tlsPin  string           // its fingerprint; agents before 0.3.0 pin it
	cur     *seal.KeyPair    // payload key in use
	curStr  string
	next    *seal.KeyPair // during a rotation
	nextStr string
	nextAt  int64
}

func (s *Server) payloadPath(slot string) string {
	if slot == "next" {
		return filepath.Join(s.cfg.DataDir, "payload-next.key")
	}
	return filepath.Join(s.cfg.DataDir, "payload.key")
}

func (s *Server) currentCert(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	return s.keys.cert, nil
}

// currentPin is the HTTPS certificate fingerprint (agents before 0.3.0).
func (s *Server) currentPin() string {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	return s.keys.tlsPin
}

// desiredPins is what every agent should trust: [payload key, new key].
func (s *Server) desiredPins() [2]string {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	return [2]string{s.keys.curStr, s.keys.nextStr}
}

// payloadKeys are the keys requests may be sealed to.
func (s *Server) payloadKeys() map[string]*seal.KeyPair {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	m := map[string]*seal.KeyPair{s.keys.curStr: s.keys.cur}
	if s.keys.next != nil {
		m[s.keys.nextStr] = s.keys.next
	}
	return m
}

func readPayloadKey(path string) (*seal.KeyPair, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return seal.ParsePrivate(b)
}

func writePayloadKey(path string, k *seal.KeyPair) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, k.MarshalPrivate(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// loadKeys loads (or on first start creates) the payload key, picks up a
// rotation in progress, and retires files of the old HTTPS-key rotation.
func (s *Server) loadKeys() error {
	cur, err := readPayloadKey(s.payloadPath("current"))
	if os.IsNotExist(err) {
		if cur, err = seal.GenerateKeyPair(); err == nil {
			err = writePayloadKey(s.payloadPath("current"), cur)
		}
	}
	if err != nil {
		return fmt.Errorf("payload key: %w", err)
	}
	s.keys.cur, s.keys.curStr = cur, cur.Public().String()
	if next, err := readPayloadKey(s.payloadPath("next")); err == nil {
		st, _ := os.Stat(s.payloadPath("next"))
		s.keys.next, s.keys.nextStr, s.keys.nextAt = next, next.Public().String(), st.ModTime().Unix()
	}
	for _, f := range []string{"tls-next.crt", "tls-next.key"} {
		os.Remove(filepath.Join(s.cfg.DataDir, f))
	}
	return nil
}

func (s *Server) generateKey() error {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	if s.keys.next != nil {
		return errors.New("a new key was already generated; switch to it or cancel first")
	}
	k, err := seal.GenerateKeyPair()
	if err != nil {
		return err
	}
	if err := writePayloadKey(s.payloadPath("next"), k); err != nil {
		return err
	}
	s.keys.next, s.keys.nextStr, s.keys.nextAt = k, k.Public().String(), time.Now().Unix()
	return nil
}

func (s *Server) cancelKey() error {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	if s.keys.next == nil {
		return errors.New("no new key to cancel")
	}
	os.Remove(s.payloadPath("next"))
	s.keys.next, s.keys.nextStr, s.keys.nextAt = nil, "", 0
	return nil
}

// switchKey makes the new payload key current and deletes the old one.
func (s *Server) switchKey() (old, cur string, err error) {
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	if s.keys.next == nil {
		return "", "", errors.New("generate a new key first")
	}
	if err := os.Rename(s.payloadPath("next"), s.payloadPath("current")); err != nil {
		return "", "", err
	}
	old = s.keys.curStr
	s.keys.cur, s.keys.curStr = s.keys.next, s.keys.nextStr
	s.keys.next, s.keys.nextStr, s.keys.nextAt = nil, "", 0
	return old, s.keys.curStr, nil
}

// syncPins tells an agent which payload keys to trust, until it reports
// exactly that. Caller holds s.st.mu.
func (s *Server) syncPins(a *Agent, actor string) *Job {
	if a.Protocol < 5 || a.Removing {
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
	Sealed     bool      `json:"sealed"` // uses the payload key (0.3.0+)
	Slots      [2]string `json:"slots"`  // per slot: current | new | unknown | "" (empty); no fingerprints
	ReportedAt int64     `json:"reported_at"`
	HasNext    bool      `json:"has_next"`
}

// slotStatus says what a key an agent trusts is to this master.
func slotStatus(k, cur, next string) string {
	switch {
	case k == "":
		return ""
	case k == cur:
		return "current"
	case next != "" && k == next:
		return "new"
	}
	return "unknown"
}

// identityView is the Identity page. full: with the key fingerprints
// (administrators); read-only users only see the status per system.
func (s *Server) identityView(full bool) map[string]any {
	s.keyMu.Lock()
	cur, next, nextAt, tlsPin := s.keys.curStr, s.keys.nextStr, s.keys.nextAt, s.keys.tlsPin
	s.keyMu.Unlock()
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	now := time.Now().Unix()
	list := []agentKeys{}
	for _, a := range s.st.st.Agents {
		k := agentKeys{ID: a.ID, Name: a.Name, Status: a.Status, Online: s.online(a, now), LastSeen: a.LastSeen,
			Version: a.CoreVersion, Sealed: a.Protocol >= 5}
		if k.Sealed {
			k.Slots = [2]string{slotStatus(a.Pins[0], cur, next), slotStatus(a.Pins[1], cur, next)}
			k.ReportedAt = a.PinsAt
			k.HasNext = next != "" && (a.Pins[0] == next || a.Pins[1] == next)
		}
		list = append(list, k)
	}
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
	v := map[string]any{"rotating": next != "", "next_at": nextAt, "agents": list, "full": full}
	if full {
		v["current"], v["next"], v["https"] = seal.Fingerprint(cur), seal.Fingerprint(next), tlsPin
	}
	return v
}

func (s *Server) handleIdentity(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.identityView(can(userFrom(r), permChange)))
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
			s.audit.write(actor(r), clientIP(r), "identity.generate", "", "new payload key "+shortPin(seal.Fingerprint(s.desiredPins()[1])))
		}
	case "cancel":
		if err = s.cancelKey(); err == nil {
			s.audit.write(actor(r), clientIP(r), "identity.cancel", "", "new payload key discarded")
		}
	case "switch":
		missing := s.missingNextKey()
		if len(missing) > 0 && !req.Force {
			http.Error(w, "not all systems have the new key: "+strings.Join(missing, ", "), http.StatusConflict)
			return
		}
		var old, cur string
		if old, cur, err = s.switchKey(); err == nil {
			detail := fmt.Sprintf("%s → %s", shortPin(seal.Fingerprint(old)), shortPin(seal.Fingerprint(cur)))
			if len(missing) > 0 {
				detail += "; switched anyway, without: " + strings.Join(missing, ", ")
			}
			s.audit.write(actor(r), clientIP(r), "identity.switch", "", detail)
			log.Printf("key rotation: switched to payload key %s", shortPin(seal.Fingerprint(cur)))
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
		s.wake(a.ID)
	}
	s.st.mu.Unlock()
	writeJSON(w, s.identityView(true)) // administrators only
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
		if next == "" || a.Protocol < 5 || (a.Pins[0] != next && a.Pins[1] != next) {
			out = append(out, a.Name)
		}
	}
	sort.Strings(out)
	return out
}

// shortPin is the short form of a fingerprint shown to people: the first
// five characters.
func shortPin(p string) string {
	if len(p) > 5 {
		return p[:5] + "…"
	}
	return p
}
