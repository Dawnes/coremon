package master

// The lowest allowed agent version: older agents may only be updated.

import (
	"fmt"
	"log"

	"coremon/internal/proto"
)

func (s *Server) oldestStored() string {
	vs := s.repo.coreVersions()
	if len(vs) == 0 {
		return ""
	}
	return vs[len(vs)-1]
}

func (s *Server) minAgent() string {
	min, oldest := s.st.st.Settings.MinAgentVersion, s.oldestStored()
	if min == "" || (oldest != "" && versionLess(min, oldest)) {
		return oldest
	}
	return min
}

func (s *Server) belowMin(a *Agent) bool {
	min := s.minAgent()
	return min != "" && a.CoreVersion != "" && versionLess(a.CoreVersion, min)
}

func upgradeOnlyJob(t string) bool {
	return t == proto.CmdCoreUpdate || t == proto.CmdUninstall || t == proto.CmdSetPins
}

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
