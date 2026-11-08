package master

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

// auditLog is an append-only JSON-lines file of every user-visible action.
type auditLog struct {
	mu   sync.Mutex
	path string
}

type auditEntry struct {
	Time   int64  `json:"time"`
	Actor  string `json:"actor"`
	IP     string `json:"ip,omitempty"`
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
	Detail string `json:"detail,omitempty"`
}

func (a *auditLog) write(actor, ip, action, target, detail string) {
	b, _ := json.Marshal(auditEntry{Time: time.Now().Unix(), Actor: actor, IP: ip, Action: action, Target: target, Detail: detail})
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	f.Write(append(b, '\n'))
	f.Close()
}

// tail returns the newest entries first.
func (a *auditLog) tail(limit int) []auditEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []auditEntry{}
	f, err := os.Open(a.path)
	if err != nil {
		return out
	}
	defer f.Close()
	st, _ := f.Stat()
	const window = 512 * 1024
	off := int64(0)
	if st.Size() > window {
		off = st.Size() - window
	}
	f.Seek(off, io.SeekStart)
	b, _ := io.ReadAll(f)
	lines := bytes.Split(b, []byte("\n"))
	if off > 0 && len(lines) > 0 {
		lines = lines[1:] // first line may be partial
	}
	for i := len(lines) - 1; i >= 0 && len(out) < limit; i-- {
		var e auditEntry
		if json.Unmarshal(lines[i], &e) == nil {
			out = append(out, e)
		}
	}
	return out
}
