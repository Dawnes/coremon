// Package seal is the payload security layer between agent and master.
//
// HTTPS only carries the traffic; agents do not rely on its certificate.
// Instead every exchange is sealed:
//
//   - The master has a payload key pair: ed25519 (signing) + X25519
//     (key agreement). Agents trust it by its public key, in two slots.
//   - Each agent has its own ed25519 key pair (no shared secret).
//   - A request is encrypted to the master's X25519 key with a fresh
//     ephemeral X25519 key (AES-256-GCM, key from HKDF), carries a random
//     nonce and timestamp, and is signed by the agent.
//   - The reply is encrypted with a key derived from the same exchange and
//     signed by the master over the request nonce, so it cannot be replayed
//     or attached to another request.
package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var b64 = base64.StdEncoding

// MaxSkew is how far a request's timestamp may differ from the master's clock.
const MaxSkew = 10 * time.Minute

// ---- master key pair ----

// KeyPair is a master payload key.
type KeyPair struct {
	Sign ed25519.PrivateKey
	Kx   *ecdh.PrivateKey
}

// PublicKey is the public half; its String form is what agents store.
type PublicKey struct {
	Sign ed25519.PublicKey
	Kx   *ecdh.PublicKey
}

// GenerateKeyPair creates a new master payload key.
func GenerateKeyPair() (*KeyPair, error) {
	_, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	kx, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &KeyPair{Sign: sk, Kx: kx}, nil
}

// Public returns the public key.
func (k *KeyPair) Public() PublicKey {
	return PublicKey{Sign: k.Sign.Public().(ed25519.PublicKey), Kx: k.Kx.PublicKey()}
}

// String encodes the public key as "<ed25519>.<x25519>" (base64).
func (p PublicKey) String() string {
	return b64.EncodeToString(p.Sign) + "." + b64.EncodeToString(p.Kx.Bytes())
}

// ParsePublic decodes a public key string.
func ParsePublic(s string) (PublicKey, error) {
	a, b, ok := strings.Cut(strings.TrimSpace(s), ".")
	if !ok {
		return PublicKey{}, errors.New("invalid master key")
	}
	sp, err1 := b64.DecodeString(a)
	kp, err2 := b64.DecodeString(b)
	if err1 != nil || err2 != nil || len(sp) != ed25519.PublicKeySize {
		return PublicKey{}, errors.New("invalid master key")
	}
	kx, err := ecdh.X25519().NewPublicKey(kp)
	if err != nil {
		return PublicKey{}, errors.New("invalid master key")
	}
	return PublicKey{Sign: ed25519.PublicKey(sp), Kx: kx}, nil
}

// Fingerprint is the short form shown to people: base64 sha256 of the key.
func Fingerprint(key string) string {
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("coremon-payload-key/v1\n" + key))
	return b64.EncodeToString(sum[:])
}

type privJSON struct {
	Sign string `json:"sign"`
	Kx   string `json:"kx"`
}

// MarshalPrivate encodes the private key for storage.
func (k *KeyPair) MarshalPrivate() []byte {
	b, _ := json.Marshal(privJSON{Sign: b64.EncodeToString(k.Sign), Kx: b64.EncodeToString(k.Kx.Bytes())})
	return b
}

// ParsePrivate decodes a stored private key.
func ParsePrivate(b []byte) (*KeyPair, error) {
	var p privJSON
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	sk, err1 := b64.DecodeString(p.Sign)
	kx, err2 := b64.DecodeString(p.Kx)
	if err1 != nil || err2 != nil || len(sk) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid payload key file")
	}
	k, err := ecdh.X25519().NewPrivateKey(kx)
	if err != nil {
		return nil, err
	}
	return &KeyPair{Sign: ed25519.PrivateKey(sk), Kx: k}, nil
}

// ---- envelopes ----

// Request is what an agent sends.
type Request struct {
	Agent string `json:"agent"` // agent ID ("" when enrolling)
	Key   string `json:"key"`   // the master public key it is sealed to
	Eph   string `json:"eph"`   // ephemeral X25519 public key
	Nonce string `json:"nonce"` // 16 random bytes
	TS    int64  `json:"ts"`
	CT    string `json:"ct"`  // AES-256-GCM ciphertext
	Sig   string `json:"sig"` // agent's ed25519 signature
}

// Reply is what the master answers.
type Reply struct {
	Key string `json:"key"` // the master key that answered
	CT  string `json:"ct"`
	Sig string `json:"sig"` // master's ed25519 signature
}

func digest(parts ...string) []byte {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return h.Sum(nil)
}

func (r *Request) signed() []byte {
	return digest("coremon/v5 request", r.Agent, r.Key, r.Eph, r.Nonce, strconv.FormatInt(r.TS, 10), r.CT)
}

func replySigned(reqNonce, key, ct string) []byte {
	return digest("coremon/v5 reply", reqNonce, key, ct)
}

func aead(shared []byte, salt, info string) (cipher.AEAD, error) {
	k, err := hkdf.Key(sha256.New, shared, []byte(salt), info, 32)
	if err != nil {
		return nil, err
	}
	blk, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(blk)
}

// Every key is used for exactly one message, so a zero nonce is safe.
var zeroNonce = make([]byte, 12)

// Pending is the agent-side state needed to open the reply.
type Pending struct {
	eph   *ecdh.PrivateKey
	nonce string
	key   PublicKey
	str   string
	trust []string
}

// SealRequest encrypts plain to the master key masterKey, signed by the
// agent's key. trusted are the master keys whose replies are accepted.
func SealRequest(agentID string, agentKey ed25519.PrivateKey, masterKey string, trusted []string, plain []byte) (*Request, *Pending, error) {
	mk, err := ParsePublic(masterKey)
	if err != nil {
		return nil, nil, err
	}
	eph, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	shared, err := eph.ECDH(mk.Kx)
	if err != nil {
		return nil, nil, err
	}
	nb := make([]byte, 16)
	rand.Read(nb)
	r := &Request{Agent: agentID, Key: masterKey, Eph: b64.EncodeToString(eph.PublicKey().Bytes()), Nonce: b64.EncodeToString(nb), TS: time.Now().Unix()}
	g, err := aead(shared, r.Nonce, "coremon/v5 request")
	if err != nil {
		return nil, nil, err
	}
	r.CT = b64.EncodeToString(g.Seal(nil, zeroNonce, plain, []byte(r.Agent)))
	r.Sig = b64.EncodeToString(ed25519.Sign(agentKey, r.signed()))
	return r, &Pending{eph: eph, nonce: r.Nonce, key: mk, str: masterKey, trust: trusted}, nil
}

// Errors from opening a request.
var (
	ErrUnknownKey = errors.New("unknown master key")
	ErrStale      = errors.New("request too old or from the future")
	ErrBadSig     = errors.New("request signature does not verify")
)

// Opened is a decrypted request and what is needed to answer it.
type Opened struct {
	Plain  []byte
	Req    *Request
	shared []byte
	key    *KeyPair
}

// OpenRequest decrypts a request sealed to one of the master's keys.
// agentKey returns the public key that must have signed it (it may look at
// the plaintext, e.g. the key carried in an enrollment).
func OpenRequest(r *Request, keys map[string]*KeyPair, agentKey func(agent string, plain []byte) (ed25519.PublicKey, error)) (*Opened, error) {
	kp := keys[r.Key]
	if kp == nil {
		return nil, ErrUnknownKey
	}
	if d := time.Since(time.Unix(r.TS, 0)); d > MaxSkew || d < -MaxSkew {
		return nil, ErrStale
	}
	ephB, err := b64.DecodeString(r.Eph)
	if err != nil {
		return nil, errors.New("bad request")
	}
	eph, err := ecdh.X25519().NewPublicKey(ephB)
	if err != nil {
		return nil, errors.New("bad request")
	}
	shared, err := kp.Kx.ECDH(eph)
	if err != nil {
		return nil, errors.New("bad request")
	}
	g, err := aead(shared, r.Nonce, "coremon/v5 request")
	if err != nil {
		return nil, err
	}
	ct, err := b64.DecodeString(r.CT)
	if err != nil {
		return nil, errors.New("bad request")
	}
	plain, err := g.Open(nil, zeroNonce, ct, []byte(r.Agent))
	if err != nil {
		return nil, errors.New("request does not decrypt")
	}
	pub, err := agentKey(r.Agent, plain)
	if err != nil {
		return nil, err
	}
	sig, err := b64.DecodeString(r.Sig)
	if err != nil || !ed25519.Verify(pub, r.signed(), sig) {
		return nil, ErrBadSig
	}
	return &Opened{Plain: plain, Req: r, shared: shared, key: kp}, nil
}

// SealReply encrypts and signs the answer to an opened request.
func (o *Opened) SealReply(plain []byte) (*Reply, error) {
	g, err := aead(o.shared, o.Req.Nonce, "coremon/v5 reply")
	if err != nil {
		return nil, err
	}
	rep := &Reply{Key: o.Req.Key, CT: b64.EncodeToString(g.Seal(nil, zeroNonce, plain, []byte(o.Req.Nonce)))}
	rep.Sig = b64.EncodeToString(ed25519.Sign(o.key.Sign, replySigned(o.Req.Nonce, rep.Key, rep.CT)))
	return rep, nil
}

// OpenReply verifies and decrypts the master's answer to this request.
func (p *Pending) OpenReply(rep *Reply) ([]byte, error) {
	if rep.Key != p.str {
		return nil, fmt.Errorf("reply from a different master key")
	}
	trusted := false
	for _, k := range p.trust {
		if k != "" && k == rep.Key {
			trusted = true
		}
	}
	if !trusted {
		return nil, errors.New("reply from an untrusted master key")
	}
	sig, err := b64.DecodeString(rep.Sig)
	if err != nil || !ed25519.Verify(p.key.Sign, replySigned(p.nonce, rep.Key, rep.CT), sig) {
		return nil, errors.New("reply signature does not verify")
	}
	mk, _ := ParsePublic(rep.Key)
	shared, err := p.eph.ECDH(mk.Kx)
	if err != nil {
		return nil, err
	}
	g, err := aead(shared, p.nonce, "coremon/v5 reply")
	if err != nil {
		return nil, err
	}
	ct, err := b64.DecodeString(rep.CT)
	if err != nil {
		return nil, errors.New("bad reply")
	}
	plain, err := g.Open(nil, zeroNonce, ct, []byte(p.nonce))
	if err != nil {
		return nil, errors.New("reply does not decrypt")
	}
	return plain, nil
}

// ---- agent key ----

// AgentKey encodes an agent's ed25519 public key.
func AgentKey(pub ed25519.PublicKey) string { return b64.EncodeToString(pub) }

// ParseAgentKey decodes an agent public key.
func ParseAgentKey(s string) (ed25519.PublicKey, error) {
	b, err := b64.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("invalid agent key")
	}
	return ed25519.PublicKey(b), nil
}
