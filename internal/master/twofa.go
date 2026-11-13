package master

// Two-factor authentication with an authenticator app: turning it on and
// off in My Account, and the code step at sign-in.

import (
	"net/http"
	"sync"
	"time"
)

const (
	tfaTicketTTL  = 5 * time.Minute
	tfaPendingTTL = 10 * time.Minute
	tfaFails      = 10
	tfaWindow     = 15 * time.Minute
	tfaTracked    = 10000
)

type tfaTicket struct {
	user, ip string
	exp      time.Time
}

type tfaPending struct {
	secret string
	exp    time.Time
}

type twoFAState struct {
	mu      sync.Mutex
	tickets map[string]tfaTicket
	pending map[string]tfaPending
	fails   map[string][]time.Time
}

func (t *twoFAState) init() {
	if t.tickets == nil {
		t.tickets, t.pending, t.fails = map[string]tfaTicket{}, map[string]tfaPending{}, map[string][]time.Time{}
	}
}

func (t *twoFAState) newTicket(user, ip string, now time.Time) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.init()
	for k, v := range t.tickets {
		if now.After(v.exp) {
			delete(t.tickets, k)
		}
	}
	if len(t.tickets) >= tfaTracked {
		for k := range t.tickets {
			delete(t.tickets, k)
			break
		}
	}
	tok := randHex(32)
	t.tickets[tok] = tfaTicket{user: user, ip: ip, exp: now.Add(tfaTicketTTL)}
	return tok
}

func (t *twoFAState) ticket(tok, ip string, now time.Time) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.init()
	v, ok := t.tickets[tok]
	if !ok || v.ip != ip || now.After(v.exp) {
		return "", false
	}
	return v.user, true
}

func (t *twoFAState) dropTicket(tok string) {
	t.mu.Lock()
	delete(t.tickets, tok)
	t.mu.Unlock()
}

// blocked: too many wrong codes for this account lately.
func (t *twoFAState) blocked(user string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.init()
	var keep []time.Time
	for _, f := range t.fails[user] {
		if now.Sub(f) < tfaWindow {
			keep = append(keep, f)
		}
	}
	t.fails[user] = keep
	return len(keep) >= tfaFails
}

// failed records a wrong code; true when this made the account blocked.
func (t *twoFAState) failed(user string, now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.init()
	if len(t.fails) >= tfaTracked {
		t.fails = map[string][]time.Time{}
	}
	t.fails[user] = append(t.fails[user], now)
	return len(t.fails[user]) == tfaFails
}

func (t *twoFAState) clearFails(user string) {
	t.mu.Lock()
	delete(t.fails, user)
	t.mu.Unlock()
}

// checkCode verifies a code for a user, with the per-account limit. Caller
// must not hold s.st.mu. secret "" uses the user's active secret.
func (s *Server) checkCode(w http.ResponseWriter, r *http.Request, uid, secret, code string) bool {
	now := time.Now()
	if s.tfa.blocked(uid, now) {
		http.Error(w, "too many wrong codes for this account; try again in 15 minutes", http.StatusTooManyRequests)
		return false
	}
	s.st.mu.Lock()
	u := s.st.userByID(uid)
	if u == nil {
		s.st.mu.Unlock()
		http.Error(w, "user not found", http.StatusNotFound)
		return false
	}
	name, last := u.Name, u.TOTPLast
	if secret == "" {
		secret = u.TOTPSecret
	}
	step := int64(-1)
	if secret != "" {
		step = totpCheck(secret, code, now, last)
	}
	if step >= 0 {
		u.TOTPLast = step
		s.st.markDirty()
	}
	s.st.mu.Unlock()
	if step < 0 {
		if s.tfa.failed(uid, now) {
			s.audit.write(name, clientIP(r), "login.code_blocked", name, "10 wrong codes: code step blocked for 15 minutes")
		}
		time.Sleep(400 * time.Millisecond)
		http.Error(w, "wrong code", http.StatusUnauthorized)
		return false
	}
	s.tfa.clearFails(uid)
	return true
}

// handleLoginCode is the second sign-in step.
func (s *Server) handleLoginCode(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Coremon") != "1" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	ip := clientIP(r)
	var req struct {
		Ticket string `json:"ticket"`
		Code   string `json:"code"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	if !s.sess.reserveLogin(ip) {
		http.Error(w, "too many failed attempts; try again later", http.StatusTooManyRequests)
		return
	}
	uid, ok := s.tfa.ticket(req.Ticket, ip, time.Now())
	if !ok {
		http.Error(w, "sign-in expired; enter your password again", http.StatusUnauthorized)
		return
	}
	s.st.mu.Lock()
	u := s.st.userByID(uid)
	name, locked := "", true
	if u != nil {
		name, locked = u.Name, u.Locked
	}
	s.st.mu.Unlock()
	if locked {
		s.tfa.dropTicket(req.Ticket)
		http.Error(w, "Your account is locked", http.StatusForbidden)
		return
	}
	if !s.checkCode(w, r, uid, "", req.Code) {
		s.audit.write(name, ip, "login.code_failed", "", "")
		return
	}
	s.tfa.dropTicket(req.Ticket)
	s.sess.forgiveLogin(ip)
	s.setCookie(w, s.sess.create(uid))
	s.audit.write(name, ip, "login", "", "with 2FA")
	writeJSON(w, map[string]bool{"ok": true})
}

// handle2FAStart makes a new key for the signed-in user and returns it with
// its QR code. Nothing changes until a code from the app confirms it.
func (s *Server) handle2FAStart(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	if me.TOTPSecret != "" {
		http.Error(w, "2FA is already on", http.StatusConflict)
		return
	}
	secret := newTOTPSecret()
	uri := totpURI(me.Name, secret)
	qr, err := qrEncode(uri)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.tfa.mu.Lock()
	s.tfa.init()
	s.tfa.pending[me.ID] = tfaPending{secret: secret, exp: time.Now().Add(tfaPendingTTL)}
	s.tfa.mu.Unlock()
	writeJSON(w, map[string]any{"secret": secret, "uri": uri, "qr": qr, "issuer": "Coremon", "account": me.Name})
}

func (s *Server) handle2FAEnable(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	var req struct {
		Code string `json:"code"`
	}
	if !readJSON(w, r, &req, 1024) {
		return
	}
	s.tfa.mu.Lock()
	s.tfa.init()
	p, ok := s.tfa.pending[me.ID]
	s.tfa.mu.Unlock()
	if !ok || time.Now().After(p.exp) {
		http.Error(w, "start again: the QR code has expired", http.StatusConflict)
		return
	}
	if !s.checkCode(w, r, me.ID, p.secret, req.Code) {
		return
	}
	s.tfa.mu.Lock()
	delete(s.tfa.pending, me.ID)
	s.tfa.mu.Unlock()
	s.st.mu.Lock()
	if u := s.st.userByID(me.ID); u != nil {
		u.TOTPSecret, u.TOTPSince = p.secret, time.Now().Unix()
		s.st.markDirty()
	}
	s.st.mu.Unlock()
	keep := ""
	if c, err := r.Cookie(cookieName); err == nil {
		keep = c.Value
	}
	s.sess.destroyUserExcept(me.ID, keep)
	s.audit.write(me.Name, clientIP(r), "user.2fa_enabled", me.Name, "other sessions signed out")
	writeJSON(w, map[string]bool{"ok": true})
}

// adminTurnOff2FA: an administrator turns 2FA off for another account (for
// your own account My Account asks for a code).
func (s *Server) adminTurnOff2FA(w http.ResponseWriter, r *http.Request, on bool) {
	if on {
		http.Error(w, "2FA can only be turned on by the user, in My Account", http.StatusBadRequest)
		return
	}
	s.st.mu.Lock()
	u := s.st.userByID(r.PathValue("id"))
	switch {
	case u == nil:
		s.st.mu.Unlock()
		http.Error(w, "user not found", http.StatusNotFound)
		return
	case u.ID == userFrom(r).ID:
		s.st.mu.Unlock()
		http.Error(w, "turn off your own 2FA in My Account", http.StatusConflict)
		return
	}
	was := u.TOTPSecret != ""
	u.TOTPSecret, u.TOTPLast, u.TOTPSince = "", 0, 0
	s.st.markDirty()
	v := viewUser(u)
	s.st.mu.Unlock()
	if was {
		s.audit.write(actor(r), clientIP(r), "user.2fa_disabled", v.Name, "by an administrator")
	}
	writeJSON(w, v)
}

func (s *Server) handle2FADisable(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	var req struct {
		Code string `json:"code"`
	}
	if !readJSON(w, r, &req, 1024) {
		return
	}
	if me.TOTPSecret == "" {
		http.Error(w, "2FA is not on", http.StatusConflict)
		return
	}
	if !s.checkCode(w, r, me.ID, "", req.Code) {
		return
	}
	s.st.mu.Lock()
	if u := s.st.userByID(me.ID); u != nil {
		u.TOTPSecret, u.TOTPLast, u.TOTPSince = "", 0, 0
		s.st.markDirty()
	}
	s.st.mu.Unlock()
	s.audit.write(me.Name, clientIP(r), "user.2fa_disabled", me.Name, "")
	writeJSON(w, map[string]bool{"ok": true})
}
