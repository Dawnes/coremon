package master

// The website's application code is only for signed-in users. It is
// served under a name with a fingerprint of its contents, which the
// session tells signed-in users only.

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
)

const appScript = "app.js"

var appNameOnce = sync.OnceValue(func() string {
	b, _ := webFS.ReadFile("web/" + appScript)
	h := sha256.Sum256(b)
	return "app-" + hex.EncodeToString(h[:6]) + ".js"
})

func (s *Server) appName() string { return appNameOnce() }

// signedIn returns a copy of the user of the request's session, or nil.
func (s *Server) signedIn(r *http.Request) *User {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	uid := s.sess.userOf(c.Value)
	if uid == "" {
		return nil
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	u := s.st.userByID(uid)
	if u == nil || u.Locked {
		return nil
	}
	uc := *u
	return &uc
}
