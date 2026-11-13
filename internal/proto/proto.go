// Package proto defines the messages between agent and master. The agent always connects out.
package proto

import "encoding/json"

const ProtocolVersion = 5

type Facts struct {
	Hostname  string   `json:"hostname"`
	OSName    string   `json:"os_name"`
	OSID      string   `json:"os_id"`
	OSVersion string   `json:"os_version"`
	Kernel    string   `json:"kernel"`
	OS        string   `json:"os"`
	Arch      string   `json:"arch"`
	Virt      string   `json:"virt"`
	UptimeSec int64    `json:"uptime_sec"`
	CPUs      int      `json:"cpus"`
	MemTotal  uint64   `json:"mem_total"`
	IPs       []string `json:"ips"`

	Gateway   string   `json:"gateway,omitempty"`
	GatewayIf string   `json:"gateway_if,omitempty"`
	DNS       []string `json:"dns,omitempty"`
	DNSVia    string   `json:"dns_via,omitempty"`

	Health *Health `json:"health,omitempty"`
}

type Health struct {
	At          int64    `json:"at"`
	FailedCount int      `json:"failed_count"`
	Failed      []string `json:"failed"`
	AutoUpdates string   `json:"auto_updates"`
	LastUpgrade *Upgrade `json:"last_upgrade,omitempty"`
}

type Upgrade struct {
	Time     int64  `json:"time"`
	Packages int    `json:"packages"`
	By       string `json:"by"`
}

type ActionSpec struct {
	Name     string `json:"name"`
	Title    string `json:"title"`
	Timeout  int    `json:"timeout"`
	Mutating bool   `json:"mutating"`
}

type FeatureInfo struct {
	Name    string       `json:"name"`
	Title   string       `json:"title"`
	Actions []ActionSpec `json:"actions"`
}

func (f *FeatureInfo) Action(name string) *ActionSpec {
	if f == nil {
		return nil
	}
	for i := range f.Actions {
		if f.Actions[i].Name == name {
			return &f.Actions[i]
		}
	}
	return nil
}

type EnrollRequest struct {
	Token       string `json:"token"`
	CoreVersion string `json:"core_version"`
	Facts       Facts  `json:"facts"`
	AgentKey    string `json:"agent_key,omitempty"`
}

type EnrollResponse struct {
	AgentID    string `json:"agent_id"`
	Secret     string `json:"secret"`
	SigningKey string `json:"signing_key"`
}

type PollRequest struct {
	Protocol    int           `json:"protocol"`
	CoreVersion string        `json:"core_version"`
	Facts       Facts         `json:"facts"`
	Features    []FeatureInfo `json:"features"`
	Running     []string      `json:"running"`
	ReportOnly  bool          `json:"report_only"`
	Pins        [2]string     `json:"pins"`
}

type PollResponse struct {
	Commands  []Command `json:"commands"`
	Heartbeat int       `json:"heartbeat,omitempty"`
}

const (
	CmdAction     = "action"
	CmdCoreUpdate = "core.update"
	CmdUninstall  = "agent.uninstall"

	CmdSetMode = "agent.mode"
)

const CmdSetPins = "agent.pins"

type PinParams struct {
	Pins [2]string `json:"pins"`
}

const CmdPing = "agent.ping"

type PingReply struct {
	TimeMs  int64     `json:"time_ms"`
	Version string    `json:"version"`
	KeyUsed string    `json:"key_used"`
	Keys    [2]string `json:"keys"`
}

type ModeParams struct {
	ReportOnly bool `json:"report_only"`
}

type Command struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Feature  string          `json:"feature,omitempty"`
	Action   string          `json:"action,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
	Artifact *Artifact       `json:"artifact,omitempty"`
	Timeout  int             `json:"timeout,omitempty"`
}

type Artifact struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	SHA256    string `json:"sha256"`
	Signature string `json:"signature"`
	URL       string `json:"url"`
}

type OutputBatch struct {
	Items []OutputItem `json:"items"`
}

type OutputItem struct {
	CommandID string `json:"command_id"`
	Data      string `json:"data"`
}

type Result struct {
	CommandID string          `json:"command_id"`
	OK        bool            `json:"ok"`
	Error     string          `json:"error,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}
