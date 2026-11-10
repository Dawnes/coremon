package master

// The lowest allowed agent version (Settings > General). An agent below it
// that can still connect (0.3.0+) is "upgrade only": the master accepts its
// heartbeat for the version and platform, stores none of its data, and only
// lets an agent update, uninstall or key-slot change through. Agents that
// cannot connect at all (before 0.3.0) are not processed or offered.

import (
	"fmt"
	"log"

	"coremon/internal/proto"
)

// oldestStored is the oldest agent version the master keeps; "" if none.
func (s *Server) oldestStored() string {
	vs := s.repo.coreVersions() // newest first
	if len(vs) == 0 {
		return ""
	}
	return vs[len(vs)-1]
}

// minAgent is the lowest allowed agent version in force: the setting, but
// never below the oldest stored version. Caller holds s.st.mu.
func (s *Server) minAgent() string {
	min, oldest := s.st.st.Settings.MinAgentVersion, s.oldestStored()
	if min == "" || (oldest != "" && versionLess(min, oldest)) {
		return oldest
	}
	return min
}

// tooOldToConnect: agents of the old connection method (before 0.3.0).
func tooOldToConnect(a *Agent) bool {
	return a.CoreVersion != "" && versionLess(a.CoreVersion, minAgentVersion)
}

// belowMin: the agent connects but is older than the minimum, so it may only
// be upgraded. Caller holds s.st.mu.
func (s *Server) belowMin(a *Agent) bool {
	min := s.minAgent()
	return min != "" && a.CoreVersion != "" && !tooOldToConnect(a) && versionLess(a.CoreVersion, min)
}

// upgradeOnlyJob: what an agent below the minimum may still receive.
func upgradeOnlyJob(t string) bool {
	return t == proto.CmdCoreUpdate || t == proto.CmdUninstall || t == proto.CmdSetPins
}

// upgradeOnlyMsg explains why something is refused for an agent below the
// minimum.
func upgradeOnlyMsg(a *Agent, min string) string {
	return fmt.Sprintf("%s: agent %s is older than the lowest allowed version %s — update the agent first", a.Name, a.CoreVersion, min)
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// raiseMinAgent keeps the stored setting at or above the oldest stored
// version (after a new release pushed the oldest out), with an audit entry.
func (s *Server) raiseMinAgent() {
	oldest := s.oldestStored()
	s.st.mu.Lock()
	min := s.st.st.Settings.MinAgentVersion
	if oldest == "" || min == "" || !versionLess(min, oldest) {
		s.st.mu.Unlock()
		return
	}
	s.st.st.Settings.MinAgentVersion = oldest
	s.st.markDirty()
	s.st.mu.Unlock()
	s.audit.write("scheduler", "", "settings.min_agent_raised", "", fmt.Sprintf("lowest allowed agent version %s → %s (%s is no longer stored)", min, oldest, min))
	log.Printf("lowest allowed agent version raised from %s to %s", min, oldest)
}
