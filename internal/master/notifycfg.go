package master

// Notification settings (Settings → Notifications): targets that messages
// go to (for now only ntfy), the events that send one, and the options
// shared by every target.

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time" // time zones from the system (tzdata, always on Debian)
)

const (
	notifyNtfy = "ntfy"

	authNone  = "none"
	authToken = "token"
	authBasic = "basic"

	defaultNtfyServer = "https://ntfy.sh"
	defaultThrottleN  = 3
	defaultThrottleM  = 15
	maxThrottleN      = 20
	maxThrottleM      = 240
)

// NotifyTarget: one place messages go to. Type says which service; each
// type uses its own fields.
type NotifyTarget struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	On   bool   `json:"on"`
	// ntfy
	Server   string `json:"server,omitempty"`
	Topic    string `json:"topic,omitempty"`
	Auth     string `json:"auth,omitempty"`
	Token    string `json:"token,omitempty"`
	User     string `json:"user,omitempty"`
	Password string `json:"password,omitempty"`
}

// NotifyEvent: whether an event sends a message, and with which priority
// (1 min, 2 low, 3 default, 4 high, 5 urgent).
type NotifyEvent struct {
	On   bool `json:"on"`
	Prio int  `json:"prio"`
}

type NotifySettings struct {
	Targets []*NotifyTarget        `json:"targets,omitempty"`
	Events  map[string]NotifyEvent `json:"events,omitempty"`
	// AlsoHidden: also for devices that are unreachable behind another
	// device (devices in maintenance never send anything).
	AlsoHidden bool   `json:"also_hidden,omitempty"`
	NoLink     bool   `json:"no_link,omitempty"`
	PublicURL  string `json:"public_url,omitempty"`
	Quiet      bool   `json:"quiet,omitempty"`
	QuietFrom  string `json:"quiet_from,omitempty"`
	QuietTo    string `json:"quiet_to,omitempty"`
	SummaryAt  string `json:"summary_at,omitempty"`
	TZ         string `json:"tz,omitempty"`
	ThrottleN  int    `json:"throttle_n,omitempty"`
	ThrottleM  int    `json:"throttle_min,omitempty"`
}

// notifyEventDef: an event. Summary events go into the daily summary, the
// others are sent right away.
type notifyEventDef struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Summary bool   `json:"summary"`
	On      bool   `json:"on"`
	Prio    int    `json:"prio"`
}

var notifyEventDefs = []notifyEventDef{
	{"device", "A device goes down, and when it is back", false, true, 4},
	{"service", "A service of a device fails, and recovers", false, true, 4},
	{"upstream", "Upstream is offline or partly online, and when it is back", false, true, 5},
	{"pending", "A new device waits for approval", false, false, 3},
	{"security", "New security updates are waiting", true, true, 3},
	{"cve", "A new serious CVE is found", true, true, 4},
	{"reboot", "A reboot is required", true, false, 2},
	{"os", "OS support ends or is about to end", true, true, 3},
	{"release", "A new Coremon release is available", true, false, 1},
}

func notifyEventDefOf(key string) *notifyEventDef {
	for i := range notifyEventDefs {
		if notifyEventDefs[i].Key == key {
			return &notifyEventDefs[i]
		}
	}
	return nil
}

var (
	ntfyTopicRe = regexp.MustCompile(`^[-_A-Za-z0-9]{1,64}$`)
	hhmmRe      = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	// tzNameRe: an IANA time zone name (Europe/Amsterdam). One the master
	// does not know counts as UTC (tzKnown).
	tzNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+\-]{0,30}(/[A-Za-z0-9_+\-]{1,30}){0,2}$`)
)

func tzKnown(name string) bool {
	_, err := time.LoadLocation(name)
	return err == nil
}

func (n *NotifySettings) normalize() {
	var nt *NotifyTarget
	for _, t := range n.Targets {
		if t.Type == notifyNtfy && nt == nil {
			nt = t
		}
	}
	if nt == nil {
		nt = &NotifyTarget{ID: notifyNtfy, Type: notifyNtfy}
	}
	nt.ID = notifyNtfy
	if nt.Server == "" {
		nt.Server = defaultNtfyServer
	}
	switch nt.Auth {
	case authToken, authBasic:
	default:
		nt.Auth = authNone
	}
	n.Targets = []*NotifyTarget{nt}
	if n.Events == nil {
		n.Events = map[string]NotifyEvent{}
	}
	for _, d := range notifyEventDefs {
		e, ok := n.Events[d.Key]
		if !ok {
			e = NotifyEvent{On: d.On, Prio: d.Prio}
		}
		e.Prio = clamp(e.Prio, 1, 5)
		n.Events[d.Key] = e
	}
	for k := range n.Events {
		if notifyEventDefOf(k) == nil {
			delete(n.Events, k)
		}
	}
	if !hhmmRe.MatchString(n.QuietFrom) {
		n.QuietFrom = "22:00"
	}
	if !hhmmRe.MatchString(n.QuietTo) {
		n.QuietTo = "07:00"
	}
	if !hhmmRe.MatchString(n.SummaryAt) {
		n.SummaryAt = "08:00"
	}
	if !tzNameRe.MatchString(n.TZ) {
		n.TZ = "UTC"
	}
	if n.ThrottleN == 0 {
		n.ThrottleN = defaultThrottleN
	}
	if n.ThrottleM == 0 {
		n.ThrottleM = defaultThrottleM
	}
	n.ThrottleN = clamp(n.ThrottleN, 1, maxThrottleN)
	n.ThrottleM = clamp(n.ThrottleM, 1, maxThrottleM)
}

func (n *NotifySettings) target(id string) *NotifyTarget {
	for _, t := range n.Targets {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// anyOn: some target is switched on.
func (n *NotifySettings) anyOn() bool {
	for _, t := range n.Targets {
		if t.On {
			return true
		}
	}
	return false
}

func (n *NotifySettings) loc() *time.Location {
	l, err := time.LoadLocation(n.TZ)
	if err != nil {
		return time.UTC
	}
	return l
}

// minuteOf: "22:30" → 1350.
func minuteOf(hhmm string) int {
	var h, m int
	fmt.Sscanf(hhmm, "%d:%d", &h, &m)
	return h*60 + m
}

// inQuiet: t falls in the quiet hours (which may pass midnight).
func (n *NotifySettings) inQuiet(t time.Time) bool {
	if !n.Quiet {
		return false
	}
	lt := t.In(n.loc())
	m, from, to := lt.Hour()*60+lt.Minute(), minuteOf(n.QuietFrom), minuteOf(n.QuietTo)
	switch {
	case from == to:
		return false
	case from < to:
		return m >= from && m < to
	}
	return m >= from || m < to
}

// checkNtfyServer: an http(s) address without query or fragment, without
// a trailing slash.
func checkNtfyServer(s string) (string, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if s == "" {
		return defaultNtfyServer, nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("the server must be an address like https://ntfy.sh")
	}
	return s, nil
}

// checkPublicURL: the address of this master as phones can open it; "" is
// allowed (messages then have no link).
func checkPublicURL(s string) (string, error) {
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if s == "" {
		return "", nil
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("the public address must be like https://coremon.example.org")
	}
	return s, nil
}
