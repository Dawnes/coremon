package master

// Website users, roles and sign-in.

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

var userNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)

type perm int

const (
	permView perm = iota
	permChange
)

var rolePerms = map[string]map[perm]bool{
	RoleAdmin:  {permView: true, permChange: true},
	RoleViewer: {permView: true},
}

func can(u *User, p perm) bool { return u != nil && rolePerms[u.Role][p] }

type ctxKey struct{}

func userFrom(r *http.Request) *User {
	u, _ := r.Context().Value(ctxKey{}).(*User)
	return u
}

func actor(r *http.Request) string {
	if u := userFrom(r); u != nil {
		return u.Name
	}
	return "?"
}

func (s *Server) requireUI(p perm, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Coremon") != "1" {
			http.Error(w, "missing X-Coremon header", http.StatusForbidden)
			return
		}
		c, err := r.Cookie(cookieName)
		uid := ""
		if err == nil {
			uid = s.sess.userOf(c.Value)
		}
		s.st.mu.Lock()
		u := s.st.userByID(uid)
		var uc User
		if u != nil {
			uc = *u
		}
		s.st.mu.Unlock()
		if u == nil || uc.Locked {
			http.Error(w, "not logged in", http.StatusUnauthorized)
			return
		}
		if uc.MustChange && r.URL.Path != "/api/ui/password" {
			http.Error(w, "password change required", http.StatusForbidden)
			return
		}
		if !can(&uc, p) {
			http.Error(w, "your account is read-only", http.StatusForbidden)
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, &uc)))
	}
}

type userView struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	Created    int64  `json:"created"`
	Permanent  bool   `json:"permanent"`
	MustChange bool   `json:"must_change"`
	Locked     bool   `json:"locked"`
	TwoFA      bool   `json:"two_fa"`
	TwoFASince int64  `json:"two_fa_since,omitempty"`
}

func viewUser(u *User) userView {
	return userView{ID: u.ID, Name: u.Name, Role: u.Role, Created: u.Created, Permanent: u.Name == adminName, MustChange: u.MustChange, Locked: u.Locked,
		TwoFA: u.TOTPSecret != "", TwoFASince: u.TOTPSince}
}

func (s *Server) setCookie(w http.ResponseWriter, tok string) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: tok, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"authenticated": false, "min_password": minPasswordLen, "max_password": maxPasswordLen}

	_, statErr := os.Stat(s.initialPwPath())
	s.st.mu.Lock()
	out["initial_setup"] = statErr == nil && s.st.userByID(s.st.st.InitialUser) != nil
	s.st.mu.Unlock()
	if c, err := r.Cookie(cookieName); err == nil {
		if uid := s.sess.userOf(c.Value); uid != "" {
			s.st.mu.Lock()
			if u := s.st.userByID(uid); u != nil {
				_, statErr := os.Stat(s.initialPwPath())
				out["authenticated"] = true
				out["version"], out["build_date"] = s.cfg.Version, s.cfg.BuildDate
				out["user"] = viewUser(u)
				out["can_change"] = can(u, permChange)
				out["initial_password"] = statErr == nil && u.ID == s.st.st.InitialUser
				out["must_change"] = u.MustChange
			}
			s.st.mu.Unlock()
		}
	}
	writeJSON(w, out)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Coremon") != "1" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	ip := clientIP(r)
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}

	// Counted before the password check, so parallel attempts cannot bypass the limit.
	if !s.sess.reserveLogin(ip) {
		http.Error(w, "too many failed attempts; try again later", http.StatusTooManyRequests)
		return
	}

	select {
	case s.hashSem <- struct{}{}:
		defer func() { <-s.hashSem }()
	case <-time.After(5 * time.Second):
		s.sess.forgiveLogin(ip)
		w.Header().Set("Retry-After", "5")
		http.Error(w, "busy, try again", http.StatusServiceUnavailable)
		return
	case <-r.Context().Done():
		s.sess.forgiveLogin(ip)
		return
	}
	name := strings.ToLower(strings.TrimSpace(req.Username))
	if len(name) > 32 {
		name = name[:32]
	}
	s.st.mu.Lock()
	u := s.st.userByName(name)
	hash, uid, locked := dummyHash, "", false
	if u != nil {
		hash, uid, name, locked = u.PasswordHash, u.ID, u.Name, u.Locked
	}
	s.st.mu.Unlock()
	if !checkPassword(hash, req.Password) || uid == "" {
		s.audit.write(cleanText(name, 32), ip, "login.failed", "", "")
		time.Sleep(400 * time.Millisecond)
		http.Error(w, "wrong user name or password", http.StatusUnauthorized)
		return
	}

	// Only told with the right password: reveals nothing about accounts.
	if locked {
		s.audit.write(name, ip, "login.locked", "", "account is locked")
		time.Sleep(400 * time.Millisecond)
		http.Error(w, "Your account is locked", http.StatusForbidden)
		return
	}
	s.sess.forgiveLogin(ip)
	s.st.mu.Lock()
	twoFA := u.TOTPSecret != ""
	s.st.mu.Unlock()
	if twoFA {
		writeJSON(w, map[string]any{"code_required": true, "ticket": s.tfa.newTicket(uid, ip, time.Now())})
		return
	}
	s.setCookie(w, s.sess.create(uid))
	s.audit.write(name, ip, "login", "", "")
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.sess.destroy(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	if msg := passwordProblem(req.New); msg != "" {
		http.Error(w, "new "+msg, http.StatusBadRequest)
		return
	}
	me := userFrom(r)
	s.st.mu.Lock()
	u := s.st.userByID(me.ID)
	ok := u != nil && checkPassword(u.PasswordHash, req.Current)
	s.st.mu.Unlock()
	if !ok {
		http.Error(w, "current password is wrong", http.StatusForbidden)
		return
	}
	if u.MustChange && req.New == req.Current {
		http.Error(w, "choose a new password, not the generated one", http.StatusBadRequest)
		return
	}
	h := hashPassword(req.New)
	s.st.mu.Lock()
	u.PasswordHash, u.MustChange = h, false
	if u.ID == s.st.st.InitialUser {
		os.Remove(s.initialPwPath())
	}
	s.st.markDirty()
	s.st.mu.Unlock()
	s.sess.destroyUser(u.ID)
	s.setCookie(w, s.sess.create(u.ID))
	s.audit.write(me.Name, clientIP(r), "password.changed", me.Name, "")
	writeJSON(w, map[string]bool{"ok": true})
}

func passwordProblem(pw string) string {
	if n := utf8.RuneCountInString(pw); n < minPasswordLen || n > maxPasswordLen || !utf8.ValidString(pw) {
		return fmt.Sprintf("password must be %d-%d characters", minPasswordLen, maxPasswordLen)
	}
	return ""
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	out := []userView{}
	for _, u := range s.st.st.Users {
		out = append(out, viewUser(u))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, out)
}

func validRole(r string) bool { _, ok := rolePerms[r]; return ok }

func (s *Server) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(req.Name))
	switch {
	case !userNameRe.MatchString(name):
		http.Error(w, "user name: 1-32 characters, lowercase letters, digits, . _ -", http.StatusBadRequest)
		return
	case !validRole(req.Role):
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	case passwordProblem(req.Password) != "":
		http.Error(w, passwordProblem(req.Password), http.StatusBadRequest)
		return
	}
	h := hashPassword(req.Password)
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	if s.st.userByName(name) != nil {
		http.Error(w, "a user with this name exists", http.StatusConflict)
		return
	}
	u := &User{ID: "u_" + randHex(6), Name: name, Role: req.Role, PasswordHash: h, Created: time.Now().Unix()}
	s.st.st.Users = append(s.st.st.Users, u)
	s.st.markDirty()
	s.audit.write(actor(r), clientIP(r), "user.created", name, "role "+req.Role)
	writeJSON(w, viewUser(u))
}

func (s *Server) handleUserUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role     *string `json:"role"`
		Password *string `json:"password"`
		Locked   *bool   `json:"locked"`
		TwoFA    *bool   `json:"two_fa"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	if req.Locked != nil && (req.Role != nil || req.Password != nil) {
		http.Error(w, "lock or unlock on its own", http.StatusBadRequest)
		return
	}
	if req.TwoFA != nil {
		s.adminTurnOff2FA(w, r, *req.TwoFA)
		return
	}
	if req.Password != nil && passwordProblem(*req.Password) != "" {
		http.Error(w, passwordProblem(*req.Password), http.StatusBadRequest)
		return
	}
	var h string
	if req.Password != nil {
		h = hashPassword(*req.Password)
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	u := s.st.userByID(r.PathValue("id"))
	if u == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	signOut := false
	if req.Locked != nil && *req.Locked != u.Locked {
		if u.ID == userFrom(r).ID {
			http.Error(w, "you cannot lock your own account", http.StatusConflict)
			return
		}
		// At least one administrator must always be able to sign in.
		if *req.Locked && s.st.lastActiveAdmin(u) {
			http.Error(w, "this is the last administrator who can sign in; it cannot be locked", http.StatusConflict)
			return
		}
		u.Locked = *req.Locked
		if u.Locked {
			s.audit.write(actor(r), clientIP(r), "user.locked", u.Name, "")
			signOut = true
		} else {
			s.audit.write(actor(r), clientIP(r), "user.unlocked", u.Name, "")
		}
	}
	if req.Role != nil && *req.Role != u.Role {
		if !validRole(*req.Role) {
			http.Error(w, "invalid role", http.StatusBadRequest)
			return
		}
		if u.Name == adminName {
			http.Error(w, "the admin account is permanent and stays administrator", http.StatusConflict)
			return
		}
		if s.st.lastActiveAdmin(u) {
			http.Error(w, "this is the last administrator who can sign in; make someone else administrator (or unlock one) first", http.StatusConflict)
			return
		}
		s.audit.write(actor(r), clientIP(r), "user.role", u.Name, u.Role+" → "+*req.Role)
		u.Role, signOut = *req.Role, true
	}
	if req.Password != nil {
		u.PasswordHash, u.MustChange = h, false
		if u.ID == s.st.st.InitialUser {
			os.Remove(s.initialPwPath())
		}
		s.audit.write(actor(r), clientIP(r), "user.password_reset", u.Name, "")
		signOut = true
	}
	s.st.markDirty()
	if signOut && u.ID != userFrom(r).ID {
		s.sess.destroyUser(u.ID)
	}
	writeJSON(w, viewUser(u))
}

func (s *Server) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	u := s.st.userByID(r.PathValue("id"))
	if u == nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	if u.ID == userFrom(r).ID {
		http.Error(w, "you cannot remove your own account", http.StatusConflict)
		return
	}
	if u.Name == adminName {
		http.Error(w, "the admin account is permanent and cannot be removed", http.StatusConflict)
		return
	}
	if s.st.lastActiveAdmin(u) {
		http.Error(w, "this is the last administrator who can sign in", http.StatusConflict)
		return
	}
	keep := s.st.st.Users[:0]
	for _, x := range s.st.st.Users {
		if x.ID != u.ID {
			keep = append(keep, x)
		}
	}
	s.st.st.Users = keep
	s.st.markDirty()
	s.sess.destroyUser(u.ID)
	s.audit.write(actor(r), clientIP(r), "user.removed", u.Name, "")
	writeJSON(w, map[string]bool{"ok": true})
}
