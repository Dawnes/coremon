package master

// Settings → Notifications (admins) and Activity → Notifications, and the
// ntfy client.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ntfyClient = &http.Client{Timeout: 20 * time.Second}

// ntfySend publishes one message (ntfy's JSON publishing). permanent: the
// server refused it (wrong topic or credentials), retrying will not help.
func ntfySend(t *NotifyTarget, m notifyMsg) (bool, error) {
	body := map[string]any{"topic": t.Topic, "title": m.Title, "message": m.Body, "priority": m.Prio}
	if len(m.Tags) > 0 {
		body["tags"] = m.Tags
	}
	if m.Click != "" {
		body["click"] = m.Click
	}
	b, _ := json.Marshal(body)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.Server, bytes.NewReader(b))
	if err != nil {
		return true, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "coremon")
	switch t.Auth {
	case authToken:
		req.Header.Set("Authorization", "Bearer "+t.Token)
	case authBasic:
		req.SetBasicAuth(t.User, t.Password)
	}
	resp, err := ntfyClient.Do(req)
	if err != nil {
		return false, cleanNetErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 == 2 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return false, nil
	}
	msg := resp.Status
	var e struct {
		Error string `json:"error"`
	}
	if raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10)); json.Unmarshal(raw, &e) == nil && e.Error != "" {
		msg = fmt.Sprintf("%d %s", resp.StatusCode, e.Error)
	}
	perm := resp.StatusCode/100 == 4 && resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != http.StatusRequestTimeout
	return perm, errors.New("the ntfy server answered " + msg)
}

func cleanNetErr(err error) error {
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 && strings.Contains(s, "Post ") {
		s = s[i+2:]
	}
	return errors.New("could not reach the server: " + s)
}

type notifyTargetView struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"`
	On          bool       `json:"on"`
	Server      string     `json:"server"`
	Topic       string     `json:"topic"`
	Auth        string     `json:"auth"`
	User        string     `json:"user,omitempty"`
	TokenSet    bool       `json:"token_set"`
	PasswordSet bool       `json:"password_set"`
	AppTokenSet bool       `json:"app_token_set"`
	UserKeySet  bool       `json:"user_key_set"`
	Devices     string     `json:"devices,omitempty"`
	NoLink      bool       `json:"no_link"`
	Waiting     int        `json:"waiting"`
	Outage      *ntfOutage `json:"outage,omitempty"`
}

func (s *Server) handleNotifyGet(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	n := s.st.st.Notify
	tz := s.st.st.Settings.TimeZone
	var ts []notifyTargetView
	for _, t := range n.Targets {
		ts = append(ts, notifyTargetView{ID: t.ID, Type: t.Type, On: t.On, Server: t.Server, Topic: t.Topic, Auth: t.Auth, User: t.User,
			TokenSet: t.Token != "", PasswordSet: t.Password != "", AppTokenSet: t.AppToken != "", UserKeySet: t.UserKey != "",
			Devices: t.Devices, NoLink: t.NoLink})
	}
	var evs []notifyEventDef
	for _, d := range notifyEventDefs {
		e := n.Events[d.Key]
		d.On, d.Prio = e.On, e.Prio
		evs = append(evs, d)
	}
	s.st.mu.Unlock()
	s.ntf.mu.Lock()
	for i := range ts {
		for _, m := range s.ntf.f.Queue {
			if m.Target == ts[i].ID {
				ts[i].Waiting++
			}
		}
		if o := s.ntf.f.Outage[ts[i].ID]; o != nil {
			c := *o
			ts[i].Outage = &c
		}
	}
	s.ntf.mu.Unlock()
	writeJSON(w, map[string]any{"targets": ts, "events": evs, "also_hidden": n.AlsoHidden, "no_link": n.NoLink, "public_url": n.PublicURL,
		"quiet": n.Quiet, "quiet_from": n.QuietFrom, "quiet_to": n.QuietTo, "summary_at": n.SummaryAt, "tz": tz,
		"throttle_n": n.ThrottleN, "throttle_min": n.ThrottleM,
		"limits": map[string]int{"throttle_n": maxThrottleN, "throttle_min": maxThrottleM}})
}

type notifyTargetReq struct {
	ID       string `json:"id"`
	On       bool   `json:"on"`
	Server   string `json:"server"`
	Topic    string `json:"topic"`
	Auth     string `json:"auth"`
	User     string `json:"user"`
	Token    string `json:"token"`     // "" keeps the stored one
	Password string `json:"password"`  // "" keeps the stored one
	AppToken string `json:"app_token"` // "" keeps the stored one
	UserKey  string `json:"user_key"`  // "" keeps the stored one
	Devices  string `json:"devices"`
	NoLink   bool   `json:"no_link"`
}

// targetFrom: the request applied to the stored target; secrets left empty
// are kept. checkTopic: a topic is required (on, or for a test).
func targetFrom(old *NotifyTarget, req notifyTargetReq, checkTopic bool) (*NotifyTarget, error) {
	if old.Type == notifyPushover {
		return pushoverFrom(old, req, checkTopic)
	}
	t := *old
	t.On = req.On
	t.NoLink = req.NoLink
	srv, err := checkNtfyServer(req.Server)
	if err != nil {
		return nil, err
	}
	t.Server = srv
	t.Topic = strings.TrimSpace(req.Topic)
	if t.Topic != "" && !ntfyTopicRe.MatchString(t.Topic) {
		return nil, errors.New("the topic may only have letters, digits, - and _ (at most 64)")
	}
	if checkTopic && t.Topic == "" {
		return nil, errors.New("enter a topic")
	}
	switch req.Auth {
	case authNone:
		t.Token, t.User, t.Password = "", "", ""
	case authToken:
		if req.Token != "" {
			t.Token = strings.TrimSpace(req.Token)
		}
		t.User, t.Password = "", ""
		if checkTopic && t.Token == "" {
			return nil, errors.New("enter the access token")
		}
		if len(t.Token) > 256 || strings.ContainsAny(t.Token, " \r\n\t") {
			return nil, errors.New("the access token is not valid")
		}
	case authBasic:
		t.User = strings.TrimSpace(req.User)
		if req.Password != "" {
			t.Password = req.Password
		}
		t.Token = ""
		if checkTopic && (t.User == "" || t.Password == "") {
			return nil, errors.New("enter the username and password")
		}
		if len(t.User) > 128 || len(t.Password) > 256 || strings.ContainsAny(t.User, ":\r\n") {
			return nil, errors.New("the username or password is not valid")
		}
	default:
		return nil, errors.New("choose how to sign in to the server")
	}
	t.Auth = req.Auth
	return &t, nil
}

func (s *Server) handleNotifySave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Targets    []notifyTargetReq      `json:"targets"`
		Events     map[string]NotifyEvent `json:"events"`
		AlsoHidden bool                   `json:"also_hidden"`
		NoLink     bool                   `json:"no_link"`
		PublicURL  string                 `json:"public_url"`
		Quiet      bool                   `json:"quiet"`
		QuietFrom  string                 `json:"quiet_from"`
		QuietTo    string                 `json:"quiet_to"`
		SummaryAt  string                 `json:"summary_at"`

		ThrottleN int `json:"throttle_n"`
		ThrottleM int `json:"throttle_min"`
	}
	if !readJSON(w, r, &req, 16<<10) {
		return
	}
	bad := func(msg string) { http.Error(w, msg, http.StatusBadRequest) }
	pub, err := checkPublicURL(req.PublicURL)
	if err != nil {
		bad(err.Error())
		return
	}
	for _, v := range []string{req.QuietFrom, req.QuietTo, req.SummaryAt} {
		if !hhmmRe.MatchString(v) {
			bad("times are written like 07:30")
			return
		}
	}
	if req.Quiet && req.QuietFrom == req.QuietTo {
		bad("the quiet hours must start and end at different times")
		return
	}

	if req.ThrottleN < 1 || req.ThrottleN > maxThrottleN || req.ThrottleM < 1 || req.ThrottleM > maxThrottleM {
		bad(fmt.Sprintf("at most 1 to %d messages in 1 to %d minutes", maxThrottleN, maxThrottleM))
		return
	}
	for k, e := range req.Events {
		if notifyEventDefOf(k) == nil || e.Prio < 1 || e.Prio > 5 {
			bad("unknown event or priority")
			return
		}
	}

	s.st.mu.Lock()
	old := s.st.st.Notify
	s.st.mu.Unlock()
	nu := NotifySettings{Events: map[string]NotifyEvent{}, AlsoHidden: req.AlsoHidden, NoLink: req.NoLink, PublicURL: pub,
		Quiet: req.Quiet, QuietFrom: req.QuietFrom, QuietTo: req.QuietTo, SummaryAt: req.SummaryAt,
		ThrottleN: req.ThrottleN, ThrottleM: req.ThrottleM}
	for k, e := range old.Events {
		nu.Events[k] = e
	}
	for k, e := range req.Events {
		nu.Events[k] = e
	}
	var changes []string
	for _, ot := range old.Targets {
		nt := ot
		for _, tr := range req.Targets {
			if tr.ID != ot.ID {
				continue
			}
			t, err := targetFrom(ot, tr, tr.On)
			if err != nil {
				bad(err.Error())
				return
			}
			nt = t
		}
		name := serviceName(ot.Type)
		switch {
		case nt.On != ot.On:
			changes = append(changes, name+" switched "+map[bool]string{true: "on", false: "off"}[nt.On])
		case nt.Server != ot.Server || nt.Topic != ot.Topic || nt.Auth != ot.Auth || nt.User != ot.User || nt.Token != ot.Token || nt.Password != ot.Password ||
			nt.AppToken != ot.AppToken || nt.UserKey != ot.UserKey || nt.Devices != ot.Devices:
			changes = append(changes, name+" connection changed")
		}
		if nt.NoLink != ot.NoLink {
			changes = append(changes, name+map[bool]string{true: " without links", false: " with links"}[nt.NoLink])
		}
		nu.Targets = append(nu.Targets, nt)
	}
	nu.normalize()
	for _, d := range notifyEventDefs {
		if nu.Events[d.Key] != old.Events[d.Key] {
			changes = append(changes, "events changed")
			break
		}
	}
	if nu.Quiet != old.Quiet || nu.QuietFrom != old.QuietFrom || nu.QuietTo != old.QuietTo {
		changes = append(changes, "quiet hours "+map[bool]string{true: nu.QuietFrom + "–" + nu.QuietTo, false: "off"}[nu.Quiet])
	}
	if nu.SummaryAt != old.SummaryAt {
		changes = append(changes, "daily summary at "+nu.SummaryAt)
	}
	if nu.ThrottleN != old.ThrottleN || nu.ThrottleM != old.ThrottleM {
		changes = append(changes, fmt.Sprintf("at most %d messages in %d minutes per device", nu.ThrottleN, nu.ThrottleM))
	}
	if nu.PublicURL != old.PublicURL || nu.NoLink != old.NoLink || nu.AlsoHidden != old.AlsoHidden {
		changes = append(changes, "options changed")
	}
	// Pushover: check the token and key with Pushover itself when they are
	// new or it is switched on (outside the lock: it goes over the network).
	warning := ""
	for _, nt := range nu.Targets {
		ot := old.target(nt.ID)
		if nt.Type != notifyPushover || !nt.On || (ot != nil && ot.On && ot.AppToken == nt.AppToken && ot.UserKey == nt.UserKey && ot.Devices == nt.Devices) {
			continue
		}
		reached, err := pushoverCheck(nt)
		switch {
		case reached && err != nil:
			bad(err.Error())
			return
		case !reached:
			warning = "Saved, but the keys could not be checked: " + err.Error()
		}
	}
	s.st.mu.Lock()
	s.st.st.Notify = nu
	s.st.markDirty()
	s.st.mu.Unlock()
	if len(changes) > 0 {
		s.audit.write(actor(r), clientIP(r), "notify.changed", "notifications", strings.Join(changes, "; "))
	}
	res := map[string]any{"ok": true}
	if warning != "" {
		res["warning"] = warning
	}
	writeJSON(w, res)
}

// pushoverCheck: tests replace it.
var pushoverCheck = pushoverValidate

// pushoverFrom: the Pushover part of targetFrom. need: the keys are
// required (on, or for a test).
func pushoverFrom(old *NotifyTarget, req notifyTargetReq, need bool) (*NotifyTarget, error) {
	t := *old
	t.On, t.NoLink = req.On, req.NoLink
	if v := strings.TrimSpace(req.AppToken); v != "" {
		if !pushKeyRe.MatchString(v) {
			return nil, errors.New("the application token has 30 letters and digits")
		}
		t.AppToken = v
	}
	if v := strings.TrimSpace(req.UserKey); v != "" {
		if !pushKeyRe.MatchString(v) {
			return nil, errors.New("the user key has 30 letters and digits")
		}
		t.UserKey = v
	}
	devs, err := checkDevices(req.Devices)
	if err != nil {
		return nil, err
	}
	t.Devices = devs
	if need && (t.AppToken == "" || t.UserKey == "") {
		return nil, errors.New("enter the application token and the user key")
	}
	return &t, nil
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	var req notifyTargetReq
	if !readJSON(w, r, &req, 8<<10) {
		return
	}
	s.st.mu.Lock()
	old := s.st.st.Notify.target(req.ID)
	var t *NotifyTarget
	var err error
	if old == nil {
		err = errors.New("unknown notifier")
	} else {
		t, err = targetFrom(old, req, true)
	}
	name := s.st.st.Settings.MasterURL
	s.st.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if name == "" {
		name = r.Host
	}
	m := notifyMsg{Title: "Coremon test message", Body: "Notifications from the Coremon master at " + name + " work.", Prio: 3, Tags: []string{"white_check_mark"}}
	s.ntf.mu.Lock()
	send := s.ntf.send
	s.ntf.mu.Unlock()
	_, err = send(t, m)
	res := map[string]any{"ok": err == nil, "at": time.Now().Unix()}
	detail := serviceName(t.Type) + " test message delivered"
	if err != nil {
		res["error"] = err.Error()
		detail = serviceName(t.Type) + " test message failed: " + err.Error()
	}
	s.audit.write(actor(r), clientIP(r), "notify.test", "notifications", detail)
	writeJSON(w, res)
}

func (s *Server) handleNotifyLog(w http.ResponseWriter, r *http.Request) {
	s.ntf.mu.Lock()
	out := make([]notifyLogEntry, 0, len(s.ntf.f.Log))
	for i := len(s.ntf.f.Log) - 1; i >= 0; i-- {
		out = append(out, s.ntf.f.Log[i])
	}
	waiting := len(s.ntf.f.Queue)
	s.ntf.mu.Unlock()
	s.st.mu.Lock()
	on := s.st.st.Notify.anyOn()
	s.st.mu.Unlock()
	writeJSON(w, map[string]any{"entries": out, "waiting": waiting, "on": on})
}
