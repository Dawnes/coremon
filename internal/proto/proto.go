// Package proto defines the wire protocol between the agent and the master.
//
// Transport: HTTPS (TLS pinned by SPKI hash on the agent side). The agent
// always initiates: it long-polls the master for commands and posts output
// and results back. No inbound ports are needed on monitored systems.
package proto

import "encoding/json"

// ProtocolVersion is bumped when the protocol changes.
// 2: built-in features instead of modules; heartbeat in poll responses.
// 3: report-only mode (agent.mode command, report_only in polls).
// 4: two master key slots (agent.pins command, pins in polls).
const ProtocolVersion = 4

// Facts describe the system an agent runs on. Sent on every poll; the
// master keeps only the latest value.
type Facts struct {
	Hostname  string   `json:"hostname"`
	OSName    string   `json:"os_name"`    // e.g. "Debian GNU/Linux 13 (trixie)"
	OSID      string   `json:"os_id"`      // e.g. "debian"
	OSVersion string   `json:"os_version"` // e.g. "13"
	Kernel    string   `json:"kernel"`
	OS        string   `json:"os"`   // Go GOOS, e.g. "linux"
	Arch      string   `json:"arch"` // Go GOARCH, e.g. "amd64"
	Virt      string   `json:"virt"` // lxc, kvm, none, ...
	UptimeSec int64    `json:"uptime_sec"`
	CPUs      int      `json:"cpus"`
	MemTotal  uint64   `json:"mem_total"` // memory assigned to the system, bytes
	IPs       []string `json:"ips"`
}

// ActionSpec describes one action of a built-in feature.
type ActionSpec struct {
	Name     string `json:"name"`
	Title    string `json:"title"`
	Timeout  int    `json:"timeout"`  // seconds
	Mutating bool   `json:"mutating"` // changes the system
}

// FeatureInfo is reported by the agent for every built-in feature.
type FeatureInfo struct {
	Name    string       `json:"name"`
	Title   string       `json:"title"`
	Actions []ActionSpec `json:"actions"`
}

// Action returns the spec for the named action, or nil.
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

// EnrollRequest is sent once, with the one-time enrollment token.
type EnrollRequest struct {
	Token       string `json:"token"`
	CoreVersion string `json:"core_version"`
	Facts       Facts  `json:"facts"`
}

// EnrollResponse gives the agent its permanent credentials.
type EnrollResponse struct {
	AgentID    string `json:"agent_id"`
	Secret     string `json:"secret"`
	SigningKey string `json:"signing_key"` // base64 ed25519 public key, pinned by the agent
}

// PollRequest is the agent heartbeat; the master answers with commands.
type PollRequest struct {
	Protocol    int           `json:"protocol"`
	CoreVersion string        `json:"core_version"`
	Facts       Facts         `json:"facts"`
	Features    []FeatureInfo `json:"features"`
	Running     []string      `json:"running"`     // command IDs still executing
	ReportOnly  bool          `json:"report_only"` // the agent's own, stored mode (protocol 3)
	Pins        [2]string     `json:"pins"`        // master keys the agent trusts, per slot (protocol 4)
}

// PollResponse carries zero or more commands. Heartbeat is the longest the
// master holds a poll open, in seconds; the agent sizes its timeout by it.
type PollResponse struct {
	Commands  []Command `json:"commands"`
	Heartbeat int       `json:"heartbeat,omitempty"`
}

// Command types. "core.update" and "agent.uninstall" are unchanged since
// protocol 1, so a new master can still update an old agent.
const (
	CmdAction     = "action"      // run Feature/Action with Params
	CmdCoreUpdate = "core.update" // replace the agent with Artifact (up- or downgrade)
	CmdUninstall  = "agent.uninstall"
	// CmdSetMode switches report-only mode on or off; Params: ModeParams.
	// In report-only mode the agent refuses every "action" command.
	CmdSetMode = "agent.mode"
)

// CmdSetPins replaces the agent's two master key slots; Params: PinParams.
// The agent only accepts a pair that still contains the key of the
// connection the command arrived on, so a new key can only be added over a
// connection verified with a key the agent already trusts.
const CmdSetPins = "agent.pins"

// PinParams are the parameters of CmdSetPins.
type PinParams struct {
	Pins [2]string `json:"pins"`
}

// ModeParams are the parameters of CmdSetMode.
type ModeParams struct {
	ReportOnly bool `json:"report_only"`
}

// Command is an instruction from master to agent.
type Command struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Feature  string          `json:"feature,omitempty"`
	Action   string          `json:"action,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
	Artifact *Artifact       `json:"artifact,omitempty"`
	Timeout  int             `json:"timeout,omitempty"` // seconds
}

// Artifact is a signed agent binary.
type Artifact struct {
	Kind      string `json:"kind"` // "core"
	Name      string `json:"name"`
	Version   string `json:"version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	SHA256    string `json:"sha256"`    // hex
	Signature string `json:"signature"` // base64 ed25519
	URL       string `json:"url"`       // path on master, e.g. /api/agent/download/...
}

// OutputBatch carries streamed command output.
type OutputBatch struct {
	Items []OutputItem `json:"items"`
}

// OutputItem is a chunk of output for one command.
type OutputItem struct {
	CommandID string `json:"command_id"`
	Data      string `json:"data"`
}

// Result is the final outcome of a command. By convention, a "state" key in
// Data is stored by the master as that feature's current state.
type Result struct {
	CommandID string          `json:"command_id"`
	OK        bool            `json:"ok"`
	Error     string          `json:"error,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}
