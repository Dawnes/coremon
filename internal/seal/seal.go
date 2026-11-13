// Package seal encrypts and signs every exchange between agent and master.
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

const MaxSkew = 10 * time.Minute

type KeyPair struct {
	Sign ed25519.PrivateKey
	Kx   *ecdh.PrivateKey
}

type PublicKey struct {
	Sign ed25519.PublicKey
	Kx   *ecdh.PublicKey
}

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

func (k *KeyPair) Public() PublicKey {
	return PublicKey{Sign: k.Sign.Public().(ed25519.PublicKey), Kx: k.Kx.PublicKey()}
}

func (p PublicKey) String() string {
	return b64.EncodeToString(p.Sign) + "." + b64.EncodeToString(p.Kx.Bytes())
}

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

func (k *KeyPair) MarshalPrivate() []byte {
	b, _ := json.Marshal(privJSON{Sign: b64.EncodeToString(k.Sign), Kx: b64.EncodeToString(k.Kx.Bytes())})
	return b
}

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

type Request struct {
	Agent string `json:"agent"`
	Key   string `json:"key"`
	Eph   string `json:"eph"`
	Nonce string `json:"nonce"`
	TS    int64  `json:"ts"`
	CT    string `json:"ct"`
	Sig   string `json:"sig"`
}

type Reply struct {
	Key string `json:"key"`
	CT  string `json:"ct"`
	Sig string `json:"sig"`
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

var zeroNonce = make([]byte, 12)

type Pending struct {
	eph   *ecdh.PrivateKey
	nonce string
	key   PublicKey
	str   string
	trust []string
}

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

var (
	ErrUnknownKey = errors.New("unknown master key")
	ErrStale      = errors.New("request too old or from the future")
	ErrBadSig     = errors.New("request signature does not verify")
)

type Opened struct {
	Plain  []byte
	Req    *Request
	shared []byte
	key    *KeyPair
}

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

func (o *Opened) SealReply(plain []byte) (*Reply, error) {
	g, err := aead(o.shared, o.Req.Nonce, "coremon/v5 reply")
	if err != nil {
		return nil, err
	}
	rep := &Reply{Key: o.Req.Key, CT: b64.EncodeToString(g.Seal(nil, zeroNonce, plain, []byte(o.Req.Nonce)))}
	rep.Sig = b64.EncodeToString(ed25519.Sign(o.key.Sign, replySigned(o.Req.Nonce, rep.Key, rep.CT)))
	return rep, nil
}

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

func AgentKey(pub ed25519.PublicKey) string { return b64.EncodeToString(pub) }

func ParseAgentKey(s string) (ed25519.PublicKey, error) {
	b, err := b64.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("invalid agent key")
	}
	return ed25519.PublicKey(b), nil
}
