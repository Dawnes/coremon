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
	// Probe: set by agents that can check agentless devices for the master
	// (remote probe); older agents leave it out.
	Probe *ProbeReport `json:"probe,omitempty"`
	// Metrics: the agent's own disk, memory and load readings (agent 0.3.12
	// and later); older agents leave it out.
	Metrics *Metrics `json:"metrics,omitempty"`
}

// Metrics: every 60 s the agent reads its filesystems, memory and load;
// the readings not yet delivered go with the next poll.
type Metrics struct {
	Disks   []DiskInfo     `json:"disks"`             // the filesystems now (local ones; no tmpfs, overlay, squashfs)
	Samples []MetricSample `json:"samples,omitempty"` // oldest first
}

type DiskInfo struct {
	Mount string `json:"mount"`
	FS    string `json:"fs"`
	Size  uint64 `json:"size"` // bytes
}

type MetricSample struct {
	AgoMs    int64     `json:"ago_ms"` // how long before sending it was read
	Load1    float64   `json:"load1"`
	Load5    float64   `json:"load5"`
	Load15   float64   `json:"load15"`
	CPUs     int       `json:"cpus"`
	MemTotal uint64    `json:"mem_total"` // bytes
	MemAvail uint64    `json:"mem_avail"` // bytes the system can still give out without swapping
	Disks    []DiskUse `json:"disks,omitempty"`
}

type DiskUse struct {
	Mount string `json:"mount"`
	Size  uint64 `json:"size"`  // bytes
	Free  uint64 `json:"free"`  // bytes free, including those reserved for root
	Avail uint64 `json:"avail"` // bytes free for normal users
}

// Remote probe: the master gives an agent a complete check list (never only
// the changes); the agent runs the checks on its own schedule and reports
// the results with every poll, and right away when a status changes (op
// "probe", body ProbeReport). The list lives in the agent's memory only.
const (
	CmdProbeList  = "probe.list"  // params ProbeList; result data ProbeAck
	CmdProbeClear = "probe.clear" // drop the list and all pending results; result data ProbeAck
)

type ProbeCheck struct {
	ID       string `json:"id"`   // the master's id of the check
	Kind     string `json:"kind"` // "ping", "tcp", "https" (agent 0.3.10 and later), "dns" (0.3.11 and later) or "cert" (0.3.15 and later)
	Address  string `json:"address"`
	Port     int    `json:"port,omitempty"`
	Greeting bool   `json:"greeting,omitempty"`
	Reverse  bool   `json:"reverse,omitempty"` // tcp: reachable counts as down
	// https: hostname for SNI and the Host header (not looked up), path,
	// accepted status codes, certificate checks.
	Host      string `json:"host,omitempty"`
	Path      string `json:"path,omitempty"`
	Codes     string `json:"codes,omitempty"`
	CertValid bool   `json:"cert_valid,omitempty"`
	CertDays  int    `json:"cert_days,omitempty"`
	// cert (agent 0.3.15 and later): Host is the name; certificate valid in
	// time; Remote: connect to where Host points (DNS) instead of Address.
	CertTime bool `json:"cert_time,omitempty"`
	Remote   bool `json:"remote,omitempty"`
	// dns: the name to resolve is Host; record type, expected value (an
	// answer must contain it) and maximum response time in ms.
	RType     string `json:"rtype,omitempty"`
	Expect    string `json:"expect,omitempty"`
	MaxMs     int    `json:"max_ms,omitempty"`
	Interval  int    `json:"interval"`             // seconds
	Timeout   int    `json:"timeout"`              // seconds
	Retries   int    `json:"retries,omitempty"`    // tcp: failed checks 30 s apart before down
	DownAfter int    `json:"down_after,omitempty"` // ping: unanswered pings before down
}

type ProbeList struct {
	Version string       `json:"version"` // "" = no checks
	Checks  []ProbeCheck `json:"checks"`
}

type ProbeResult struct {
	ID     string `json:"id"`
	AgoMs  int64  `json:"ago_ms"` // how long before sending the check finished (no clock comparison needed)
	OK     bool   `json:"ok"`     // ping answered, or TCP connected (and greeted when asked)
	RTTUs  int64  `json:"rtt_us,omitempty"`
	Banner string `json:"banner,omitempty"`
	Error  string `json:"error,omitempty"`
	Setup  bool   `json:"setup,omitempty"`  // the agent could not run the check at all (no permission)
	Failed string `json:"failed,omitempty"` // the sub-probe that failed (connect, greeting, tls, cert, cert_days, http)
	// cert checks: the certificate the server presented (also when the check failed).
	Cert *CertInfo `json:"cert,omitempty"`
}

// CertInfo: what a TLS server presented: the connection and the chain, the
// server's own certificate first.
type CertInfo struct {
	Addr   string      `json:"addr"`   // the address connected to
	TLS    string      `json:"tls"`    // e.g. "TLS 1.3"
	Cipher string      `json:"cipher"` // e.g. "TLS_AES_128_GCM_SHA256"
	Chain  []CertEntry `json:"chain"`
}

type CertEntry struct {
	Subject   string   `json:"subject"`
	Issuer    string   `json:"issuer"`
	Names     []string `json:"names,omitempty"` // every name and IP address it is valid for
	NotBefore int64    `json:"not_before"`
	NotAfter  int64    `json:"not_after"`
	Key       string   `json:"key"`    // e.g. "RSA 2048", "ECDSA P-256"
	SigAlg    string   `json:"sigalg"` // e.g. "SHA256-RSA"
	Serial    string   `json:"serial"` // hex
	SHA256    string   `json:"sha256"` // fingerprint, hex with colons
	CA        bool     `json:"ca,omitempty"`
}

type ProbeReport struct {
	Version string        `json:"version"` // the list the agent runs
	Results []ProbeResult `json:"results,omitempty"`
}

type ProbeAck struct {
	Version string `json:"version"`
	Checks  int    `json:"checks"`
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
