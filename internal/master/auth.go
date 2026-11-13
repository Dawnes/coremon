package master

// Sessions and login limits.

import (
	"context"
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

var dummyHash = hashPassword(randHex(8))

type session struct {
	user string
	exp  int64
}

type sessions struct {
	mu   sync.Mutex
	m    map[string]session
	fail map[string][]int64
}

const sessionTTL = 12 * time.Hour
const cookieName = "cm_session"

func newSessions() *sessions {
	return &sessions{m: map[string]session{}, fail: map[string][]int64{}}
}

func (s *sessions) create(userID string) string {
	tok := randHex(32)
	s.mu.Lock()
	s.m[sha256hex(tok)] = session{user: userID, exp: time.Now().Add(sessionTTL).Unix()}
	s.mu.Unlock()
	return tok
}

func (s *sessions) userOf(tok string) string {
	if tok == "" {
		return ""
	}
	h := sha256hex(tok)
	s.mu.Lock()
	defer s.mu.Unlock()
	se, ok := s.m[h]
	now := time.Now()
	if !ok || now.Unix() > se.exp {
		delete(s.m, h)
		return ""
	}
	se.exp = now.Add(sessionTTL).Unix()
	s.m[h] = se
	return se.user
}

func (s *sessions) destroy(tok string) {
	s.mu.Lock()
	delete(s.m, sha256hex(tok))
	s.mu.Unlock()
}

func (s *sessions) destroyUserExcept(userID, keepTok string) {
	keep := sha256hex(keepTok)
	s.mu.Lock()
	for h, se := range s.m {
		if se.user == userID && h != keep {
			delete(s.m, h)
		}
	}
	s.mu.Unlock()
}

func (s *sessions) destroyUser(userID string) {
	s.mu.Lock()
	for h, se := range s.m {
		if se.user == userID {
			delete(s.m, h)
		}
	}
	s.mu.Unlock()
}

const (
	loginFails  = 5
	loginWindow = 15 * time.Minute
	hashAtOnce  = 4
)

func (s *sessions) reserveLogin(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cut := time.Now().Add(-loginWindow).Unix()
	var keep []int64
	for _, t := range s.fail[ip] {
		if t > cut {
			keep = append(keep, t)
		}
	}
	if len(keep) >= loginFails {
		s.fail[ip] = keep
		return false
	}
	s.fail[ip] = append(keep, time.Now().Unix())
	return true
}

func (s *sessions) forgiveLogin(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l := s.fail[ip]; len(l) > 1 {
		s.fail[ip] = l[:len(l)-1]
	} else {
		delete(s.fail, ip)
	}
}

func (s *sessions) sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	cut := now.Add(-loginWindow).Unix()
	for ip, l := range s.fail {
		if len(l) == 0 || l[len(l)-1] <= cut {
			delete(s.fail, ip)
		}
	}
	for h, se := range s.m {
		if now.Unix() > se.exp {
			delete(s.m, h)
		}
	}
}

func clientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(proxiedIPKey{}).(string); ok {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

type proxiedIPKey struct{}

func viaLocalProxy(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			if xff := r.Header.Values("X-Forwarded-For"); len(xff) > 0 {
				parts := strings.Split(xff[len(xff)-1], ",")
				if fwd := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); fwd != nil {
					r = r.WithContext(context.WithValue(r.Context(), proxiedIPKey{}, fwd.String()))
				}
			}
		}
		h.ServeHTTP(w, r)
	})
}
