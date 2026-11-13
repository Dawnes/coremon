package master

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// auditLog is an append-only JSON-lines file of every user-visible action.
// Entries older than the retention period are purged.
type auditLog struct {
	mu   sync.Mutex
	path string
}

type auditEntry struct {
	Time   int64  `json:"time"`
	Actor  string `json:"actor"`
	IP     string `json:"ip,omitempty"`
	Action string `json:"action"`
	Agent  string `json:"agent,omitempty"` // system the entry is about
	Target string `json:"target,omitempty"`
	Detail string `json:"detail,omitempty"`
	Job    string `json:"job,omitempty"` // job started by this action (its output)
}

// write records an entry that is not about one system.
func (a *auditLog) write(actor, ip, action, target, detail string) {
	a.add(auditEntry{Actor: actor, IP: ip, Action: action, Target: target, Detail: detail})
}

// writeAgent records an entry about one system.
func (a *auditLog) writeAgent(actor, ip, action string, ag *Agent, detail string) {
	a.add(auditEntry{Actor: actor, IP: ip, Action: action, Agent: ag.ID, Target: ag.Name, Detail: detail})
}

// writeJob records an action about one system that started a job.
func (a *auditLog) writeJob(actor, ip, action string, ag *Agent, detail string, j *Job) {
	e := auditEntry{Actor: actor, IP: ip, Action: action, Agent: ag.ID, Target: ag.Name, Detail: detail}
	if j != nil {
		e.Job = j.ID
	}
	a.add(e)
}

func (a *auditLog) add(e auditEntry) {
	e.Time = time.Now().Unix()
	b, _ := json.Marshal(e)
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	f.Write(append(b, '\n'))
	f.Close()
}

// about reports whether the entry concerns the given system. Entries from
// before 0.2 have no agent field but carried the agent ID as target.
func (e *auditEntry) about(agentID string) bool {
	return e.Agent == agentID || (e.Agent == "" && e.Target == agentID)
}

const maxAuditRead = 32 << 20

// list returns the newest entries first, optionally only about one system
// and/or by one actor (user name, "agent" or "scheduler").
func (a *auditLog) list(agentID, actor string, limit int) []auditEntry {
	out, _ := a.page(agentID, actor, 0, 0, limit)
	return out
}

// page returns up to limit entries, newest first, older than a cursor:
// before is the time of the last entry already shown (0: from the newest)
// and skip how many entries with exactly that time were shown already.
// Appends only add newer entries and the cleanup only removes the oldest,
// so the cursor stays valid. more: there are older entries.
func (a *auditLog) page(agentID, actor string, before int64, skip, limit int) (out []auditEntry, more bool) {
	out = []auditEntry{}
	all := a.read(agentID, actor)
	for _, e := range all {
		if before > 0 {
			if e.Time > before {
				continue
			}
			if e.Time == before && skip > 0 {
				skip--
				continue
			}
		}
		if len(out) == limit {
			return out, true
		}
		out = append(out, e)
	}
	return out, false
}

// read returns the matching entries, newest first.
func (a *auditLog) read(agentID, actor string) []auditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []auditEntry{}
	f, err := os.Open(a.path)
	if err != nil {
		return out
	}
	defer f.Close()
	st, _ := f.Stat()
	off := int64(0)
	if st.Size() > maxAuditRead {
		off = st.Size() - maxAuditRead
	}
	f.Seek(off, io.SeekStart)
	b, _ := io.ReadAll(f)
	lines := bytes.Split(b, []byte("\n"))
	if off > 0 && len(lines) > 0 {
		lines = lines[1:] // first line may be partial
	}
	for i := len(lines) - 1; i >= 0; i-- {
		var e auditEntry
		if json.Unmarshal(lines[i], &e) != nil {
			continue
		}
		if agentID != "" && !e.about(agentID) {
			continue
		}
		if actor != "" && !strings.EqualFold(e.Actor, actor) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// countOlder counts entries older than cutoff.
func (a *auditLog) countOlder(cutoff int64) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.Open(a.path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	for sc.Scan() {
		var e auditEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Time < cutoff {
			n++
		}
	}
	return n
}

// purge drops entries older than cutoff by rewriting the file.
func (a *auditLog) purge(cutoff int64) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.Open(a.path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var keep bytes.Buffer
	removed := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	for sc.Scan() {
		line := sc.Bytes()
		var e auditEntry
		if json.Unmarshal(line, &e) == nil && e.Time < cutoff {
			removed++
			continue
		}
		if len(bytes.TrimSpace(line)) > 0 {
			keep.Write(line)
			keep.WriteByte('\n')
		}
	}
	f.Close()
	if err := sc.Err(); err != nil || removed == 0 {
		return 0, err
	}
	tmp := a.path + ".tmp"
	if err := os.WriteFile(tmp, keep.Bytes(), 0o600); err != nil {
		return 0, err
	}
	return removed, os.Rename(tmp, a.path)
}
