// Package proto defines the wire protocol between agent-core and master.
//
// Transport: HTTPS (TLS pinned by SPKI hash on the agent side). The agent
// always initiates: it long-polls the master for commands and posts output
// and results back. No inbound ports are needed on monitored systems.
package proto

import "encoding/json"

// ProtocolVersion is bumped on incompatible changes.
const ProtocolVersion = 1

// Facts describe the system an agent runs on. Sent on every poll.
type Facts struct {
	Hostname  string   `json:"hostname"`
	OSName    string   `json:"os_name"`    // e.g. "Debian GNU/Linux 12 (bookworm)"
	OSID      string   `json:"os_id"`      // e.g. "debian"
	OSVersion string   `json:"os_version"` // e.g. "12"
	Kernel    string   `json:"kernel"`
	OS        string   `json:"os"`   // Go GOOS, e.g. "linux"
	Arch      string   `json:"arch"` // Go GOARCH, e.g. "amd64"
	Virt      string   `json:"virt"` // lxc, kvm, none, ...
	UptimeSec int64    `json:"uptime_sec"`
	CPUs      int      `json:"cpus"`
	Load1     float64  `json:"load1"`
	MemTotal  uint64   `json:"mem_total"`
	MemAvail  uint64   `json:"mem_avail"`
	DiskTotal uint64   `json:"disk_total"`
	DiskFree  uint64   `json:"disk_free"`
	IPs       []string `json:"ips"`
}

// Manifest describes a module and the actions it offers.
type Manifest struct {
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Actions     []ActionSpec `json:"actions"`
}

// ActionSpec describes one action of a module.
type ActionSpec struct {
	Name     string `json:"name"`
	Title    string `json:"title"`
	Timeout  int    `json:"timeout"`  // seconds
	Mutating bool   `json:"mutating"` // changes the system
}

// Action returns the spec for the named action, or nil.
func (m *Manifest) Action(name string) *ActionSpec {
	if m == nil {
		return nil
	}
	for i := range m.Actions {
		if m.Actions[i].Name == name {
			return &m.Actions[i]
		}
	}
	return nil
}

// ModuleInfo is reported by the agent for every installed module.
type ModuleInfo struct {
	Name     string    `json:"name"`
	Version  string    `json:"version"`
	Manifest *Manifest `json:"manifest,omitempty"`
	Error    string    `json:"error,omitempty"`
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
	Protocol    int          `json:"protocol"`
	CoreVersion string       `json:"core_version"`
	Facts       Facts        `json:"facts"`
	Modules     []ModuleInfo `json:"modules"`
	Running     []string     `json:"running"` // command IDs still executing
}

// PollResponse carries zero or more commands.
type PollResponse struct {
	Commands []Command `json:"commands"`
}

// Command types.
const (
	CmdModuleCall    = "module.call"    // run Module/Action with Params
	CmdModuleInstall = "module.install" // install Artifact
	CmdModuleRemove  = "module.remove"  // remove Module
	CmdCoreUpdate    = "core.update"    // replace agent-core with Artifact
	CmdUninstall     = "agent.uninstall"
)

// Command is an instruction from master to agent.
type Command struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Module   string          `json:"module,omitempty"`
	Action   string          `json:"action,omitempty"`
	Params   json.RawMessage `json:"params,omitempty"`
	Artifact *Artifact       `json:"artifact,omitempty"`
	Timeout  int             `json:"timeout,omitempty"` // seconds
}

// Artifact is a signed binary (agent-core or a module).
type Artifact struct {
	Kind      string `json:"kind"` // "core" or "module"
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

// Result is the final outcome of a command.
type Result struct {
	CommandID string          `json:"command_id"`
	OK        bool            `json:"ok"`
	Error     string          `json:"error,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// Module stdio protocol: the agent runs `<module> manifest` (prints Manifest
// JSON) or `<module> run <action>` with params JSON on stdin. The module
// writes JSON lines (ModuleEvent) to stdout.
type ModuleEvent struct {
	T     string          `json:"t"`           // "log" or "result"
	D     string          `json:"d,omitempty"` // log line
	OK    bool            `json:"ok,omitempty"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}
