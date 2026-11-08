package seal

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"
)

func setup(t *testing.T) (*KeyPair, string, ed25519.PublicKey, ed25519.PrivateKey) {
	mk, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	apub, apriv, _ := ed25519.GenerateKey(rand.Reader)
	return mk, mk.Public().String(), apub, apriv
}

func TestRoundTrip(t *testing.T) {
	mk, mkStr, apub, apriv := setup(t)
	keys := map[string]*KeyPair{mkStr: mk}
	req, pend, err := SealRequest("a_1", apriv, mkStr, []string{mkStr, ""}, []byte(`{"op":"poll"}`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(req.CT, "poll") {
		t.Fatal("request not encrypted")
	}
	o, err := OpenRequest(req, keys, func(string, []byte) (ed25519.PublicKey, error) { return apub, nil })
	if err != nil || string(o.Plain) != `{"op":"poll"}` {
		t.Fatalf("open: %v %s", err, o.Plain)
	}
	rep, _ := o.SealReply([]byte(`{"commands":[]}`))
	plain, err := pend.OpenReply(rep)
	if err != nil || string(plain) != `{"commands":[]}` {
		t.Fatalf("reply: %v %s", err, plain)
	}
	k2, err := ParsePrivate(mk.MarshalPrivate())
	if err != nil || k2.Public().String() != mkStr {
		t.Fatal("private key round trip")
	}
}

func TestAttacks(t *testing.T) {
	mk, mkStr, apub, apriv := setup(t)
	keys := map[string]*KeyPair{mkStr: mk}
	agentKey := func(string, []byte) (ed25519.PublicKey, error) { return apub, nil }

	// Tampered request.
	req, _, _ := SealRequest("a_1", apriv, mkStr, []string{mkStr}, []byte("hello"))
	bad := *req
	bad.Agent = "a_2"
	if _, err := OpenRequest(&bad, keys, agentKey); err == nil {
		t.Error("request with a changed agent must fail")
	}
	// Signed by someone else.
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	forged, _, _ := SealRequest("a_1", other, mkStr, []string{mkStr}, []byte("hello"))
	if _, err := OpenRequest(forged, keys, agentKey); err != ErrBadSig {
		t.Errorf("forged agent signature: %v", err)
	}
	// Old request.
	old := *req
	old.TS = time.Now().Add(-time.Hour).Unix()
	if _, err := OpenRequest(&old, keys, agentKey); err != ErrStale {
		t.Errorf("stale request: %v", err)
	}
	// Sealed to a key the master does not have.
	mk2, _ := GenerateKeyPair()
	wrong, _, _ := SealRequest("a_1", apriv, mk2.Public().String(), nil, []byte("x"))
	if _, err := OpenRequest(wrong, keys, agentKey); err != ErrUnknownKey {
		t.Errorf("unknown key: %v", err)
	}

	// Replies: a reply for one request cannot answer another (replay).
	r1, p1, _ := SealRequest("a_1", apriv, mkStr, []string{mkStr}, []byte("one"))
	r2, p2, _ := SealRequest("a_1", apriv, mkStr, []string{mkStr}, []byte("two"))
	o1, _ := OpenRequest(r1, keys, agentKey)
	OpenRequest(r2, keys, agentKey)
	rep1, _ := o1.SealReply([]byte(`{"commands":[{"type":"action"}]}`))
	if _, err := p2.OpenReply(rep1); err == nil {
		t.Error("a reply must not be accepted for a different request")
	}
	if _, err := p1.OpenReply(rep1); err != nil {
		t.Errorf("genuine reply: %v", err)
	}
	// Tampered reply.
	t1 := *rep1
	t1.CT = t1.CT[:len(t1.CT)-4] + "AAA="
	if _, err := p1.OpenReply(&t1); err == nil {
		t.Error("tampered reply must fail")
	}
	// A reply signed by a key the agent does not trust: an attacker's own
	// "master" answering a request sealed to its own key.
	evil, _ := GenerateKeyPair()
	evilStr := evil.Public().String()
	r3, p3, _ := SealRequest("a_1", apriv, evilStr, []string{mkStr}, []byte("x"))
	o3, _ := OpenRequest(r3, map[string]*KeyPair{evilStr: evil}, agentKey)
	rep3, _ := o3.SealReply([]byte("{}"))
	if _, err := p3.OpenReply(rep3); err == nil {
		t.Error("reply from an untrusted key must fail")
	}
}
