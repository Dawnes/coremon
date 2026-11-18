package master

// Upcoming tasks (Activity > Jobs > Upcoming tasks): the next run of each
// of the master's own scheduled tasks, and the next agentless checks.

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const upcomingChecks = 10

type upcomingTask struct {
	Key    string `json:"key"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	At     int64  `json:"at"` // unix seconds; at or before now means "now"
}

type upcomingCheck struct {
	DeviceID string `json:"device_id"`
	Device   string `json:"device"`
	Check    string `json:"check"`
	Detail   string `json:"detail,omitempty"`
	At       int64  `json:"at"`
	Running  bool   `json:"running,omitempty"`
}

// soonest: the later of t and now (a task that is overdue runs at the next tick).
func soonest(t, now time.Time) time.Time {
	if t.Before(now) {
		return now
	}
	return t
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return itoa(n) + " " + word + "s"
}

func itoa(n int) string { return strconv.Itoa(n) }

// masterTasks: each scheduled task of the master once, at its next run.
func (s *Server) masterTasks(now time.Time) []upcomingTask {
	var out []upcomingTask
	add := func(key, title, detail string, at time.Time) {
		out = append(out, upcomingTask{Key: key, Title: title, Detail: detail, At: soonest(at, now).Unix()})
	}

	s.st.mu.Lock()
	set := s.st.st.Settings
	checked := 0
	for _, a := range s.st.st.Agents {
		if a.Status == StatusActive && !a.Removing && !s.belowMin(a) {
			checked++
		}
	}
	roundAt := time.Unix(s.st.st.CheckRoundAt, 0).Add(time.Duration(set.CheckIntervalHours) * time.Hour)
	s.st.mu.Unlock()
	add("check_round", "Update check", plural(checked, "device")+" · every "+plural(set.CheckIntervalHours, "hour"), roundAt)

	if set.BackupSchedule != "off" {
		s.bk.idxMu.Lock()
		last := s.loadBackupIndex().LastAuto
		s.bk.idxMu.Unlock()
		from := now
		if last > 0 {
			from = time.Unix(last, 0)
		}
		if next := nextBackupAfter(set.BackupSchedule, from); !next.IsZero() {
			add("backup", "Automatic backup", set.BackupSchedule, next)
		}
	}

	if s.indexURL() != "" {
		st := s.idx.status()
		next := now
		if st.CheckedAt > 0 {
			next = time.Unix(st.CheckedAt, 0).Add(indexEvery)
		}
		add("releases", "Release check", "new master and agent versions · every 6 hours", next)
	}

	s.vul.mu.Lock()
	d := s.vul.d
	s.vul.mu.Unlock()
	full := time.Unix(d.LastRun, 0).Add(time.Duration(set.VulnDays) * 24 * time.Hour)
	kev := time.Unix(d.KEVAt, 0).Add(vulnKEVEvery)
	vNext, vDetail := full, "full update of the known vulnerabilities"
	if kev.Before(full) {
		vNext, vDetail = kev, "list of actively exploited vulnerabilities"
	}
	if d.Error != "" {
		if retry := time.Unix(d.LastTry, 0).Add(vulnRetry); retry.After(vNext) {
			vNext, vDetail = retry, "retry after an error"
		}
	}
	add("vulns", "Vulnerability data", vDetail, vNext)

	s.osi.mu.Lock()
	f := s.osi.f
	s.osi.mu.Unlock()
	oNext := now
	if f.Data != nil {
		oNext = time.Unix(f.Data.FetchedAt, 0).Add(time.Duration(set.OSInfoDays) * 24 * time.Hour)
	}
	if f.Error != "" {
		if retry := time.Unix(f.LastTry, 0).Add(osInfoRetry); retry.After(oNext) {
			oNext = retry
		}
	}
	add("osinfo", "OS support data", "end-of-life dates of Debian and Ubuntu releases", oNext)

	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

// nextChecks: the next agentless checks (ping and services), soonest first.
func (s *Server) nextChecks(now time.Time) []upcomingCheck {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	s.loc.mu.Lock()
	defer s.loc.mu.Unlock()
	var out []upcomingCheck
	for _, d := range s.checkDevs() {
		if d.Probe != "" || d.Maint != nil {
			continue // checked by a remote probe on its own schedule, or in maintenance
		}
		if d.pinging() {
			if next, ok := s.loc.next[d.ID]; ok {
				out = append(out, upcomingCheck{DeviceID: d.ID, Device: d.Name, Check: "Ping", Detail: "every " + itoa(s.st.st.Local.IntervalSec) + " s",
					At: soonest(next, now).Unix(), Running: s.loc.busy[d.ID]})
			}
		}
		for _, sv := range d.Services {
			if sv.Paused {
				continue
			}
			next, ok := s.loc.next[sv.ID]
			if !ok {
				continue
			}
			detail := strings.ToUpper(sv.kind()) + " " + itoa(sv.Port)
			if sv.Fails > 0 && !(sv.Checked && !sv.Up) {
				detail += " · retry " + itoa(sv.Fails) + " of " + itoa(sv.Retries)
			}
			out = append(out, upcomingCheck{DeviceID: d.ID, Device: d.Name, Check: sv.Name, Detail: detail,
				At: soonest(next, now).Unix(), Running: s.loc.busy[sv.ID]})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].At != out[j].At {
			return out[i].At < out[j].At
		}
		if out[i].Device != out[j].Device {
			return strings.ToLower(out[i].Device) < strings.ToLower(out[j].Device)
		}
		return out[i].Check < out[j].Check
	})
	if len(out) > upcomingChecks {
		out = out[:upcomingChecks]
	}
	return out
}

func (s *Server) handleUpcoming(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	writeJSON(w, map[string]any{"now": now.Unix(), "tasks": s.masterTasks(now), "checks": s.nextChecks(now)})
}
