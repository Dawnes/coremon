package master

// Settings > General is split into four blocks, each saved on its own
// (POST /api/ui/settings/{block}); a block only changes its own settings
// and writes its own audit entry.
//   - agents:  heartbeat, update check interval, lowest allowed agent version
//   - access:  website port, master URL, 2FA code and trusted networks
//   - lookups: OS support dates, vulnerability checks, GitHub repository
//   - history: audit log and job history retention, received probe results

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

type settingsBlock struct {
	CheckIntervalHours int    `json:"check_interval_hours"`
	HeartbeatSec       int    `json:"heartbeat_sec"`
	MinAgentVersion    string `json:"min_agent_version"`
	UIPort             int    `json:"ui_port"`
	MasterURL          string `json:"master_url"`
	TFAMode            string `json:"tfa_mode"`
	TFATrusted         string `json:"tfa_trusted"`
	TFANever           string `json:"tfa_never"`
	OSInfoDays         int    `json:"os_info_days"`
	VulnDays           int    `json:"vuln_days"`
	GitHubRepo         string `json:"github_repo"`
	RetentionDays      int    `json:"retention_days"`
	ProbeResultsShown  bool   `json:"probe_results_shown"`
}

func (s *Server) handleSettingsBlock(w http.ResponseWriter, r *http.Request) {
	block := r.PathValue("block")
	var req settingsBlock
	if !readJSON(w, r, &req, 8192) {
		return
	}
	bad := func(msg string) { http.Error(w, msg, http.StatusBadRequest) }
	var nets, never []string
	switch block {
	case "agents":
		if req.CheckIntervalHours < 1 || req.CheckIntervalHours > 168 {
			bad("check interval must be 1-168 hours")
			return
		}
		if req.HeartbeatSec < minHeartbeat || req.HeartbeatSec > maxHeartbeat {
			bad(fmt.Sprintf("heartbeat must be %d-%d seconds", minHeartbeat, maxHeartbeat))
			return
		}
	case "access":
		if req.UIPort != 8443 && req.UIPort != 443 {
			bad("website port must be 8443 or 443")
			return
		}
		req.MasterURL = strings.TrimRight(strings.TrimSpace(req.MasterURL), "/")
		if req.MasterURL != "" && !masterURLRe.MatchString(req.MasterURL) {
			bad("master URL must look like https://host:port")
			return
		}
		if req.TFAMode != tfaAlways && req.TFAMode != tfaUntrusted {
			bad("2FA mode must be always or untrusted")
			return
		}
		var err error
		if nets, err = parseTrusted(req.TFATrusted); err != nil {
			bad(err.Error())
			return
		}
		if never, err = parseNever(req.TFANever); err != nil {
			bad("never trust: " + err.Error())
			return
		}
		if len(nets) == 0 {
			req.TFAMode = tfaAlways // no networks: always ask
		}
	case "lookups":
		if !validOSInfoDays(req.OSInfoDays) {
			bad("OS support lookup must be every 3, 14, 30 or 90 days")
			return
		}
		if !validVulnDays(req.VulnDays) {
			bad("vulnerability check must be every 1, 7, 30 or 90 days")
			return
		}
		req.GitHubRepo = strings.Trim(strings.TrimSpace(req.GitHubRepo), "/")
		req.GitHubRepo = strings.TrimPrefix(req.GitHubRepo, "https://github.com/")
		if req.GitHubRepo != "" && !ghRepoRe.MatchString(req.GitHubRepo) {
			bad("GitHub repository must look like owner/name")
			return
		}
	case "history":
		if req.RetentionDays < minRetention || req.RetentionDays > maxRetention {
			bad(fmt.Sprintf("retention must be %d-%d days", minRetention, maxRetention))
			return
		}
	default:
		http.Error(w, "unknown settings block", http.StatusNotFound)
		return
	}

	who, ip := actor(r), clientIP(r)
	s.st.mu.Lock()
	old := s.st.st.Settings
	set := &s.st.st.Settings
	var detail string
	switch block {
	case "agents":
		if req.MinAgentVersion != "" && !contains(s.repo.coreVersions(), req.MinAgentVersion) {
			s.st.mu.Unlock()
			bad("lowest allowed agent version must be one of the versions stored on the master")
			return
		}
		if req.MinAgentVersion != "" {
			set.MinAgentVersion = req.MinAgentVersion
		}
		set.CheckIntervalHours, set.HeartbeatSec = req.CheckIntervalHours, req.HeartbeatSec
		detail = fmt.Sprintf("heartbeat=%ds check=%dh min-agent=%s", set.HeartbeatSec, set.CheckIntervalHours, set.MinAgentVersion)
	case "access":
		set.UIPort, set.MasterURL, set.TFAMode, set.TFATrusted, set.TFANever = req.UIPort, req.MasterURL, req.TFAMode, nets, never
		detail = fmt.Sprintf("port=%d url=%q", set.UIPort, set.MasterURL)
	case "lookups":
		set.OSInfoDays, set.VulnDays, set.GitHubRepo, set.GitHubSeeded = req.OSInfoDays, req.VulnDays, req.GitHubRepo, true
		detail = fmt.Sprintf("os-info=%dd vulns=%dd repo=%q", set.OSInfoDays, set.VulnDays, set.GitHubRepo)
	case "history":
		set.RetentionDays, set.ProbeResultsShown = req.RetentionDays, req.ProbeResultsShown
		detail = fmt.Sprintf("retention=%dd", set.RetentionDays)
	}
	now := *set
	s.st.markDirty()
	s.st.mu.Unlock()

	s.audit.write(who, ip, "settings."+block, "", detail)
	if old.ProbeResultsShown != now.ProbeResultsShown {
		s.audit.write(who, ip, "settings.probe_results", "", "show received probe results on Remote Probes: "+onOff(now.ProbeResultsShown))
	}
	if old.TFAMode != now.TFAMode || strings.Join(old.TFATrusted, ",") != strings.Join(now.TFATrusted, ",") || strings.Join(old.TFANever, ",") != strings.Join(now.TFANever, ",") {
		s.audit.write(who, ip, "settings.2fa_trusted", "", fmt.Sprintf("ask 2FA code: %s; trusted networks: %s; never trust: %s", tfaModeText(now.TFAMode),
			orNone(strings.Join(now.TFATrusted, ", ")), orNone(strings.Join(now.TFANever, ", "))))
	}
	if old.OSInfoDays != now.OSInfoDays {
		s.kickOSInfo()
	}
	if old.VulnDays != now.VulnDays {
		s.kickVulns()
	}
	if old.GitHubRepo != now.GitHubRepo {
		s.kickIndex()
	}
	if old.RetentionDays != now.RetentionDays {
		s.setNextPurge(purgeAfterChange(time.Now()))
	}
	if old.UIPort != now.UIPort && s.handler != nil {
		s.applyUIPort(s.handler)
	}
	res := map[string]any{"settings": now, "next_purge": s.nextPurgeAt().Unix(), "ui_port_error": s.uiPortError()}
	for k, v := range tfaView(now, ip) {
		res[k] = v
	}
	writeJSON(w, res)
}
