package master

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const pbkdfIter = 600_000

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// hashPassword returns "pbkdf2-sha256$iter$salt$hash".
func hashPassword(pw string) string {
	salt := make([]byte, 16)
	_, _ = rand.Read(salt)
	k, err := pbkdf2.Key(sha256.New, pw, salt, pbkdfIter, 32)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdfIter,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(k))
}

func checkPassword(stored, pw string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1000 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

// sessions are kept in memory; a master restart logs the user out.
type sessions struct {
	mu   sync.Mutex
	m    map[string]int64 // token hash -> expiry
	fail map[string][]int64
}

const sessionTTL = 12 * time.Hour
const cookieName = "cm_session"

func newSessions() *sessions {
	return &sessions{m: map[string]int64{}, fail: map[string][]int64{}}
}

func (s *sessions) create() string {
	tok := randHex(32)
	s.mu.Lock()
	s.m[sha256hex(tok)] = time.Now().Add(sessionTTL).Unix()
	s.mu.Unlock()
	return tok
}

func (s *sessions) valid(tok string) bool {
	if tok == "" {
		return false
	}
	h := sha256hex(tok)
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.m[h]
	now := time.Now()
	if !ok || now.Unix() > exp {
		delete(s.m, h)
		return false
	}
	s.m[h] = now.Add(sessionTTL).Unix() // sliding expiry
	return true
}

func (s *sessions) destroy(tok string) {
	s.mu.Lock()
	delete(s.m, sha256hex(tok))
	s.mu.Unlock()
}

func (s *sessions) destroyAll() {
	s.mu.Lock()
	s.m = map[string]int64{}
	s.mu.Unlock()
}

// allowLogin rate-limits failed logins per client IP: 5 per 15 minutes.
func (s *sessions) allowLogin(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cut := time.Now().Add(-15 * time.Minute).Unix()
	var keep []int64
	for _, t := range s.fail[ip] {
		if t > cut {
			keep = append(keep, t)
		}
	}
	s.fail[ip] = keep
	return len(keep) < 5
}

func (s *sessions) recordFail(ip string) {
	s.mu.Lock()
	s.fail[ip] = append(s.fail[ip], time.Now().Unix())
	s.mu.Unlock()
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
