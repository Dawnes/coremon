package master

// Website users and roles. Every UI request carries the signed-in user in
// its context; requests that change something need a role that is allowed
// to (checked here, on the server - the website only hides buttons).

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

var userNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)

// Permissions per role. A new role (e.g. an operator that may install
// updates but not change settings) is a new entry here.
type perm int

const (
	permView   perm = iota // see everything (except enrollment tokens)
	permChange             // change anything
)

var rolePerms = map[string]map[perm]bool{
	RoleAdmin:  {permView: true, permChange: true},
	RoleViewer: {permView: true},
}

func can(u *User, p perm) bool { return u != nil && rolePerms[u.Role][p] }

type ctxKey struct{}

// userFrom returns the signed-in user of a UI request.
func userFrom(r *http.Request) *User {
	u, _ := r.Context().Value(ctxKey{}).(*User)
	return u
}

// actor is the name recorded in the audit log for a UI request.
func actor(r *http.Request) string {
	if u := userFrom(r); u != nil {
		return u.Name
	}
	return "?"
}

// requireUI checks the anti-CSRF header, the session and the permission.
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
		if u == nil {
			http.Error(w, "not logged in", http.StatusUnauthorized)
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
	ID      string `json:"id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Created int64  `json:"created"`
}

func viewUser(u *User) userView {
	return userView{ID: u.ID, Name: u.Name, Role: u.Role, Created: u.Created}
}

func (s *Server) setCookie(w http.ResponseWriter, tok string) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: tok, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"authenticated": false, "version": s.cfg.Version, "build_date": s.cfg.BuildDate, "min_password": minPasswordLen}
	// For the login page: is the generated first password still in use?
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
				out["user"] = viewUser(u)
				out["can_change"] = can(u, permChange)
				out["initial_password"] = statErr == nil && u.ID == s.st.st.InitialUser
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
	if !s.sess.allowLogin(ip) {
		http.Error(w, "too many failed attempts; try again later", http.StatusTooManyRequests)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(req.Username))
	s.st.mu.Lock()
	u := s.st.userByName(name)
	hash, uid := dummyHash, ""
	if u != nil {
		hash, uid, name = u.PasswordHash, u.ID, u.Name
	}
	s.st.mu.Unlock()
	if !checkPassword(hash, req.Password) || uid == "" {
		s.sess.recordFail(ip)
		s.audit.write(name, ip, "login.failed", "", "")
		time.Sleep(400 * time.Millisecond)
		http.Error(w, "wrong user name or password", http.StatusUnauthorized)
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

// handlePassword changes the signed-in user's own password.
func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	if len(req.New) < minPasswordLen {
		http.Error(w, fmt.Sprintf("new password must be at least %d characters", minPasswordLen), http.StatusBadRequest)
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
	h := hashPassword(req.New)
	s.st.mu.Lock()
	u.PasswordHash = h
	if u.ID == s.st.st.InitialUser {
		os.Remove(s.initialPwPath())
	}
	s.st.markDirty()
	s.st.mu.Unlock()
	s.sess.destroyUser(u.ID) // sign out everywhere else
	s.setCookie(w, s.sess.create(u.ID))
	s.audit.write(me.Name, clientIP(r), "password.changed", me.Name, "")
	writeJSON(w, map[string]bool{"ok": true})
}

// ---- user management (administrators) ----

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
	case len(req.Password) < minPasswordLen:
		http.Error(w, fmt.Sprintf("password must be at least %d characters", minPasswordLen), http.StatusBadRequest)
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

// handleUserUpdate changes a user's role and/or sets a new password.
func (s *Server) handleUserUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Role     *string `json:"role"`
		Password *string `json:"password"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	if req.Password != nil && len(*req.Password) < minPasswordLen {
		http.Error(w, fmt.Sprintf("password must be at least %d characters", minPasswordLen), http.StatusBadRequest)
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
	if req.Role != nil && *req.Role != u.Role {
		if !validRole(*req.Role) {
			http.Error(w, "invalid role", http.StatusBadRequest)
			return
		}
		if u.Role == RoleAdmin && s.st.admins() == 1 {
			http.Error(w, "this is the last administrator; make someone else administrator first", http.StatusConflict)
			return
		}
		s.audit.write(actor(r), clientIP(r), "user.role", u.Name, u.Role+" → "+*req.Role)
		u.Role, signOut = *req.Role, true
	}
	if req.Password != nil {
		u.PasswordHash = h
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
	if u.Role == RoleAdmin && s.st.admins() == 1 {
		http.Error(w, "this is the last administrator", http.StatusConflict)
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
