package master

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"coremon/internal/proto"
	"coremon/internal/seal"
)

const pingWait = 15 * time.Second

type pingResult struct {
	Answered   bool      `json:"answered"`
	JobID      string    `json:"job_id"`
	RTTMs      int64     `json:"rtt_ms"`
	Version    string    `json:"version,omitempty"`
	KeyUsed    string    `json:"key_used,omitempty"`
	KeyCurrent bool      `json:"key_current"`
	Slots      [2]string `json:"slots"`
	SlotsOK    bool      `json:"slots_ok"`
	ClockMs    int64     `json:"clock_ms"`
	Error      string    `json:"error,omitempty"`
	LastSeen   int64     `json:"last_seen"`
	Online     bool      `json:"online"`
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	a := s.agentOr404(w, r)
	if a == nil {
		s.st.mu.Unlock()
		return
	}
	if a.Removing {
		s.st.mu.Unlock()
		http.Error(w, "this system is being removed", http.StatusConflict)
		return
	}
	if s.belowMin(a) {
		s.st.mu.Unlock()
		http.Error(w, upgradeOnlyMsg(a, s.minAgent()), http.StatusConflict)
		return
	}
	if s.openJob(a.ID, proto.CmdPing, "") {
		s.st.mu.Unlock()
		http.Error(w, "a ping to this system is already running", http.StatusConflict)
		return
	}
	j := s.enqueue(a, &Job{Type: proto.CmdPing, Actor: actor(r), Timeout: int(pingWait / time.Second)})
	ch := make(chan proto.Result, 1)
	if s.pings == nil {
		s.pings = map[string]chan proto.Result{}
	}
	s.pings[j.ID] = ch
	agentID := a.ID
	s.st.mu.Unlock()
	sent := time.Now()

	timer := time.NewTimer(pingWait)
	defer timer.Stop()
	var res proto.Result
	answered := false
	select {
	case res = <-ch:
		answered = true
	case <-timer.C:
	case <-r.Context().Done():
	}
	rtt := time.Since(sent)

	s.st.mu.Lock()
	delete(s.pings, j.ID)
	if !answered {
		if !j.done() {
			j.Status, j.Finished, j.Error = JobCancelled, time.Now().Unix(), "no answer within 15 s"
			s.st.markDirty()
		}
	}
	out := pingResult{Answered: answered, JobID: j.ID}
	if a := s.st.st.Agents[agentID]; a != nil {
		out.LastSeen, out.Online = a.LastSeen, s.online(a, time.Now().Unix())
	}
	s.st.mu.Unlock()
	if r.Context().Err() != nil {
		return
	}
	if !answered {
		out.Error = "no answer within 15 s"
		writeJSON(w, out)
		return
	}
	if !res.OK {
		out.Error = clip(res.Error, 500)
		writeJSON(w, out)
		return
	}
	var p proto.PingReply
	json.Unmarshal(res.Data, &p)
	want := s.desiredPins()
	out.RTTMs = rtt.Milliseconds()
	out.Version = clip(p.Version, 32)

	out.KeyUsed = shortPin(seal.Fingerprint(p.KeyUsed))
	out.KeyCurrent = p.KeyUsed != "" && p.KeyUsed == want[0]
	out.Slots = [2]string{shortPin(seal.Fingerprint(p.Keys[0])), shortPin(seal.Fingerprint(p.Keys[1]))}
	out.SlotsOK = samePair(p.Keys, want)

	if p.TimeMs > 0 {
		out.ClockMs = p.TimeMs - sent.Add(rtt/2).UnixMilli()
	}
	writeJSON(w, out)
}

func samePair(a, b [2]string) bool {
	return (a[0] == b[0] && a[1] == b[1]) || (a[0] == b[1] && a[1] == b[0])
}

func (s *Server) pingAnswered(j *Job, res proto.Result) {
	if ch := s.pings[j.ID]; ch != nil {
		select {
		case ch <- res:
		default:
		}
	}
	line := "no answer data\n"
	if !res.OK {
		line = "refused: " + clip(res.Error, 500) + "\n"
	} else {
		var p proto.PingReply
		if json.Unmarshal(res.Data, &p) == nil {
			line = fmt.Sprintf("pong from agent %s (connection key %s)\n", clip(p.Version, 32), shortPin(seal.Fingerprint(p.KeyUsed)))
		}
	}
	if f, err := os.OpenFile(s.st.jobLogPath(j.ID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		n, _ := f.WriteString(line)
		f.Close()
		j.LogSize += int64(n)
	}
}
