// Package sign implements ed25519 signing of release artifacts.
//
// The signing private key lives on the build machine (not on the master).
// Agents pin the public key at enrollment and refuse any binary whose
// signature does not verify, so even a compromised master cannot push
// code that was not signed by you.
package sign

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Message builds the exact byte string that is signed for an artifact.
func Message(kind, name, version, goos, goarch, sha256hex string) []byte {
	return []byte(fmt.Sprintf("coremon/v1\n%s\n%s\n%s\n%s/%s\n%s",
		kind, name, version, goos, goarch, strings.ToLower(sha256hex)))
}

// FileSHA256 returns the hex sha256 of a file.
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// GenerateKey creates a new key pair, returned base64 encoded.
func GenerateKey() (pub, priv string, err error) {
	p, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(p), base64.StdEncoding.EncodeToString(k), nil
}

// ParsePublic decodes a base64 public key.
func ParsePublic(s string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("invalid ed25519 public key")
	}
	return ed25519.PublicKey(b), nil
}

// ParsePrivate decodes a base64 private key.
func ParsePrivate(s string) (ed25519.PrivateKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid ed25519 private key")
	}
	return ed25519.PrivateKey(b), nil
}

// Sign signs msg and returns a base64 signature.
func Sign(priv ed25519.PrivateKey, msg []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg))
}

// Verify checks a base64 signature.
func Verify(pub ed25519.PublicKey, msg []byte, sigB64 string) bool {
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigB64))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pub, msg, sig)
}
