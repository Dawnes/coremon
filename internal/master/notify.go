package master

// Notifications: every 10 seconds the master takes a snapshot of what
// matters (devices down, services failing, Upstream, new devices; every 5
// minutes also security updates, serious CVEs, reboots, OS support and
// releases) and compares it with the previous one. A change becomes a
// message right away, or an item of the daily summary. Messages are
// throttled per device, held back in quiet hours (except Urgent ones) and
// kept for up to 24 hours when the service cannot be reached.
//
// Only changes count: what is already wrong when the master starts, or when
// a device leaves maintenance, sends nothing. A recovery is only sent when
// its failure was.

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	notifyFileName = "notify.json"
	notifyTick     = 10 * time.Second
	notifyFullTick = 5 * time.Minute
	notifyGrace    = 2 * time.Minute // after start: agents first report again
	notifyKeep     = 24 * time.Hour
	notifyRetry    = time.Minute
	notifyLogMax   = 200
)

type notifyMsg struct {
	Seq    int64    `json:"seq"`
	Target string   `json:"target"`
	Event  string   `json:"event"`
	At     int64    `json:"at"`
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Prio   int      `json:"prio"`
	Tags   []string `json:"tags,omitempty"`
	Click  string   `json:"click,omitempty"`
}

// notifyLogEntry: what happened to a message (Activity → Notifications).
type notifyLogEntry struct {
	At     int64  `json:"at"`
	Target string `json:"target"`
	Event  string `json:"event"`
	Title  string `json:"title"`
	Status string `json:"status"` // sent, delayed, failed, quiet, dropped, held
	Error  string `json:"error,omitempty"`
}

// sumItem: something for the daily summary.
type sumItem struct {
	Kind  string   `json:"kind"`
	Dev   string   `json:"dev,omitempty"`
	Name  string   `json:"name,omitempty"`
	Count int      `json:"count,omitempty"`
	CVE   string   `json:"cve,omitempty"`
	App   string   `json:"app,omitempty"`
	Devs  []string `json:"devs,omitempty"`
	Text  string   `json:"text,omitempty"`
	At    int64    `json:"at"`
}

type ntfOutage struct {
	From    int64 `json:"from"`
	Late    int   `json:"late"`
	Dropped int   `json:"dropped"`
	OK      bool  `json:"ok"` // a message went through again
}

// notifyFile: kept in notify.json, so a restart loses nothing waiting.
type notifyFile struct {
	Seq      int64                 `json:"seq"`
	Queue    []notifyMsg           `json:"queue"`
	Summary  map[string]*sumItem   `json:"summary"`
	Log      []notifyLogEntry      `json:"log"`
	LastSlot int64                 `json:"last_slot"` // the last daily summary slot handled
	Outage   map[string]*ntfOutage `json:"outage,omitempty"`
}

type ntfSvc struct {
	Name, Why string
	Down      bool
	Since     int64
}

type ntfDev struct {
	Name, Group, Why, Behind string
	Down, Unknown            bool
	Since                    int64
	Svcs                     map[string]ntfSvc
	// full snapshots only
	Security int
	Reboot   bool
	OS       string            // "" fine, else the support state (soon/expired and the date)
	osText   string            // the OS support line for the summary
	CVEs     map[string]string // CVE → application
}

type ntfSnap struct {
	full            bool
	devs            map[string]*ntfDev
	upName, upState string
	upAnswer, upTot int
	upSince         int64
	pending         map[string]string
	master, core    string
}

type ntfEvent struct {
	Kind, Dev, Title, Body string
	Tags                   []string
	Click                  string
}

type notifyState struct {
	mu       sync.Mutex
	f        notifyFile
	dirty    bool
	path     string
	prev     *ntfSnap
	prevFull *ntfSnap
	notified map[string]bool
	sends    map[string][]int64
	held     map[string]int
	started  time.Time
	lastFull time.Time
	nextTry  map[string]time.Time
	send     func(t *NotifyTarget, m notifyMsg) (permanent bool, err error)
}

func (s *Server) loadNotify() {
	n := &s.ntf
	n.path = filepath.Join(s.cfg.DataDir, notifyFileName)
	n.notified, n.sends, n.held, n.nextTry = map[string]bool{}, map[string][]int64{}, map[string]int{}, map[string]time.Time{}
	n.started = time.Now()
	if n.send == nil {
		n.send = ntfySend
	}
	if b, err := os.ReadFile(n.path); err == nil {
		if err := json.Unmarshal(b, &n.f); err != nil {
			log.Printf("notify: %s: %v (starting empty)", n.path, err)
			n.f = notifyFile{}
		}
	}
	if n.f.Summary == nil {
		n.f.Summary = map[string]*sumItem{}
	}
	if n.f.Outage == nil {
		n.f.Outage = map[string]*ntfOutage{}
	}
}

func (n *notifyState) save() {
	if !n.dirty {
		return
	}
	b, err := json.Marshal(&n.f)
	if err != nil {
		return
	}
	tmp := n.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err == nil && os.Rename(tmp, n.path) == nil {
		n.dirty = false
	}
}

func (n *notifyState) logEntry(e notifyLogEntry) {
	n.f.Log = append(n.f.Log, e)
	if over := len(n.f.Log) - notifyLogMax; over > 0 {
		n.f.Log = append([]notifyLogEntry(nil), n.f.Log[over:]...)
	}
	n.dirty = true
}

func (s *Server) notifyLoop(stop <-chan struct{}) {
	t := time.NewTicker(notifyTick)
	defer t.Stop()
	for {
		select {
		case <-stop:
			s.ntf.mu.Lock()
			s.ntf.save()
			s.ntf.mu.Unlock()
			return
		case <-t.C:
			s.notifyTickOnce(time.Now())
		}
	}
}

// notifyTickOnce: one round: compare, queue, summary, deliver.
func (s *Server) notifyTickOnce(now time.Time) {
	n := &s.ntf
	full := n.lastFull.IsZero() || now.Sub(n.lastFull) >= notifyFullTick
	s.st.mu.Lock()
	cfg := s.st.st.Notify
	cfg.Targets = nil
	for _, t := range s.st.st.Notify.Targets {
		c := *t
		cfg.Targets = append(cfg.Targets, &c)
	}
	snap := s.notifySnap(full, now)
	s.st.mu.Unlock()
	if full {
		n.lastFull = now
	}

	n.mu.Lock()
	evs, items := n.diff(snap, &cfg, now)
	on := cfg.anyOn()
	if on {
		for _, it := range items {
			n.addSummary(it)
		}
		for _, e := range evs {
			n.emit(&cfg, e, now)
		}
		n.releaseHeld(&cfg, snap, now)
		n.summary(&cfg, now)
	} else {
		n.f.LastSlot = summarySlot(&cfg, now)
	}
	n.mu.Unlock()
	s.deliver(&cfg, now)
	n.mu.Lock()
	n.save()
	n.mu.Unlock()
}

// ---- the snapshot ----

func clockText(cfg *NotifySettings, at int64) string {
	t := time.Unix(at, 0).In(cfg.loc())
	if time.Since(t) > 20*time.Hour {
		return t.Format("Jan 2 15:04")
	}
	return t.Format("15:04")
}

// notifySnap: caller holds s.st.mu.
func (s *Server) notifySnap(full bool, now time.Time) *ntfSnap {
	ts := now.Unix()
	cfg := &s.st.st.Notify
	sn := &ntfSnap{full: full, devs: map[string]*ntfDev{}, pending: map[string]string{}}
	group := func(id string) string {
		if g := s.st.group(id); g != nil {
			return g.Name
		}
		return ""
	}
	for _, a := range s.st.st.Agents {
		if a.Status == StatusPending {
			name := a.Name
			if a.Facts.Hostname != "" && a.Facts.Hostname != name {
				name += " (" + a.Facts.Hostname + ")"
			}
			if a.EnrollIP != "" {
				name += ", from " + a.EnrollIP
			}
			sn.pending[a.ID] = name
			continue
		}
		if a.Maint != nil || a.Removing {
			continue
		}
		on := s.online(a, ts)
		d := &ntfDev{Name: a.Name, Group: group(a.GroupID), Svcs: map[string]ntfSvc{}}
		if !on {
			d.Down, d.Since = true, a.LastSeen
			d.Why = "No contact from its agent since " + clockText(cfg, a.LastSeen) + "."
			if b := s.blockedBy(a.ID, ts); b != nil {
				d.Behind = b.Name
			}
		}
		if agentHasChecks(a) {
			lv := s.checksView(a.Checks, true, !on)
			ntfServices(d, lv, a.Checks.Services)
		}
		if full && !s.belowMin(a) {
			var u updatesSummary
			if raw, ok := a.State["updates"]; ok && json.Unmarshal(raw, &u) == nil {
				d.Security, d.Reboot = u.Security, u.RebootRequired
			}
			d.CVEs = map[string]string{}
			if vv := s.vulnsFor(a, true); vv != nil {
				if u.SecurityFromCVE {
					d.Security = min(vv.fixPkgs, u.Total)
				}
				for app, av := range vv.Apps {
					for _, c := range av.CVEs {
						if c.red() {
							d.CVEs[c.ID] = app
						}
					}
				}
			}
			if d.OS != "" { // the OS support service is down
				d.OS, d.osText = "down", "OS support for "+releaseName(a.Facts)+" is ending"
				if o := s.supportFor(a.Facts.OSID, a.Facts.OSVersion, now); o != nil && o.End != "" {
					if o.Status == "expired" {
						d.OS, d.osText = "expired "+o.End, "OS support for "+releaseName(a.Facts)+" ended on "+o.End
					} else {
						d.OS, d.osText = "soon "+o.End, "OS support for "+releaseName(a.Facts)+" ends on "+o.End
					}
				}
			}
		} else {
			d.OS = ""
		}
		sn.devs[a.ID] = d
	}
	for _, l := range s.st.st.Locals {
		if l.Maint != nil {
			continue
		}
		lv := s.checksView(&l.Checks, true, false)
		d := &ntfDev{Name: l.Name, Group: group(l.GroupID), Svcs: map[string]ntfSvc{}}
		switch {
		case !lv.Checked:
			d.Unknown = true
		case !lv.Up:
			d.Down, d.Since = true, lv.Since
			switch {
			case lv.Ping && lv.LastError != "":
				d.Why = fmt.Sprintf("Ping failed %d times: %s.", lv.Fails, strings.TrimSuffix(lv.LastError, "."))
			case lv.Ping:
				d.Why = fmt.Sprintf("Ping failed %d times.", lv.Fails)
			case lv.SvcFail != nil:
				d.Why = "All services are down; " + lv.SvcFail.Name + ": " + failText(lv.SvcFail.Step, lv.SvcFail.Error)
			default:
				d.Why = "All services are down."
			}
			if b := s.blockedBy(l.ID, ts); b != nil {
				d.Behind = b.Name
			}
		}
		ntfServices(d, lv, l.Checks.Services)
		sn.devs[l.ID] = d
	}
	if u := s.upstreamView(); u.Enabled {
		sn.upName, sn.upState, sn.upAnswer, sn.upTot, sn.upSince = u.Name, u.State, u.Answer, u.Total, u.Since
	}
	sn.master, sn.core = s.repo.masterLatest(), s.repo.coreLatest()
	return sn
}

func failText(step, err string) string {
	err = strings.TrimSuffix(strings.TrimSpace(err), ".")
	switch {
	case step != "" && err != "":
		return "failed at " + step + ": " + err + "."
	case step != "":
		return "failed at " + step + "."
	case err != "":
		return err + "."
	}
	return "no answer."
}

// ntfServices: the running services of a device that were checked; the OS
// support service marks the OS line instead.
func ntfServices(d *ntfDev, lv *localView, svcs []*Service) {
	for i, sv := range svcs {
		if sv.Paused || i >= len(lv.Services) {
			continue
		}
		v := lv.Services[i]
		if sv.isOSSupport() {
			if v.Checked && !v.Up {
				d.OS = "down"
			}
			continue
		}
		if !v.Checked {
			continue
		}
		since := v.Since
		d.Svcs[sv.ID] = ntfSvc{Name: v.Name, Down: !v.Up, Why: failText(v.Failed, v.LastError), Since: since}
	}
}

// ---- comparing ----

// diff compares the snapshot with the previous one: the events to send now
// and the items for the summary. Caller holds n.mu.
func (n *notifyState) diff(now *ntfSnap, cfg *NotifySettings, at time.Time) ([]ntfEvent, []*sumItem) {
	prev := n.prev
	baseline := prev == nil || at.Sub(n.started) < notifyGrace
	var evs []ntfEvent
	link := func(path string) string {
		if cfg.NoLink || cfg.PublicURL == "" {
			return ""
		}
		return cfg.PublicURL + "/#/" + path
	}
	grp := func(d *ntfDev) string {
		if d.Group == "" {
			return ""
		}
		return " Group: " + d.Group + "."
	}
	for id, d := range now.devs {
		var p *ntfDev
		if prev != nil {
			p = prev.devs[id]
		}
		if p == nil {
			continue // new, or back from maintenance: the starting point
		}
		if d.Unknown {
			d.Down, d.Behind, d.Why, d.Since = p.Down, p.Behind, p.Why, p.Since
		}
		key := "dev:" + id
		switch {
		case d.Down && !p.Down && !baseline:
			if d.Behind != "" && !cfg.AlsoHidden {
				break
			}
			e := ntfEvent{Kind: "device", Dev: id, Title: d.Name + " is down", Body: d.Why + grp(d), Tags: []string{"red_circle"}, Click: link("device/" + id)}
			if d.Behind != "" {
				e.Title = d.Name + " is unreachable"
				e.Body = "It does not answer, and " + d.Behind + ", which it is connected via, is down or in maintenance." + grp(d)
				e.Tags = []string{"orange_circle"}
			}
			evs = append(evs, e)
			n.notified[key] = true
		case !d.Down && p.Down:
			if n.notified[key] {
				body := "It is answering again."
				if p.Since > 0 {
					body = "It was down for " + spanText(at.Unix()-p.Since) + "."
				}
				evs = append(evs, ntfEvent{Kind: "device", Dev: id, Title: d.Name + " is up again", Body: body + grp(d), Tags: []string{"green_circle"}, Click: link("device/" + id)})
			}
			delete(n.notified, key)
		}
		if d.Down || d.Unknown {
			d.Svcs = p.Svcs // services wait while the device itself is down
			continue
		}
		for sid, ps := range p.Svcs {
			sv, ok := d.Svcs[sid]
			if !ok {
				d.Svcs[sid] = ps // not checked this time: unchanged
				continue
			}
			skey := "svc:" + id + ":" + sid
			switch {
			case sv.Down && !ps.Down && !baseline:
				evs = append(evs, ntfEvent{Kind: "service", Dev: id, Title: sv.Name + " on " + d.Name + " is down", Body: upFirst(sv.Why) + grp(d), Tags: []string{"warning"}, Click: link("device/" + id)})
				n.notified[skey] = true
			case !sv.Down && ps.Down:
				if n.notified[skey] {
					body := "It works again."
					if ps.Since > 0 {
						body = "It was down for " + spanText(at.Unix()-ps.Since) + "."
					}
					evs = append(evs, ntfEvent{Kind: "service", Dev: id, Title: sv.Name + " on " + d.Name + " is up again", Body: body + grp(d), Tags: []string{"white_check_mark"}, Click: link("device/" + id)})
				}
				delete(n.notified, skey)
			}
		}
	}
	// Upstream: ok, partial, down; wait and off change nothing.
	if prev != nil {
		ps, cs := prev.upState, now.upState
		if cs == "wait" || cs == "" {
			now.upState, now.upSince = ps, prev.upSince
			cs = ps
		}
		if cs != ps && ps != "" && ps != "wait" && cs != "off" && ps != "off" {
			switch {
			case cs == "ok":
				if n.notified["up"] {
					body := "All addresses answer again."
					if prev.upSince > 0 {
						body = "It was offline or partly online for " + spanText(at.Unix()-prev.upSince) + "."
					}
					evs = append(evs, ntfEvent{Kind: "upstream", Dev: upstreamID, Title: now.upName + " is online again", Body: body, Tags: []string{"globe_with_meridians"}, Click: link("dashboard/overview")})
				}
				delete(n.notified, "up")
			case !baseline:
				e := ntfEvent{Kind: "upstream", Dev: upstreamID, Title: now.upName + " is offline", Body: fmt.Sprintf("None of the %d addresses answer.", now.upTot), Tags: []string{"no_entry"}, Click: link("dashboard/overview")}
				if cs == "partial" {
					e.Title, e.Body, e.Tags = now.upName+" is partly online", fmt.Sprintf("%d of %d addresses answer.", now.upAnswer, now.upTot), []string{"warning"}
				}
				evs = append(evs, e)
				n.notified["up"] = true
			}
		}
		if !baseline {
			ids := make([]string, 0, len(now.pending))
			for id := range now.pending {
				if _, was := prev.pending[id]; !was {
					ids = append(ids, id)
				}
			}
			sort.Strings(ids)
			for _, id := range ids {
				evs = append(evs, ntfEvent{Kind: "pending", Title: "New device waiting for approval", Body: now.pending[id] + ". Approve it in Coremon.", Tags: []string{"new"}, Click: link("dashboard/devices")})
			}
		}
	}
	n.prev = now

	// The summary items, from full snapshots.
	var items []*sumItem
	if !now.full {
		if n.prevFull != nil {
			for id, d := range now.devs { // keep the last full values for the next comparison
				if p := n.prevFull.devs[id]; p != nil {
					d.Security, d.Reboot, d.OS, d.CVEs, d.osText = p.Security, p.Reboot, p.OS, p.CVEs, p.osText
				}
			}
		}
		return evs, nil
	}
	pf := n.prevFull
	n.prevFull = now
	if pf == nil || baseline {
		return evs, nil
	}
	ts := at.Unix()
	for id, d := range now.devs {
		p := pf.devs[id]
		if p == nil {
			continue
		}
		if d.Security > p.Security && d.Security > 0 {
			items = append(items, &sumItem{Kind: "security", Dev: id, Name: d.Name, Count: d.Security, At: ts})
		}
		for c, app := range d.CVEs {
			if _, had := p.CVEs[c]; !had {
				items = append(items, &sumItem{Kind: "cve", CVE: c, App: app, Devs: []string{id}, At: ts})
			}
		}
		if d.Reboot && !p.Reboot {
			items = append(items, &sumItem{Kind: "reboot", Dev: id, Name: d.Name, At: ts})
		}
		if d.OS != "" && d.OS != p.OS && d.osText != "" {
			items = append(items, &sumItem{Kind: "os", Dev: id, Name: d.Name, Text: d.osText, At: ts})
		}
	}
	if now.master != pf.master && pf.master != "" && now.master != "" {
		items = append(items, &sumItem{Kind: "release", Name: "master", Text: now.master, At: ts})
	}
	if now.core != pf.core && pf.core != "" && now.core != "" {
		items = append(items, &sumItem{Kind: "release", Name: "agent", Text: now.core, At: ts})
	}
	return evs, items
}

func upFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// spanText: 125 → "2 minutes".
func spanText(sec int64) string {
	switch {
	case sec < 90:
		return "1 minute"
	case sec < 3600:
		return fmt.Sprintf("%d minutes", (sec+30)/60)
	case sec < 2*3600:
		return fmt.Sprintf("1 hour %d minutes", (sec-3600+30)/60)
	case sec < 48*3600:
		return fmt.Sprintf("%d hours", (sec+1800)/3600)
	}
	return fmt.Sprintf("%d days", (sec+43200)/86400)
}

// ---- sending ----

// emit: an event becomes a message for every target that is on, unless the
// device sent too many lately or it is quiet. Caller holds n.mu.
func (n *notifyState) emit(cfg *NotifySettings, e ntfEvent, at time.Time) {
	ev := cfg.Events[e.Kind]
	if !ev.On {
		return
	}
	if e.Dev != "" && !n.allow(cfg, e.Dev, at) {
		n.held[e.Dev]++
		n.logEntry(notifyLogEntry{At: at.Unix(), Event: e.Kind, Title: e.Title, Status: "held", Error: fmt.Sprintf("more than %d messages in %d minutes for this device", cfg.ThrottleN, cfg.ThrottleM)})
		return
	}
	n.queue(cfg, e.Kind, e.Title, e.Body, ev.Prio, e.Tags, e.Click, at)
}

// allow: the device may send one more message now (and that is counted).
func (n *notifyState) allow(cfg *NotifySettings, dev string, at time.Time) bool {
	cut := at.Add(-time.Duration(cfg.ThrottleM) * time.Minute).Unix()
	keep := n.sends[dev][:0]
	for _, t := range n.sends[dev] {
		if t > cut {
			keep = append(keep, t)
		}
	}
	n.sends[dev] = keep
	if len(keep) >= cfg.ThrottleN {
		return false
	}
	n.sends[dev] = append(keep, at.Unix())
	return true
}

// queue: a message for every target that is on; in quiet hours only
// Urgent ones. Caller holds n.mu.
func (n *notifyState) queue(cfg *NotifySettings, event, title, body string, prio int, tags []string, click string, at time.Time) {
	if prio < 5 && cfg.inQuiet(at) {
		n.logEntry(notifyLogEntry{At: at.Unix(), Event: event, Title: title, Status: "quiet"})
		return
	}
	for _, t := range cfg.Targets {
		if !t.On {
			continue
		}
		n.f.Seq++
		n.f.Queue = append(n.f.Queue, notifyMsg{Seq: n.f.Seq, Target: t.ID, Event: event, At: at.Unix(), Title: title, Body: body, Prio: prio, Tags: tags, Click: click})
	}
	n.dirty = true
}

// releaseHeld: a throttled device whose window has room again sends one
// message with how things stand now. Caller holds n.mu.
func (n *notifyState) releaseHeld(cfg *NotifySettings, snap *ntfSnap, at time.Time) {
	for dev, count := range n.held {
		if !n.allow(cfg, dev, at) {
			continue
		}
		delete(n.held, dev)
		changes := fmt.Sprintf("%d change%s held back in the last %d minutes.", count, map[bool]string{true: "", false: "s"}[count == 1], cfg.ThrottleM)
		if dev == upstreamID {
			state := map[string]string{"ok": "online", "partial": "partly online", "down": "offline"}[snap.upState]
			n.queue(cfg, "upstream", snap.upName+" is now "+state, changes, cfg.Events["upstream"].Prio, []string{"repeat"}, "", at)
			continue
		}
		d := snap.devs[dev]
		if d == nil {
			continue
		}
		title, kind := d.Name+" is up", "device"
		if d.Down {
			title = d.Name + " is still down"
		} else {
			var down []string
			for _, sv := range d.Svcs {
				if sv.Down {
					down = append(down, sv.Name)
				}
			}
			sort.Strings(down)
			if len(down) > 0 {
				title, kind = d.Name+": down "+strings.Join(down, ", "), "service"
			}
		}
		click := ""
		if !cfg.NoLink && cfg.PublicURL != "" {
			click = cfg.PublicURL + "/#/device/" + dev
		}
		n.queue(cfg, kind, title, changes, cfg.Events[kind].Prio, []string{"repeat"}, click, at)
	}
}

func (n *notifyState) addSummary(it *sumItem) {
	var key string
	switch it.Kind {
	case "cve":
		key = "cve:" + it.CVE
		if old := n.f.Summary[key]; old != nil {
			for _, d := range old.Devs {
				if d != it.Devs[0] {
					it.Devs = append(it.Devs, d)
				}
			}
		}
	case "release":
		key = "release:" + it.Name
	default:
		key = it.Kind + ":" + it.Dev
	}
	n.f.Summary[key] = it
	n.dirty = true
}

// summarySlot: the most recent daily summary time (unix), at or before now.
func summarySlot(cfg *NotifySettings, now time.Time) int64 {
	lt := now.In(cfg.loc())
	m := minuteOf(cfg.SummaryAt)
	slot := time.Date(lt.Year(), lt.Month(), lt.Day(), m/60, m%60, 0, 0, lt.Location())
	if slot.After(lt) {
		slot = slot.AddDate(0, 0, -1)
	}
	return slot.Unix()
}

// summary: once a day, at the chosen time (or when the quiet hours end),
// what is new and still true. Caller holds n.mu.
func (n *notifyState) summary(cfg *NotifySettings, at time.Time) {
	slot := summarySlot(cfg, at)
	if n.f.LastSlot == 0 {
		n.f.LastSlot, n.dirty = slot, true
		return
	}
	if slot <= n.f.LastSlot || cfg.inQuiet(at) {
		return
	}
	n.f.LastSlot, n.dirty = slot, true
	cur := n.prevFull
	var sec, reboot, osl, rel []string
	type cveLine struct {
		id, app string
		n       int
	}
	var cves []cveLine
	prio := 0
	use := func(kind string) bool {
		ev := cfg.Events[kind]
		if ev.On && ev.Prio > prio {
			prio = ev.Prio
		}
		return ev.On
	}
	keys := make([]string, 0, len(n.f.Summary))
	for k := range n.f.Summary {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		it := n.f.Summary[k]
		var d *ntfDev
		if cur != nil && it.Dev != "" {
			d = cur.devs[it.Dev]
		}
		switch it.Kind {
		case "security":
			if d != nil && d.Security > 0 && use("security") {
				sec = append(sec, fmt.Sprintf("%s (%d)", d.Name, d.Security))
			}
		case "reboot":
			if d != nil && d.Reboot && use("reboot") {
				reboot = append(reboot, d.Name)
			}
		case "os":
			if d != nil && d.OS != "" && use("os") {
				osl = append(osl, d.Name+": "+it.Text)
			}
		case "cve":
			still := 0
			for _, id := range it.Devs {
				if cur != nil {
					if dd := cur.devs[id]; dd != nil {
						if _, ok := dd.CVEs[it.CVE]; ok {
							still++
						}
					}
				}
			}
			if still > 0 && use("cve") {
				cves = append(cves, cveLine{it.CVE, it.App, still})
			}
		case "release":
			if use("release") {
				rel = append(rel, it.Name+" "+it.Text)
			}
		}
	}
	n.f.Summary = map[string]*sumItem{}
	var lines []string
	if len(sec) > 0 {
		lines = append(lines, "Security updates: "+strings.Join(sec, ", "))
	}
	if len(cves) > 0 {
		sort.Slice(cves, func(i, j int) bool {
			return cves[i].n > cves[j].n || (cves[i].n == cves[j].n && cves[i].id > cves[j].id)
		})
		var parts []string
		for i, c := range cves {
			if i == 8 {
				parts = append(parts, fmt.Sprintf("and %d more", len(cves)-8))
				break
			}
			parts = append(parts, fmt.Sprintf("%s (%s on %d device%s)", c.id, c.app, c.n, map[bool]string{true: "", false: "s"}[c.n == 1]))
		}
		lines = append(lines, "New serious CVEs: "+strings.Join(parts, ", "))
	}
	if len(reboot) > 0 {
		lines = append(lines, "Reboot required: "+strings.Join(reboot, ", "))
	}
	if len(osl) > 0 {
		lines = append(lines, "OS support: "+strings.Join(osl, "; "))
	}
	if len(rel) > 0 {
		lines = append(lines, "New release: "+strings.Join(rel, ", "))
	}
	if len(lines) == 0 {
		return
	}
	click := ""
	if !cfg.NoLink && cfg.PublicURL != "" {
		click = cfg.PublicURL + "/#/dashboard/overview"
	}
	n.queue(cfg, "summary", "Coremon daily summary", strings.Join(lines, "\n"), prio, []string{"memo"}, click, at)
}

// deliver sends what is waiting, oldest first, per target; nothing is
// locked while a message is on its way.
func (s *Server) deliver(cfg *NotifySettings, at time.Time) {
	n := &s.ntf
	for _, t := range cfg.Targets {
		for {
			n.mu.Lock()
			if !t.On {
				// Switched off: nothing is kept.
				n.dropTarget(t.ID)
				n.mu.Unlock()
				break
			}
			n.expire(t.ID, at)
			if next := n.nextTry[t.ID]; at.Before(next) {
				n.mu.Unlock()
				break
			}
			var m *notifyMsg
			for i := range n.f.Queue {
				if n.f.Queue[i].Target == t.ID {
					c := n.f.Queue[i]
					m = &c
					break
				}
			}
			if m == nil {
				n.outageNote(cfg, t, at)
				n.mu.Unlock()
				break
			}
			send := n.send
			n.mu.Unlock()

			perm, err := send(t, *m)

			n.mu.Lock()
			o := n.f.Outage[t.ID]
			switch {
			case err == nil:
				n.remove(m.Seq)
				st := "sent"
				if o != nil {
					o.Late++
					o.OK = true
					st = "delayed"
				}
				n.logEntry(notifyLogEntry{At: time.Now().Unix(), Target: t.ID, Event: m.Event, Title: m.Title, Status: st})
			case perm:
				n.remove(m.Seq)
				n.logEntry(notifyLogEntry{At: time.Now().Unix(), Target: t.ID, Event: m.Event, Title: m.Title, Status: "failed", Error: err.Error()})
			default:
				if o == nil {
					n.f.Outage[t.ID] = &ntfOutage{From: at.Unix()}
					n.logEntry(notifyLogEntry{At: time.Now().Unix(), Target: t.ID, Event: m.Event, Title: m.Title, Status: "failed", Error: err.Error() + " (kept, retried every minute for up to 24 hours)"})
				}
				n.nextTry[t.ID] = at.Add(notifyRetry)
			}
			n.dirty = true
			done := err != nil && !perm
			n.mu.Unlock()
			if done {
				break
			}
		}
	}
}

func (n *notifyState) remove(seq int64) {
	for i := range n.f.Queue {
		if n.f.Queue[i].Seq == seq {
			n.f.Queue = append(n.f.Queue[:i], n.f.Queue[i+1:]...)
			return
		}
	}
}

func (n *notifyState) dropTarget(id string) {
	keep := n.f.Queue[:0]
	for _, m := range n.f.Queue {
		if m.Target != id {
			keep = append(keep, m)
		}
	}
	if len(keep) != len(n.f.Queue) || n.f.Outage[id] != nil {
		n.dirty = true
	}
	n.f.Queue = keep
	delete(n.f.Outage, id)
}

// expire: messages older than 24 hours are given up.
func (n *notifyState) expire(id string, at time.Time) {
	cut := at.Add(-notifyKeep).Unix()
	keep := n.f.Queue[:0]
	for _, m := range n.f.Queue {
		if m.Target == id && m.At < cut {
			n.logEntry(notifyLogEntry{At: at.Unix(), Target: id, Event: m.Event, Title: m.Title, Status: "dropped", Error: "could not be delivered for 24 hours"})
			if o := n.f.Outage[id]; o != nil {
				o.Dropped++
			}
			continue
		}
		keep = append(keep, m)
	}
	n.f.Queue = keep
}

// outageNote: after an outage, once everything waiting went out: a note
// that messages were late. Caller holds n.mu.
func (n *notifyState) outageNote(cfg *NotifySettings, t *NotifyTarget, at time.Time) {
	o := n.f.Outage[t.ID]
	if o == nil || (!o.OK && o.Dropped == 0) {
		return
	}
	delete(n.f.Outage, t.ID)
	if o.Late == 0 && o.Dropped == 0 {
		return
	}
	body := fmt.Sprintf("The %s server could not be reached from %s to %s.", serviceName(t.Type), clockText(cfg, o.From), clockText(cfg, at.Unix()))
	if o.Late > 0 {
		body += fmt.Sprintf(" %d message%s arrived late.", o.Late, map[bool]string{true: "", false: "s"}[o.Late == 1])
	}
	if o.Dropped > 0 {
		body += fmt.Sprintf(" %d message%s could not be delivered within 24 hours and %s lost.", o.Dropped, map[bool]string{true: "", false: "s"}[o.Dropped == 1], map[bool]string{true: "was", false: "were"}[o.Dropped == 1])
	}
	n.f.Seq++
	n.f.Queue = append(n.f.Queue, notifyMsg{Seq: n.f.Seq, Target: t.ID, Event: "outage", At: at.Unix(), Title: "Notifications were delayed", Body: body, Prio: 3, Tags: []string{"hourglass"}})
	n.dirty = true
}

func serviceName(typ string) string {
	if typ == notifyNtfy {
		return "ntfy"
	}
	return typ
}
