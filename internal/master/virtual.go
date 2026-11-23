package master

// Virtual devices: a place in the network that Coremon cannot check, such as
// an unmanaged switch, so that "Connected via" can still describe the
// network. A virtual device has a name, a group and a "Connected via", but
// no address and no checks. It never causes "unreachable" itself: it passes
// on the state of the devices above it.
//
// One virtual device is special: Upstream (the internet connection). It is
// only created and removed in Settings → Upstream, where the master also
// pings a few public addresses to show whether the internet answers (see
// internetcheck.go). It is the top of the network map.

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type VirtualDevice struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	GroupID string `json:"group_id"`
	Order   int    `json:"order"`
	AddedAt int64  `json:"added_at"`
	Via     string `json:"via,omitempty"`
	// Special: "upstream" for the internet connection (Settings → Upstream).
	Special string `json:"special,omitempty"`
}

const (
	maxVirtualDevices = 200
	specialUpstream   = "upstream"
	upstreamID        = "v_upstream"
)

// virtualView: a virtual device as the website sees it.
type virtualView struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	GroupID string  `json:"group_id"`
	Order   int     `json:"order"`
	Via     *devRef `json:"via,omitempty"`
	// ViaChain: the devices above it, nearest first.
	ViaChain []devRef `json:"via_chain,omitempty"`
	Special  string   `json:"special,omitempty"`
	Unreach  *devRef  `json:"unreach,omitempty"` // a device above it is down or in maintenance
	Monitor  string   `json:"monitor"`           // always "virtual"
}

// virtualViews: every virtual device. Caller holds s.st.mu.
func (s *Server) virtualViews() []virtualView {
	out := []virtualView{}
	now := time.Now().Unix()
	for _, v := range s.st.st.Virtuals {
		x := virtualView{ID: v.ID, Name: v.Name, GroupID: v.GroupID, Order: v.Order, Special: v.Special, Monitor: "virtual"}
		if v.Via != "" {
			x.Via = s.devRefOf(v.Via)
			x.ViaChain = s.viaChain(v.ID)
		}
		if v.Special == "" {
			x.Unreach = s.blockedBy(v.ID, now)
		}
		out = append(out, x)
	}
	sortByName(out, func(v virtualView) string { return v.Name })
	return out
}

// isPlainVirtual: id is a virtual device other than Upstream. Caller holds s.st.mu.
func (s *Server) isPlainVirtual(id string) bool {
	v := s.st.st.Virtuals[id]
	return v != nil && v.Special == ""
}

func sortByName[T any](xs []T, name func(T) string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && strings.ToLower(name(xs[j])) < strings.ToLower(name(xs[j-1])); j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

func (s *Server) virtualOr404(w http.ResponseWriter, r *http.Request) *VirtualDevice {
	v := s.st.st.Virtuals[r.PathValue("id")]
	if v == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return nil
	}
	if v.Special != "" {
		http.Error(w, "this device is managed in Settings → Upstream", http.StatusConflict)
		return nil
	}
	return v
}

type virtualReq struct {
	Name    *string `json:"name"`
	GroupID *string `json:"group_id"`
	Via     *string `json:"via"`
}

func (s *Server) handleVirtualCreate(w http.ResponseWriter, r *http.Request) {
	var req virtualReq
	if !readJSON(w, r, &req, 4096) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	if len(s.st.st.Virtuals) >= maxVirtualDevices {
		http.Error(w, fmt.Sprintf("at most %d virtual devices", maxVirtualDevices), http.StatusBadRequest)
		return
	}
	v := &VirtualDevice{ID: "v_" + randHex(8), AddedAt: time.Now().Unix()}
	if err := s.applyVirtual(v, req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if v.Name == "" {
		http.Error(w, "a name is required", http.StatusBadRequest)
		return
	}
	v.Order = s.st.nextOrder(v.GroupID)
	s.st.st.Virtuals[v.ID] = v
	s.st.markDirty()
	detail := "virtual device"
	if v.Via != "" {
		detail += ", connected via " + s.viaName(v.Via)
	}
	s.audit.add(auditEntry{Actor: actor(r), IP: clientIP(r), Action: "device.virtual_added", Agent: v.ID, Target: v.Name, Detail: detail})
	writeJSON(w, v)
}

func (s *Server) handleVirtualUpdate(w http.ResponseWriter, r *http.Request) {
	var req virtualReq
	if !readJSON(w, r, &req, 4096) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	v := s.virtualOr404(w, r)
	if v == nil {
		return
	}
	n := *v
	if err := s.applyVirtual(&n, req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var changes []string
	if n.Name != v.Name {
		changes = append(changes, "name "+v.Name+" → "+n.Name)
	}
	if n.Via != v.Via {
		changes = append(changes, "connected via "+s.viaName(v.Via)+" → "+s.viaName(n.Via))
	}
	if n.GroupID != v.GroupID {
		n.Order = s.st.nextOrder(n.GroupID)
	}
	*v = n
	s.st.markDirty()
	if len(changes) > 0 {
		s.audit.add(auditEntry{Actor: actor(r), IP: clientIP(r), Action: "device.virtual_changed", Agent: v.ID, Target: v.Name, Detail: strings.Join(changes, "; ")})
	}
	writeJSON(w, v)
}

func (s *Server) handleVirtualDelete(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	v := s.virtualOr404(w, r)
	if v == nil {
		return
	}
	delete(s.st.st.Virtuals, v.ID)
	s.dropVia(v.ID)
	s.st.markDirty()
	s.audit.add(auditEntry{Actor: actor(r), IP: clientIP(r), Action: "device.virtual_removed", Agent: v.ID, Target: v.Name})
	writeJSON(w, map[string]bool{"removed": true})
}

// gatewayHint: the device whose address is the default gateway of most
// agents: probably the internet router.
type gatewayHint struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	IP    string `json:"ip"`
	Count int    `json:"count"`
	// Done: it is already connected via Upstream.
	Done bool `json:"done"`
}

// gateway: caller holds s.st.mu. nil when no gateway belongs to a device.
func (s *Server) gateway() *gatewayHint {
	count := map[string]int{}
	for _, a := range s.st.st.Agents {
		if a.Status != StatusPending && a.Facts.Gateway != "" {
			count[canonIP(a.Facts.Gateway)]++
		}
	}
	var best *gatewayHint
	for ip, n := range count {
		if ip == "" {
			continue
		}
		id, _ := s.ipOwner(ip, "", "")
		p := s.devRefOf(id)
		if id == "" || p == nil {
			continue
		}
		if best == nil || n > best.Count || (n == best.Count && ip < best.IP) {
			best = &gatewayHint{ID: p.ID, Name: p.Name, IP: ip, Count: n}
		}
	}
	if best != nil {
		best.Done = s.viaOf(best.ID) == upstreamID
	}
	return best
}

// applyVirtual checks and applies a request. Caller holds s.st.mu.
func (s *Server) applyVirtual(v *VirtualDevice, req virtualReq) error {
	if req.Name != nil {
		n, err := checkName("device name", *req.Name, maxSystemName)
		if err != nil {
			return err
		}
		if n == "" {
			return fmt.Errorf("a name is required")
		}
		v.Name = n
	}
	if req.GroupID != nil {
		if *req.GroupID != "" && s.st.group(*req.GroupID) == nil {
			return fmt.Errorf("group not found")
		}
		v.GroupID = *req.GroupID
	}
	if req.Via != nil {
		via, err := s.checkVia(v.ID, *req.Via)
		if err != nil {
			return err
		}
		v.Via = via
	}
	return nil
}
