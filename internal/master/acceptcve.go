package master

// Accepted CVEs: an admin can accept a vulnerability for all devices, with
// a note, for example when it does not apply to how a service is used. An
// accepted CVE stays listed (dimmed, with who, when and the note) but no
// longer counts: not for the red CVE badge, the counts or the device state.
// Accepting can be undone. Once a CVE is on no device any more (after a
// complete check), its acceptance is forgotten.

import (
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

type AcceptedCVE struct {
	By   string `json:"by"`
	At   int64  `json:"at"`
	Note string `json:"note,omitempty"`
}

const (
	maxAcceptedCVEs = 2000
	maxAcceptNote   = 200
)

// cveIDRe: a vulnerability ID as the data sources write it (CVE-2024-6387,
// GHSA-xxxx-xxxx-xxxx, …).
var cveIDRe = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}-[A-Za-z0-9][A-Za-z0-9:-]{2,60}$`)

func checkCVEID(id string) (string, error) {
	id = strings.TrimSpace(id)
	if !cveIDRe.MatchString(id) {
		return "", errors.New("not a vulnerability ID like CVE-2024-6387")
	}
	return id, nil
}

func (s *Server) handleCVEAccept(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Note string `json:"note"`
	}
	if !readJSON(w, r, &req, 4096) {
		return
	}
	id, err := checkCVEID(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	note := cleanText(strings.TrimSpace(req.Note), maxAcceptNote)
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	if s.st.st.AcceptedCVEs == nil {
		s.st.st.AcceptedCVEs = map[string]*AcceptedCVE{}
	}
	if _, ok := s.st.st.AcceptedCVEs[id]; !ok && len(s.st.st.AcceptedCVEs) >= maxAcceptedCVEs {
		http.Error(w, "too many accepted vulnerabilities", http.StatusConflict)
		return
	}
	s.st.st.AcceptedCVEs[id] = &AcceptedCVE{By: actor(r), At: time.Now().Unix(), Note: note}
	s.st.markDirty()
	detail := id
	if note != "" {
		detail += ": " + note
	}
	s.audit.write(actor(r), clientIP(r), "cve.accepted", "", detail)
	writeJSON(w, map[string]any{"accepted": s.st.st.AcceptedCVEs[id]})
}

// forgetAccepted drops the acceptance of CVEs that no device has any more
// (known: every CVE in the data after a complete check). Called only after
// a successful complete check, so a failed or partial one forgets nothing.
func (s *Server) forgetAccepted(known []string) {
	in := make(map[string]bool, len(known))
	for _, id := range known {
		in[id] = true
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	var gone []string
	for id := range s.st.st.AcceptedCVEs {
		if !in[id] {
			gone = append(gone, id)
		}
	}
	if len(gone) == 0 {
		return
	}
	sort.Strings(gone)
	for _, id := range gone {
		delete(s.st.st.AcceptedCVEs, id)
	}
	s.st.markDirty()
	s.audit.write("scheduler", "", "cve.forgotten", "", "no longer on any device: "+strings.Join(gone, ", "))
}

func (s *Server) handleCVEUnaccept(w http.ResponseWriter, r *http.Request) {
	id, err := checkCVEID(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	if s.st.st.AcceptedCVEs[id] == nil {
		http.Error(w, id+" is not accepted", http.StatusNotFound)
		return
	}
	delete(s.st.st.AcceptedCVEs, id)
	s.st.markDirty()
	s.audit.write(actor(r), clientIP(r), "cve.unaccepted", "", id)
	writeJSON(w, map[string]bool{"accepted": false})
}
