package master

// Last and previous sign-in per user, with the IP address it came from.

import (
	"strings"
	"time"
)

// signedInNow records a completed sign-in: password, and the code with 2FA.
func (s *Server) signedInNow(uid, ip string) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	u := s.st.userByID(uid)
	if u == nil {
		return
	}
	u.PrevLogin, u.PrevIP = u.LastLogin, u.LastIP
	u.LastLogin, u.LastIP = time.Now().Unix(), ip
	s.st.markDirty()
}

// fillLoginsFromAudit runs once: it takes the last two sign-ins of each
// user from the audit log, for users who signed in before this was stored.
func (s *Server) fillLoginsFromAudit() {
	s.st.mu.Lock()
	done := s.st.st.LoginsFilled
	s.st.mu.Unlock()
	if done {
		return
	}
	logins := map[string][]auditEntry{}
	for _, e := range s.audit.read("", "") { // newest first
		if e.Action == "login" {
			k := strings.ToLower(e.Actor)
			if len(logins[k]) < 2 {
				logins[k] = append(logins[k], e)
			}
		}
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	for _, u := range s.st.st.Users {
		l := logins[strings.ToLower(u.Name)]
		if u.LastLogin != 0 || len(l) == 0 {
			continue
		}
		u.LastLogin, u.LastIP = l[0].Time, l[0].IP
		if len(l) > 1 {
			u.PrevLogin, u.PrevIP = l[1].Time, l[1].IP
		}
	}
	s.st.st.LoginsFilled = true
	s.st.markDirty()
}
