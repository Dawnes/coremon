package master

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func notifyCfg() *NotifySettings {
	n := &NotifySettings{}
	n.normalize()
	n.Targets[0].On, n.Targets[0].Topic = true, "coremon-test"
	n.PublicURL = "https://cm.example.org"
	return n
}

func newNotifyState(start time.Time) *notifyState {
	return &notifyState{f: notifyFile{Summary: map[string]*sumItem{}, Outage: map[string]*ntfOutage{}}, notified: map[string]bool{},
		sends: map[string][]int64{}, held: map[string]int{}, nextTry: map[string]time.Time{}, started: start.Add(-time.Hour)}
}

func snapWith(devs map[string]*ntfDev) *ntfSnap {
	return &ntfSnap{devs: devs, pending: map[string]string{}}
}

func dev(name string, down bool) *ntfDev {
	return &ntfDev{Name: name, Group: "Servers", Down: down, Why: "No contact from its agent since 14:05.", Since: 1000, Svcs: map[string]ntfSvc{}}
}

func TestNotifyDiff(t *testing.T) {
	cfg := notifyCfg()
	now := time.Unix(100000, 0)
	n := newNotifyState(now)

	// The first snapshot is the starting point: nothing is sent.
	if evs, _ := n.diff(snapWith(map[string]*ntfDev{"a1": dev("web", true)}), cfg, now); len(evs) != 0 {
		t.Fatalf("baseline sent %v", evs)
	}
	// Up again without a notified failure: silent.
	if evs, _ := n.diff(snapWith(map[string]*ntfDev{"a1": dev("web", false)}), cfg, now); len(evs) != 0 {
		t.Fatalf("recovery without failure sent %v", evs)
	}
	evs, _ := n.diff(snapWith(map[string]*ntfDev{"a1": dev("web", true)}), cfg, now)
	if len(evs) != 1 || evs[0].Title != "web is down" || !strings.Contains(evs[0].Body, "Group: Servers") || evs[0].Click != "https://cm.example.org/#/device/a1" {
		t.Fatalf("down: %+v", evs)
	}
	evs, _ = n.diff(snapWith(map[string]*ntfDev{"a1": dev("web", false)}), cfg, now)
	if len(evs) != 1 || evs[0].Title != "web is up again" {
		t.Fatalf("up: %+v", evs)
	}

	// Unreachable behind a device that is down: silent unless AlsoHidden.
	behind := dev("vm", true)
	behind.Behind = "host"
	if evs, _ := n.diff(snapWith(map[string]*ntfDev{"a1": dev("web", false), "a2": dev("vm", false)}), cfg, now); len(evs) != 0 {
		t.Fatal(evs)
	}
	if evs, _ := n.diff(snapWith(map[string]*ntfDev{"a1": dev("web", false), "a2": behind}), cfg, now); len(evs) != 0 {
		t.Fatalf("unreachable sent %v", evs)
	}

	// A service: down and back; not while the device itself is down.
	up := dev("web", false)
	up.Svcs["s1"] = ntfSvc{Name: "HTTPS", Since: 99000}
	n.diff(snapWith(map[string]*ntfDev{"a1": up}), cfg, now)
	down := dev("web", false)
	down.Svcs["s1"] = ntfSvc{Name: "HTTPS", Down: true, Why: "failed at TLS: certificate expired.", Since: 99500}
	evs, _ = n.diff(snapWith(map[string]*ntfDev{"a1": down}), cfg, now)
	if len(evs) != 1 || evs[0].Kind != "service" || evs[0].Title != "HTTPS on web is down" || !strings.HasPrefix(evs[0].Body, "Failed at TLS") {
		t.Fatalf("service down: %+v", evs)
	}
	back := dev("web", false)
	back.Svcs["s1"] = ntfSvc{Name: "HTTPS"}
	evs, _ = n.diff(snapWith(map[string]*ntfDev{"a1": back}), cfg, now)
	if len(evs) != 1 || evs[0].Title != "HTTPS on web is up again" || !strings.Contains(evs[0].Body, "8 minutes") {
		t.Fatalf("service up: %+v", evs)
	}

	// Upstream and a new device waiting for approval.
	s1 := snapWith(map[string]*ntfDev{})
	s1.upName, s1.upState, s1.upTot = "Internet", "ok", 3
	n.diff(s1, cfg, now)
	s2 := snapWith(map[string]*ntfDev{})
	s2.upName, s2.upState, s2.upAnswer, s2.upTot = "Internet", "partial", 2, 3
	s2.pending["a9"] = "newbox, from 10.0.0.9"
	evs, _ = n.diff(s2, cfg, now)
	if len(evs) != 2 || evs[0].Title != "Internet is partly online" || evs[1].Kind != "pending" {
		t.Fatalf("upstream/pending: %+v", evs)
	}
	s3 := snapWith(map[string]*ntfDev{})
	s3.upName, s3.upState, s3.upTot, s3.pending = "Internet", "wait", 3, s2.pending
	if evs, _ = n.diff(s3, cfg, now); len(evs) != 0 { // waiting changes nothing
		t.Fatalf("wait: %+v", evs)
	}
	s4 := snapWith(map[string]*ntfDev{})
	s4.upName, s4.upState, s4.upTot = "Internet", "ok", 3
	if evs, _ = n.diff(s4, cfg, now); len(evs) != 1 || evs[0].Title != "Internet is online again" {
		t.Fatalf("upstream back: %+v", evs)
	}
}

func TestNotifyGrace(t *testing.T) {
	cfg := notifyCfg()
	now := time.Unix(100000, 0)
	n := newNotifyState(now)
	n.started = now // just started: agents may not have reported yet
	n.diff(snapWith(map[string]*ntfDev{"a1": dev("web", false)}), cfg, now)
	if evs, _ := n.diff(snapWith(map[string]*ntfDev{"a1": dev("web", true)}), cfg, now.Add(time.Minute)); len(evs) != 0 {
		t.Fatalf("sent during the grace period: %v", evs)
	}
}

func TestNotifyThrottleQuiet(t *testing.T) {
	cfg := notifyCfg()
	cfg.ThrottleN, cfg.ThrottleM = 2, 10
	now := time.Unix(100000, 0)
	n := newNotifyState(now)
	e := ntfEvent{Kind: "device", Dev: "a1", Title: "web is down"}
	for i := 0; i < 4; i++ {
		n.emit(cfg, e, now)
	}
	if len(n.f.Queue) != 2 || n.held["a1"] != 2 {
		t.Fatalf("throttle: queue %d held %d", len(n.f.Queue), n.held["a1"])
	}
	snap := snapWith(map[string]*ntfDev{"a1": dev("web", true)})
	n.releaseHeld(cfg, snap, now.Add(5*time.Minute))
	if len(n.f.Queue) != 2 {
		t.Fatal("released before the window had room")
	}
	n.releaseHeld(cfg, snap, now.Add(11*time.Minute))
	if len(n.f.Queue) != 3 || n.f.Queue[2].Title != "web is still down" || !strings.Contains(n.f.Queue[2].Body, "2 changes held back") {
		t.Fatalf("release: %+v", n.f.Queue)
	}

	// Quiet hours over midnight: only Urgent.
	cfg.Quiet, cfg.QuietFrom, cfg.QuietTo = true, "22:00", "07:00"
	night := time.Date(2026, 11, 27, 23, 30, 0, 0, time.UTC)
	if !cfg.inQuiet(night) || cfg.inQuiet(night.Add(8*time.Hour)) || !cfg.inQuiet(night.Add(7*time.Hour)) {
		t.Error("inQuiet")
	}
	q := len(n.f.Queue)
	n.queue(cfg, "device", "x is down", "", 4, nil, "", night)
	n.queue(cfg, "upstream", "Internet is offline", "", 5, nil, "", night)
	if len(n.f.Queue) != q+1 || n.f.Queue[q].Prio != 5 {
		t.Fatalf("quiet: %+v", n.f.Queue[q:])
	}
}

func TestNotifySummary(t *testing.T) {
	cfg := notifyCfg()
	cfg.SummaryAt = "08:00"
	n := newNotifyState(time.Date(2026, 11, 26, 0, 0, 0, 0, time.UTC))
	day := func(h, m int) time.Time { return time.Date(2026, 11, 27, h, m, 0, 0, time.UTC) }

	full := func(sec int, cves ...string) *ntfSnap {
		d := dev("web", false)
		d.Security, d.CVEs = sec, map[string]string{}
		for _, c := range cves {
			d.CVEs[c] = "OpenSSL"
		}
		s := snapWith(map[string]*ntfDev{"a1": d})
		s.full, s.master = true, "0.3.89"
		return s
	}
	n.diff(full(0), cfg, day(6, 0))
	_, items := n.diff(full(5, "CVE-2026-1"), cfg, day(7, 0))
	if len(items) != 2 {
		t.Fatalf("items: %+v", items)
	}
	for _, it := range items {
		n.addSummary(it)
	}
	n.summary(cfg, day(7, 30)) // first run: sets the slot, sends nothing
	if len(n.f.Queue) != 0 {
		t.Fatal("summary before its time")
	}
	n.summary(cfg, day(8, 0))
	if len(n.f.Queue) != 1 {
		t.Fatalf("no summary: %+v", n.f)
	}
	m := n.f.Queue[0]
	if m.Title != "Coremon daily summary" || !strings.Contains(m.Body, "Security updates: web (5)") || !strings.Contains(m.Body, "CVE-2026-1 (OpenSSL on 1 device)") || m.Prio != 4 {
		t.Fatalf("summary: %+v", m)
	}
	n.summary(cfg, day(9, 0))
	if len(n.f.Queue) != 1 {
		t.Fatal("two summaries on one day")
	}
	// An item that is no longer true is left out; nothing left: no message.
	_, items = n.diff(full(7), cfg, day(10, 0))
	for _, it := range items {
		n.addSummary(it)
	}
	n.diff(full(0), cfg, day(11, 0))
	n.summary(cfg, day(8, 0).AddDate(0, 0, 1))
	if len(n.f.Queue) != 1 {
		t.Fatalf("summary of things already solved: %+v", n.f.Queue)
	}
	// In quiet hours the summary waits until they end.
	cfg.Quiet, cfg.QuietFrom, cfg.QuietTo, cfg.SummaryAt = true, "22:00", "07:00", "23:00"
	n.f.Summary["release:master"] = &sumItem{Kind: "release", Name: "master", Text: "0.3.90"}
	cfg.Events["release"] = NotifyEvent{On: true, Prio: 1}
	n.f.LastSlot = summarySlot(cfg, day(22, 0))
	n.summary(cfg, day(23, 5))
	if len(n.f.Queue) != 1 {
		t.Fatal("summary in quiet hours")
	}
	n.summary(cfg, day(7, 1).AddDate(0, 0, 1))
	if len(n.f.Queue) != 2 || !strings.Contains(n.f.Queue[1].Body, "New release: master 0.3.90") {
		t.Fatalf("summary after quiet hours: %+v", n.f.Queue)
	}
}

func TestNotifyDeliverOutage(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	cfg := notifyCfg()
	now := time.Now()
	fail := true
	var got []notifyMsg
	s.ntf.send = func(_ *NotifyTarget, m notifyMsg) (bool, error) {
		if fail {
			return false, errors.New("could not reach the server: connection refused")
		}
		got = append(got, m)
		return false, nil
	}
	s.ntf.mu.Lock()
	s.ntf.queue(cfg, "device", "web is down", "", 4, nil, "", now)
	s.ntf.queue(cfg, "device", "db is down", "", 4, nil, "", now)
	s.ntf.mu.Unlock()
	s.deliver(cfg, now)
	if len(s.ntf.f.Queue) != 2 || s.ntf.f.Outage["ntfy"] == nil {
		t.Fatalf("kept: %+v", s.ntf.f)
	}
	s.deliver(cfg, now.Add(10*time.Second)) // waits a minute before retrying
	fail = false
	s.deliver(cfg, now.Add(2*time.Minute))
	s.deliver(cfg, now.Add(3*time.Minute)) // the note goes out with the next round
	if len(got) != 3 || got[0].Title != "web is down" || got[1].Title != "db is down" || got[2].Title != "Notifications were delayed" || !strings.Contains(got[2].Body, "2 messages arrived late") {
		t.Fatalf("delivered: %+v", got)
	}
	if s.ntf.f.Outage["ntfy"] != nil || len(s.ntf.f.Queue) != 0 {
		t.Fatalf("after outage: %+v", s.ntf.f)
	}
	// Given up after 24 hours.
	fail = true
	s.ntf.mu.Lock()
	s.ntf.queue(cfg, "device", "old", "", 4, nil, "", now.Add(-25*time.Hour))
	s.ntf.mu.Unlock()
	s.deliver(cfg, now.Add(4*time.Minute))
	if len(s.ntf.f.Queue) != 0 || s.ntf.f.Log[len(s.ntf.f.Log)-1].Status != "dropped" {
		t.Fatalf("expire: %+v", s.ntf.f.Queue)
	}
	// Switched off: nothing is kept.
	s.ntf.mu.Lock()
	s.ntf.queue(cfg, "device", "x", "", 4, nil, "", now)
	s.ntf.mu.Unlock()
	cfg.Targets[0].On = false
	s.deliver(cfg, now.Add(5*time.Minute))
	if len(s.ntf.f.Queue) != 0 {
		t.Fatal("kept while off")
	}
	s.ntf.mu.Lock()
	s.ntf.dirty = true
	s.ntf.save()
	s.ntf.mu.Unlock()
	if _, err := os.Stat(filepath.Join(s.cfg.DataDir, notifyFileName)); err != nil {
		t.Fatal(err)
	}
}

func TestNtfySend(t *testing.T) {
	var gotAuth string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&body)
		if body["topic"] == "denied" {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"code":40301,"http":403,"error":"forbidden"}`))
			return
		}
		if body["topic"] == "busy" {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"id":"x"}`))
	}))
	defer srv.Close()
	tg := &NotifyTarget{Server: srv.URL, Topic: "coremon-abc", Auth: authToken, Token: "tk_123"}
	if perm, err := ntfySend(tg, notifyMsg{Title: "t", Body: "b", Prio: 4, Tags: []string{"warning"}, Click: "https://x/#/device/a"}); err != nil || perm {
		t.Fatal(err)
	}
	if gotAuth != "Bearer tk_123" || body["title"] != "t" || body["priority"] != float64(4) || body["click"] != "https://x/#/device/a" {
		t.Errorf("sent %v %v", gotAuth, body)
	}
	tg.Topic = "denied"
	if perm, err := ntfySend(tg, notifyMsg{}); err == nil || !perm || !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("403: %v %v", perm, err)
	}
	tg.Topic = "busy"
	if perm, err := ntfySend(tg, notifyMsg{}); err == nil || perm {
		t.Errorf("429: %v %v", perm, err)
	}
	tg.Server = "http://127.0.0.1:1"
	if perm, err := ntfySend(tg, notifyMsg{}); err == nil || perm {
		t.Errorf("unreachable: %v %v", perm, err)
	}
}

func TestPushover(t *testing.T) {
	var form map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		form = map[string]string{"path": r.URL.Path}
		for k := range r.PostForm {
			form[k] = r.PostForm.Get(k)
		}
		switch {
		case r.PostForm.Get("user") == strings.Repeat("b", 30):
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"user":"invalid","errors":["user identifier is not a valid user, group, or subscribed user key"],"status":0}`))
		case r.PostForm.Get("token") == strings.Repeat("z", 30):
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.Write([]byte(`{"status":1,"request":"x"}`))
		}
	}))
	defer srv.Close()
	old := pushoverURL
	pushoverURL = srv.URL + "/1"
	defer func() { pushoverURL = old }()

	tg := &NotifyTarget{Type: notifyPushover, AppToken: strings.Repeat("a", 30), UserKey: strings.Repeat("u", 30), Devices: "phone,tablet"}
	m := notifyMsg{Title: "web is down", Body: "No contact.", Prio: 5, Click: "https://cm/#/device/a1", At: 1795800000}
	if perm, err := sendTarget(tg, m); err != nil || perm {
		t.Fatal(err)
	}
	if form["path"] != "/1/messages.json" || form["priority"] != "1" || form["device"] != "phone,tablet" || form["url"] != "https://cm/#/device/a1" ||
		form["url_title"] != "Open in Coremon" || form["message"] != "No contact." || form["timestamp"] != "1795800000" {
		t.Errorf("sent: %v", form)
	}
	tg.NoLink = true
	sendTarget(tg, m)
	if _, ok := form["url"]; ok {
		t.Error("link sent while switched off")
	}
	for p, want := range map[int]string{1: "-2", 2: "-1", 3: "0", 4: "1", 5: "1"} {
		if pushoverPrio(p) != want {
			t.Errorf("prio %d", p)
		}
	}
	bad := *tg
	bad.UserKey = strings.Repeat("b", 30)
	if perm, err := sendTarget(&bad, m); !perm || err == nil || !strings.Contains(err.Error(), "not a valid user") {
		t.Errorf("bad key: %v %v", perm, err)
	}
	if reached, err := pushoverValidate(&bad); !reached || err == nil {
		t.Errorf("validate bad key: %v %v", reached, err)
	}
	if reached, err := pushoverValidate(tg); !reached || err != nil || form["path"] != "/1/users/validate.json" {
		t.Errorf("validate: %v %v %v", reached, err, form)
	}
	down := *tg
	down.AppToken = strings.Repeat("z", 30)
	if reached, err := pushoverValidate(&down); reached || err == nil {
		t.Errorf("validate while Pushover fails: %v %v", reached, err)
	}
	if d, err := checkDevices(" phone , my-tablet,"); err != nil || d != "phone,my-tablet" {
		t.Errorf("devices: %q %v", d, err)
	}
	if _, err := checkDevices("bad device"); err == nil {
		t.Error("bad device name accepted")
	}
}

func TestNotifyAPI(t *testing.T) {
	pub, _ := keys(t)
	s := testServer(t, pub, "http://127.0.0.1:1/none", "0.3.0")
	srv := httptest.NewTLSServer(s.routes())
	defer srv.Close()
	pw, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "initial-password.txt"))
	c := newUIClient(t, srv.URL)
	c.do("POST", "/api/ui/login", `{"username":"admin","password":"`+strings.TrimSpace(string(pw))+`"}`)

	code, body := c.do("GET", "/api/ui/notify", "")
	if code != 200 || !strings.Contains(body, `"server":"https://ntfy.sh"`) || !strings.Contains(body, `"key":"device"`) {
		t.Fatalf("get: %d %s", code, body)
	}
	save := func(js string) (int, string) { return c.do("POST", "/api/ui/notify", js) }
	base := `"events":{"pending":{"on":true,"prio":3}},"public_url":"https://cm.example.org/","quiet":true,"quiet_from":"22:00","quiet_to":"07:00","summary_at":"08:30","tz":"UTC","throttle_n":3,"throttle_min":15`
	if code, _ := save(`{"targets":[{"id":"ntfy","on":true,"server":"https://ntfy.sh","topic":"","auth":"none"}],` + base + `}`); code != 400 {
		t.Error("on without a topic was accepted")
	}
	if code, _ := save(`{"targets":[{"id":"ntfy","on":true,"server":"https://ntfy.sh","topic":"bad topic","auth":"none"}],` + base + `}`); code != 400 {
		t.Error("bad topic accepted")
	}
	if code, b := save(`{"targets":[{"id":"ntfy","on":true,"server":"https://ntfy.example.org/","topic":"coremon-x1","auth":"token","token":"tk_secret"}],` + base + `}`); code != 200 {
		t.Fatalf("save: %d %s", code, b)
	}
	_, body = c.do("GET", "/api/ui/notify", "")
	if strings.Contains(body, "tk_secret") || !strings.Contains(body, `"token_set":true`) || !strings.Contains(body, `"public_url":"https://cm.example.org"`) || !strings.Contains(body, `"server":"https://ntfy.example.org"`) {
		t.Fatalf("after save: %s", body)
	}
	s.st.mu.Lock()
	n := s.st.st.Notify
	s.st.mu.Unlock()
	if n.Targets[0].Token != "tk_secret" || !n.Events["pending"].On || n.SummaryAt != "08:30" || !n.Quiet {
		t.Fatalf("stored: %+v %+v", n, n.Targets[0])
	}
	// The token is kept when left empty.
	save(`{"targets":[{"id":"ntfy","on":true,"server":"https://ntfy.example.org","topic":"coremon-x1","auth":"token","token":""}],` + base + `}`)
	s.st.mu.Lock()
	if s.st.st.Notify.Targets[0].Token != "tk_secret" {
		t.Error("token lost")
	}
	s.st.mu.Unlock()
	// The test message uses the form's values.
	var sent *NotifyTarget
	s.ntf.send = func(tg *NotifyTarget, m notifyMsg) (bool, error) { sent = tg; return false, nil }
	code, body = c.do("POST", "/api/ui/notify/test", `{"id":"ntfy","on":false,"server":"https://ntfy.sh","topic":"coremon-test2","auth":"token","token":""}`)
	if code != 200 || !strings.Contains(body, `"ok":true`) || sent == nil || sent.Topic != "coremon-test2" || sent.Token != "tk_secret" {
		t.Fatalf("test: %d %s %+v", code, body, sent)
	}
	if e := s.audit.list("", "", 5); len(e) < 2 || e[0].Action != "notify.test" || e[1].Action != "notify.changed" {
		t.Errorf("audit: %+v", e)
	}
	if code, _ := c.do("GET", "/api/ui/notify/log", ""); code != 200 {
		t.Error("log")
	}

	// Pushover: the keys are checked with Pushover on save.
	old := pushoverCheck
	defer func() { pushoverCheck = old }()
	tok, key := strings.Repeat("a", 30), strings.Repeat("k", 30)
	ntfy := `{"id":"ntfy","on":false,"server":"https://ntfy.sh","topic":"coremon-x1","auth":"none"}`
	po := func(extra string) string {
		return `{"targets":[` + ntfy + `,{"id":"pushover","on":true,"app_token":"` + tok + `","user_key":"` + key + `"` + extra + `}],` + base + `}`
	}
	pushoverCheck = func(*NotifyTarget) (bool, error) { return true, errors.New("Pushover answered: user key is invalid") }
	if code, b := save(po("")); code != 400 || !strings.Contains(b, "user key is invalid") {
		t.Errorf("rejected key: %d %s", code, b)
	}
	pushoverCheck = func(*NotifyTarget) (bool, error) { return false, errors.New("could not reach the server: timeout") }
	if code, b := save(po(`,"devices":"phone, tablet","no_link":true`)); code != 200 || !strings.Contains(b, "could not be checked") {
		t.Errorf("unreachable: %d %s", code, b)
	}
	_, body = c.do("GET", "/api/ui/notify", "")
	if strings.Contains(body, tok) || strings.Contains(body, key) || !strings.Contains(body, `"app_token_set":true`) || !strings.Contains(body, `"devices":"phone,tablet"`) {
		t.Errorf("pushover view: %s", body)
	}
	checked := 0
	pushoverCheck = func(*NotifyTarget) (bool, error) { checked++; return true, nil }
	save(`{"targets":[` + ntfy + `,{"id":"pushover","on":true,"devices":"phone,tablet","no_link":true}],` + base + `}`) // keys kept, unchanged
	if checked != 0 {
		t.Error("unchanged keys checked again")
	}
	if code, _ := save(`{"targets":[` + ntfy + `,{"id":"pushover","on":true,"user_key":"short"}],` + base + `}`); code != 400 {
		t.Error("bad user key accepted")
	}
	s.st.mu.Lock()
	p := s.st.st.Notify.target("pushover")
	s.st.mu.Unlock()
	if p.AppToken != tok || p.UserKey != key || !p.NoLink || !p.On {
		t.Errorf("stored: %+v", p)
	}
}
