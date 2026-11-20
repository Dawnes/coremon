package master

// Network dependencies ("Connected via"): a device can sit behind another
// device, for example a switch or the host of a VM. That is separate from
// "Checked by" (who runs the checks). When a device fails while a device
// above it is down or in maintenance, it is shown as unreachable behind
// that device instead of down: the cause is shown once, at the top. Its
// checks keep running as normal, so it recovers the moment it answers.

import (
	"fmt"
	"strings"
	"time"
)

// devRef: a device as other devices refer to it.
type devRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// viaOf: the device that id is connected via, or "". Caller holds s.st.mu.
func (s *Server) viaOf(id string) string {
	if d := s.st.st.Locals[id]; d != nil {
		return d.Via
	}
	if a := s.st.st.Agents[id]; a != nil {
		return a.Via
	}
	return ""
}

// devRefOf: the name of a device, or nil when it does not exist (or still
// waits for approval). Caller holds s.st.mu.
func (s *Server) devRefOf(id string) *devRef {
	if d := s.st.st.Locals[id]; d != nil {
		return &devRef{ID: d.ID, Name: d.Name}
	}
	if a := s.st.st.Agents[id]; a != nil && a.Status != StatusPending {
		return &devRef{ID: a.ID, Name: a.Name}
	}
	return nil
}

// checkVia: a valid "Connected via" for device id: empty, or an existing
// device that is not id itself and not connected via id (no loops).
// Caller holds s.st.mu.
func (s *Server) checkVia(id, via string) (string, error) {
	via = strings.TrimSpace(via)
	if via == "" {
		return "", nil
	}
	if via == id {
		return "", fmt.Errorf("a device cannot be connected via itself")
	}
	p := s.devRefOf(via)
	if p == nil {
		return "", fmt.Errorf("the device to connect via was not found")
	}
	seen := map[string]bool{}
	for x := s.viaOf(via); x != "" && !seen[x]; x = s.viaOf(x) {
		seen[x] = true
		if x == id {
			return "", fmt.Errorf("%s is connected via this device: choose a device above it, not below it", p.Name)
		}
	}
	return via, nil
}

// viaName: the name of a "Connected via" device for the audit log.
// Caller holds s.st.mu.
func (s *Server) viaName(id string) string {
	if id == "" {
		return "none"
	}
	if p := s.devRefOf(id); p != nil {
		return p.Name
	}
	return id
}

// viaChain: the devices above id, nearest first. Caller holds s.st.mu.
func (s *Server) viaChain(id string) []devRef {
	var out []devRef
	seen := map[string]bool{id: true}
	for x := s.viaOf(id); x != "" && !seen[x]; x = s.viaOf(x) {
		seen[x] = true
		p := s.devRefOf(x)
		if p == nil {
			break
		}
		out = append(out, *p)
	}
	return out
}

// dropVia: a removed device no longer carries others. Caller holds s.st.mu.
func (s *Server) dropVia(id string) {
	for _, d := range s.st.st.Locals {
		if d.Via == id {
			d.Via = ""
		}
	}
	for _, a := range s.st.st.Agents {
		if a.Via == id {
			a.Via = ""
		}
	}
}

// failing: the device itself is down, not in maintenance: an agent that
// stopped reporting, or checks that decided it is down. Unknown (waiting,
// remote probe without results) is not down. Caller holds s.st.mu.
func (s *Server) failing(id string, now int64) bool {
	if d := s.st.st.Locals[id]; d != nil {
		if d.Maint != nil {
			return false
		}
		lv := s.checksView(&d.Checks, false, false)
		return lv.Checked && !lv.Up
	}
	if a := s.st.st.Agents[id]; a != nil {
		return a.Maint == nil && a.Status != StatusPending && !s.online(a, now)
	}
	return false
}

// inMaint: the device is in maintenance. Caller holds s.st.mu.
func (s *Server) inMaint(id string) bool {
	if d := s.st.st.Locals[id]; d != nil {
		return d.Maint != nil
	}
	if a := s.st.st.Agents[id]; a != nil {
		return a.Maint != nil
	}
	return false
}

// blockedBy: the device that cuts id off: walking up from its "Connected
// via", the highest device of the unbroken run of devices that are down
// or in maintenance. nil when the device right above it is fine (or there
// is none). Caller holds s.st.mu.
func (s *Server) blockedBy(id string, now int64) *devRef {
	var cause string
	seen := map[string]bool{id: true}
	for x := s.viaOf(id); x != "" && !seen[x]; x = s.viaOf(x) {
		seen[x] = true
		if !s.inMaint(x) && !s.failing(x, now) {
			break
		}
		cause = x
	}
	if cause == "" {
		return nil
	}
	return s.devRefOf(cause)
}

// unreachable: the device fails (down) while a device above it is down or
// in maintenance: the device behind which it is cut off, or nil.
// down: whether the device itself counts as down. Caller holds s.st.mu.
func (s *Server) unreachable(id string, down bool) *devRef {
	if !down {
		return nil
	}
	return s.blockedBy(id, time.Now().Unix())
}

// noteBehind: a check of device id just failed (its result is the last
// slot of hist). When a device above it is down or in maintenance, the
// failure is counted as unreachable in that slot of the 24-hour history.
// Caller holds s.st.mu.
func (s *Server) noteBehind(id string, failed bool, hist []histSlot, now time.Time) {
	if !failed || len(hist) == 0 || s.blockedBy(id, now.Unix()) == nil {
		return
	}
	hist[len(hist)-1].U++
}
