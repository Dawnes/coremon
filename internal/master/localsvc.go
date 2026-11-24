package master

// Services of agentless devices: the master opens a TCP connection to each
// service's port (and, where the service greets first, reads the greeting)
// to see whether it is up. Nothing is sent and nothing is logged in to.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"coremon/internal/probe"
	"coremon/internal/proto"
)

type Service struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Preset   string `json:"preset,omitempty"` // key of a default service; "" = custom
	Method   string `json:"method,omitempty"` // check method: "" (tcp) or "https"
	Port     int    `json:"port"`
	Greeting bool   `json:"greeting,omitempty"` // tcp: the service sends a greeting first: wait for it
	Interval int    `json:"interval"`
	Retries  int    `json:"retries"`
	Timeout  int    `json:"timeout"`
	Reverse  bool   `json:"reverse,omitempty"` // tcp: reachable counts as down
	Paused   bool   `json:"paused,omitempty"`  // not checked until resumed

	// https: hostname (SNI and Host header, not looked up), path, accepted
	// status codes, certificate must be valid, valid for at least N days (0 = off).
	Host      string `json:"host,omitempty"`
	Path      string `json:"path,omitempty"`
	Codes     string `json:"codes,omitempty"`
	CertValid bool   `json:"cert_valid,omitempty"`
	CertDays  int    `json:"cert_days,omitempty"`

	// cert (Certificate Validity): Host and Port are the web address;
	// CertValid is the valid chain (trusted and issued for Host), CertTime
	// valid at this time, CertDays the warning before it expires. It
	// connects to the device's address (for a device known by a name:
	// where the name points).
	CertTime bool `json:"cert_time,omitempty"`

	// dns: the name to resolve is Host; record type, expected value (an
	// answer must contain it, "" = not checked), maximum response time in ms
	// (0 = not checked). The query goes to the DNS server on the device.
	RType  string `json:"rtype,omitempty"`
	Expect string `json:"expect,omitempty"`
	MaxMs  int    `json:"max_ms,omitempty"`

	// Measured by the device's own agent (Method disk, memory or load):
	// the filesystem (disk), the limit (disk: percent used, or GB free with
	// LimitKind "free"; memory: percent used; load: per core) and the
	// readings in a row past it before down.
	Mount     string  `json:"mount,omitempty"`
	Limit     float64 `json:"limit,omitempty"`
	LimitKind string  `json:"limit_kind,omitempty"`
	Readings  int     `json:"readings,omitempty"`

	// Results: kept in checks.dat, not in state.dat.
	Checked   bool            `json:"-"`
	Up        bool            `json:"-"`
	Since     int64           `json:"-"`
	LastCheck int64           `json:"-"`
	LastOK    int64           `json:"-"`
	RTTUs     int64           `json:"-"`
	Fails     int             `json:"-"`
	LastError string          `json:"-"`
	Banner    string          `json:"-"`
	Failed    string          `json:"-"` // the sub-probe of the last failed check
	Hist      []histSlot      `json:"-"`
	Value     float64         `json:"-"` // measured: the last reading (in metricUnit)
	Vals      []valSlot       `json:"-"` // measured: the readings in five-minute slots, for the graph
	Cert      *proto.CertInfo `json:"-"` // cert: what the last check saw, for "Show"
}

// spec: what the check needs.
func (sv *Service) spec() probe.Service {
	return probe.Service{Method: sv.Method, Port: sv.Port, Greeting: sv.Greeting, Host: sv.Host, Path: sv.Path, Codes: sv.Codes,
		CertValid: sv.CertValid, CertDays: sv.CertDays, RType: sv.RType, Expect: sv.Expect, MaxMs: sv.MaxMs, CertTime: sv.CertTime}
}

// kind: the check method's name in a remote probe's list.
func (sv *Service) kind() string {
	if sv.Method == "" {
		return probe.MethodTCP
	}
	return sv.Method
}

const (
	defaultSvcInterval = 300
	minSvcInterval     = 60
	maxSvcInterval     = 86400
	defaultSvcRetries  = 3
	maxSvcRetries      = 10
	defaultSvcTimeout  = 5
	minSvcTimeout      = 1
	maxSvcTimeout      = 60
	svcRetryEvery      = 30 * time.Second
	maxServices        = 50
	maxServiceName     = 48
	maxBanner          = 160
)

type svcPreset struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Port      int    `json:"port"`
	Greeting  bool   `json:"greeting"`
	Group     string `json:"group"`
	Method    string `json:"method,omitempty"`     // the check method it starts with ("" = tcp)
	CertValid bool   `json:"cert_valid,omitempty"` // https: the certificate must be valid
	// Methods: the check methods that suit it, the default first (set by init).
	Methods []string `json:"methods"`
	// Agent: measured by the device's own agent; only for devices with an agent.
	Agent bool `json:"agent,omitempty"`
}

// svcAlso: further check methods that suit a default service, after its own.
var svcAlso = map[string][]string{
	"https":   {probe.MethodTCP},
	"proxmox": {probe.MethodTCP},
	"dns":     {probe.MethodTCP},
}

func init() {
	for i := range svcPresets {
		p := &svcPresets[i]
		own := p.Method
		if own == "" {
			own = probe.MethodTCP
		}
		p.Methods = append([]string{own}, svcAlso[p.Key]...)
	}
}

// allows: the check method suits this default service.
func (p *svcPreset) allows(method string) bool {
	if method == "" {
		method = probe.MethodTCP
	}
	for _, m := range p.Methods {
		if m == method {
			return true
		}
	}
	return false
}

// The default services: their port and whether they greet first are known.
var svcPresets = []svcPreset{
	{Key: metricDisk, Name: "Disk space", Group: "Measured by the agent", Method: metricDisk, Agent: true},
	{Key: metricMem, Name: "Memory", Group: "Measured by the agent", Method: metricMem, Agent: true},
	{Key: metricLoad, Name: "CPU load", Group: "Measured by the agent", Method: metricLoad, Agent: true},
	// The port is part of the web address the admin enters (443 when none).
	{Key: "cert", Name: "Certificate Validity", Port: 443, Group: "Certificates", Method: probe.MethodCert},
	{Key: "ssh", Name: "SSH", Port: 22, Greeting: true, Group: "Remote access"},
	{Key: "telnet", Name: "Telnet", Port: 23, Group: "Remote access"},
	{Key: "rdp", Name: "RDP", Port: 3389, Group: "Remote access"},
	{Key: "vnc", Name: "VNC", Port: 5900, Greeting: true, Group: "Remote access"},
	{Key: "http", Name: "HTTP", Port: 80, Group: "Web"},
	{Key: "https", Name: "HTTPS", Port: 443, Group: "Web", Method: probe.MethodHTTPS, CertValid: true},
	// Proxmox uses a self-signed certificate unless one was installed.
	{Key: "proxmox", Name: "Proxmox", Port: 8006, Group: "Web", Method: probe.MethodHTTPS},
	{Key: "smtp", Name: "SMTP", Port: 25, Greeting: true, Group: "Mail"},
	{Key: "smtps", Name: "SMTPS", Port: 465, Group: "Mail"},
	{Key: "submission", Name: "Submission", Port: 587, Greeting: true, Group: "Mail"},
	{Key: "pop3", Name: "POP3", Port: 110, Greeting: true, Group: "Mail"},
	{Key: "pop3s", Name: "POP3S", Port: 995, Group: "Mail"},
	{Key: "imap", Name: "IMAP", Port: 143, Greeting: true, Group: "Mail"},
	{Key: "imaps", Name: "IMAPS", Port: 993, Group: "Mail"},
	{Key: "ftp", Name: "FTP", Port: 21, Greeting: true, Group: "Files and directories"},
	{Key: "smb", Name: "SMB", Port: 445, Group: "Files and directories"},
	{Key: "nfs", Name: "NFS", Port: 2049, Group: "Files and directories"},
	{Key: "ldap", Name: "LDAP", Port: 389, Group: "Files and directories"},
	{Key: "ldaps", Name: "LDAPS", Port: 636, Group: "Files and directories"},
	{Key: "mysql", Name: "MySQL/MariaDB", Port: 3306, Greeting: true, Group: "Databases"},
	{Key: "postgresql", Name: "PostgreSQL", Port: 5432, Group: "Databases"},
	{Key: "redis", Name: "Redis", Port: 6379, Group: "Databases"},
	{Key: "mongodb", Name: "MongoDB", Port: 27017, Group: "Databases"},
	{Key: "dns", Name: "DNS", Port: 53, Group: "Other", Method: probe.MethodDNS},
	{Key: "mqtt", Name: "MQTT", Port: 1883, Group: "Other"},
	{Key: "ipp", Name: "Printer IPP", Port: 631, Group: "Other"},
	{Key: "jetdirect", Name: "Printer raw", Port: 9100, Group: "Other"},
}

// svcField: one setting of a check method, as the service form shows it.
// Types: bool (tickbox), text, select (one of Options), limit (tickbox
// with a number in Unit; 0 = off), switch (one of Choices: value, label).
type svcField struct {
	Key         string      `json:"key"`
	Label       string      `json:"label"`
	Type        string      `json:"type"`
	Hint        string      `json:"hint,omitempty"`
	Placeholder string      `json:"placeholder,omitempty"`
	Default     any         `json:"default,omitempty"`
	Max         int         `json:"max,omitempty"`
	Unit        string      `json:"unit,omitempty"`
	Options     []string    `json:"options,omitempty"`
	Choices     [][2]string `json:"choices,omitempty"`
}

// svcMethod: a check method and its settings. Its sub-probes come from
// probe.Steps (per service, in the service view).
type svcMethod struct {
	Key    string     `json:"key"`
	Name   string     `json:"name"`
	Fields []svcField `json:"fields"`
}

// svcMethods: the check methods. The service form is built from this
// table; probe.Validate checks the values.
var svcMethods = []svcMethod{
	{Key: probe.MethodTCP, Name: "TCP connect",
		Fields: []svcField{
			{Key: "greeting", Label: "Wait for a greeting", Type: "bool",
				Hint: "Only for services that send text first (SSH, SMTP, FTP…): the check then also needs that first line. Otherwise a connection is enough."},
			{Key: "reverse", Label: "Reverse mode", Type: "bool",
				Hint: "The status is turned upside down: reachable counts as down. For example for a port that must stay closed."},
		}},
	{Key: probe.MethodHTTPS, Name: "HTTPS",
		Fields: []svcField{
			{Key: "host", Label: "Hostname", Type: "text", Placeholder: "www.example.com",
				Hint: "Sent as the TLS server name and the Host header. It is not looked up: the check always goes to this device's address."},
			{Key: "path", Label: "Path", Type: "text", Default: "/", Placeholder: "/"},
			{Key: "codes", Label: "Accepted status codes", Type: "text", Default: probe.DefaultCodes, Placeholder: probe.DefaultCodes,
				Hint: "For example 200-399 or 200,204,301-302. Redirects are not followed."},
			{Key: "cert_valid", Label: "Certificate must be valid", Type: "bool", Default: true,
				Hint: "Trusted by the standard roots of the system that checks, valid for the hostname and not expired. Untick for a self-signed certificate."},
			{Key: "cert_days", Label: "Certificate valid for at least", Type: "limit", Unit: "days", Default: probe.DefaultCertDays, Max: probe.MaxCertDays,
				Hint: "Fails when the certificate expires sooner, also when it is not trusted."},
		}},
	{Key: probe.MethodDNS, Name: "DNS query",
		Fields: []svcField{
			{Key: "host", Label: "Name to resolve", Type: "text", Placeholder: "example.com",
				Hint: "Asked of the DNS server on this device itself (UDP, and TCP when the answer is too long). For example example.com or _dmarc.example.com."},
			{Key: "rtype", Label: "Record type", Type: "select", Default: probe.DefaultDNSType, Options: probe.DNSTypes},
			{Key: "expect", Label: "Expected value (optional)", Type: "text", Placeholder: "e.g. 192.0.2.10 or mail.example.com",
				Hint: "Fails when no answer contains this text (case does not matter). Empty: any answer is fine."},
			{Key: "max_ms", Label: "Answer within", Type: "limit", Unit: "ms", Default: probe.DefaultMaxMs, Max: probe.MaxMaxMs,
				Hint: "Fails when the answer takes longer. The request timeout below still ends a check that gets no answer."},
		}},
	{Key: probe.MethodCert, Name: "Certificate",
		Fields: []svcField{
			{Key: "host", Label: "Web address", Type: "text", Placeholder: "www.example.com or www.example.com:8443",
				Hint: "A name, optionally with a port (443 when none). A full https:// address works too; a path is ignored."},
			{Key: "cert_valid", Label: "Valid chain", Type: "bool", Default: true,
				Hint: "Trusted by the standard roots of the system that checks, and issued for this name. A self-signed certificate fails."},
			{Key: "cert_time", Label: "Valid in time", Type: "bool", Default: true,
				Hint: "Not expired, and already valid (also the intermediate certificates the server sends)."},
			{Key: "cert_days", Label: "Down when it expires within", Type: "limit", Unit: "days", Default: probe.CertCheckDays, Max: probe.MaxCertDays,
				Hint: "A warning in time, before it expires."},
		}},
}

func presetByKey(k string) *svcPreset {
	for i := range svcPresets {
		if svcPresets[i].Key == k {
			return &svcPresets[i]
		}
	}
	return nil
}

// svcCheckFunc runs one service check against addr.
type svcCheckFunc func(ctx context.Context, addr string, spec probe.Service, timeout time.Duration) probe.Outcome

func serviceCheck(ctx context.Context, addr string, spec probe.Service, timeout time.Duration) probe.Outcome {
	o := probe.Check(ctx, addr, spec, timeout)
	o.Detail = cleanText(o.Detail, maxBanner)
	return o
}

// ---- view ----

type serviceView struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Preset    string          `json:"preset,omitempty"`
	Method    string          `json:"method"`
	Port      int             `json:"port"`
	Greeting  bool            `json:"greeting"`
	Host      string          `json:"host,omitempty"`
	Path      string          `json:"path,omitempty"`
	Codes     string          `json:"codes,omitempty"`
	CertValid bool            `json:"cert_valid"`
	CertDays  int             `json:"cert_days"`
	RType     string          `json:"rtype,omitempty"`
	Expect    string          `json:"expect,omitempty"`
	MaxMs     int             `json:"max_ms"`
	CertTime  bool            `json:"cert_time"`
	Cert      *proto.CertInfo `json:"cert,omitempty"`   // cert: what the last check saw (device page only)
	Steps     []string        `json:"steps"`            // the sub-probes, in order
	Failed    string          `json:"failed,omitempty"` // the sub-probe of the last failed check
	Interval  int             `json:"interval"`
	Retries   int             `json:"retries"`
	Timeout   int             `json:"timeout"`
	Reverse   bool            `json:"reverse"`
	Paused    bool            `json:"paused"`
	Checked   bool            `json:"checked"`
	Up        bool            `json:"up"`
	Since     int64           `json:"since"`
	LastCheck int64           `json:"last_check"`
	LastOK    int64           `json:"last_ok"`
	RTTUs     int64           `json:"rtt_us"`
	Fails     int             `json:"fails"`
	LastError string          `json:"last_error,omitempty"`
	Banner    string          `json:"banner,omitempty"`
	Uptime    float64         `json:"uptime"`
	Hist      []histSlot      `json:"hist,omitempty"`

	// Measured by the agent: the settings, the last reading and the graph.
	Metric    bool       `json:"metric,omitempty"`
	Mount     string     `json:"mount,omitempty"`
	Limit     float64    `json:"limit,omitempty"`
	LimitKind string     `json:"limit_kind,omitempty"`
	Readings  int        `json:"readings,omitempty"`
	Unit      string     `json:"unit,omitempty"`
	Value     float64    `json:"value,omitempty"`
	Stale     bool       `json:"stale,omitempty"` // no readings now: the agent is offline (unknown, not down)
	Vals      []valPoint `json:"vals,omitempty"`
}

func (s *Server) serviceView(sv *Service, now int64, full bool) serviceView {
	h, up := recentHist(sv.Hist, now)
	v := serviceView{ID: sv.ID, Name: sv.Name, Preset: sv.Preset, Method: sv.kind(), Port: sv.Port, Greeting: sv.Greeting,
		Host: sv.Host, Path: sv.Path, Codes: sv.Codes, CertValid: sv.CertValid, CertDays: sv.CertDays, RType: sv.RType, Expect: sv.Expect, MaxMs: sv.MaxMs, Steps: []string{}, Failed: sv.Failed,
		Interval: sv.Interval, Retries: sv.Retries, Timeout: sv.Timeout, Reverse: sv.Reverse, Paused: sv.Paused, Checked: sv.Checked, Up: sv.Up, Since: sv.Since,
		LastCheck: sv.LastCheck, LastOK: sv.LastOK, RTTUs: sv.RTTUs, Fails: sv.Fails, LastError: sv.LastError, Banner: sv.Banner, Uptime: up}
	if sv.isMetric() {
		v.Metric, v.Mount, v.Limit, v.LimitKind, v.Readings, v.Unit, v.Value = true, sv.Mount, sv.Limit, sv.LimitKind, sv.Readings, sv.metricUnit(), sv.Value
		if full {
			v.Vals = recentVals(sv.Vals, now)
		}
	} else {
		v.Steps = probe.Steps(sv.spec())
	}
	v.CertTime = sv.CertTime
	if full {
		v.Hist = h
		v.Cert = sv.Cert
	}
	return v
}

// ---- results and scheduling ----

// recordService stores one check. Retries follow 30 s apart; after the
// last one fails the service is down, and it is then checked every 30 s
// until it is up again. Caller holds s.st.mu and s.loc.mu.
func (s *Server) recordService(sv *Service, o probe.Outcome, now time.Time) {
	if sv.Method == probe.MethodCert && o.Cert != nil {
		sv.Cert = cleanCert(o.Cert)
	}
	if s.storeService(sv, o.RTT, o.Detail, o.Err == nil, o.Err, o.Failed, now) {
		s.loc.next[sv.ID] = now.Add(time.Duration(sv.Interval) * time.Second)
	} else {
		s.loc.next[sv.ID] = now.Add(svcRetryEvery)
	}
}

// storeService stores one result without planning the next check (a
// remote probe plans its own); true when the service counted as up.
// failed: the sub-probe that failed. Caller holds s.st.mu.
func (s *Server) storeService(sv *Service, rtt time.Duration, banner string, reached bool, cerr error, failed string, now time.Time) bool {
	t := now.Unix()
	ok := reached != sv.Reverse
	sv.LastCheck = t
	if reached || !probe.KnownStep(failed) {
		failed = ""
	}
	if ok {
		sv.LastOK, sv.Fails, sv.LastError, sv.Failed = t, 0, "", ""
		if reached {
			sv.RTTUs = rtt.Microseconds()
			if banner != "" {
				sv.Banner = banner
			}
		} else {
			sv.RTTUs = 0
		}
		if !sv.Checked || !sv.Up {
			sv.Checked, sv.Up, sv.Since = true, true, t
		}
	} else {
		sv.Fails++
		if banner != "" {
			sv.Banner = banner // what was answered, e.g. the records without the expected value
		}
		if reached {
			sv.RTTUs, sv.LastError = rtt.Microseconds(), "reachable, but reverse mode expects it not to be"
		} else {
			sv.LastError = clip(cerr.Error(), 200)
		}
		sv.Failed = failed
		if sv.Fails > sv.Retries && (!sv.Checked || sv.Up) {
			sv.Checked, sv.Up, sv.Since = true, false, t
		}
	}
	sv.Hist = addHist(sv.Hist, ok, failed, t)
	s.markChecks()
	return ok
}

type svcKey struct{ dev, svc string }

// dueServices: services whose next check is due, marked busy. Caller holds
// s.st.mu and s.loc.mu.
func (s *Server) dueServices(devs []*checkDev, now time.Time) []svcKey {
	var due []svcKey
	known := map[string]bool{}
	i := 0
	for _, d := range devs {
		if d.Probe != "" || d.Maint != nil {
			continue // checked by a remote probe on its own schedule, or in maintenance
		}
		for _, sv := range d.outside() {
			if sv.Paused {
				continue
			}
			known[sv.ID] = true
			next, seen := s.loc.next[sv.ID]
			if !seen {
				next = now.Add(time.Duration(i%20) * time.Second)
				s.loc.next[sv.ID] = next
				i++
			}
			if !s.loc.busy[sv.ID] && !now.Before(next) {
				s.loc.busy[sv.ID] = true
				// Safety net; recordService sets the real next time.
				s.loc.next[sv.ID] = now.Add(time.Duration(sv.Interval) * time.Second)
				due = append(due, svcKey{d.ID, sv.ID})
			}
		}
	}
	for id := range s.loc.next {
		if strings.HasPrefix(id, "s_") && !known[id] {
			delete(s.loc.next, id)
		}
	}
	return due
}

// checkService checks one service and records the result. Without locks held.
func (s *Server) checkService(devID, svcID string) {
	s.st.mu.Lock()
	d := s.checkDevByID(devID)
	var sv *Service
	if d != nil {
		sv = d.service(svcID)
	}
	if sv == nil {
		s.st.mu.Unlock()
		return
	}
	addr, spec, timeout := d.Address, sv.spec(), time.Duration(sv.Timeout)*time.Second
	s.st.mu.Unlock()
	// HTTPS may need a second connection (GET after a failed HEAD).
	ctx, cancel := context.WithTimeout(context.Background(), 4*timeout+time.Second)
	o := s.svcCheck(ctx, addr, spec, timeout)
	cancel()
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	d = s.checkDevByID(devID)
	if d == nil || d.Address != addr {
		return
	}
	if sv = d.service(svcID); sv == nil || sv.spec() != spec {
		return
	}
	s.loc.mu.Lock()
	now := time.Now()
	s.recordService(sv, o, now)
	s.loc.mu.Unlock()
	d.noteIP(o.IP)
	s.noteBehind(d.ID, o.Err != nil && !sv.Reverse, sv.Hist, now)
}

// ---- handlers ----

type serviceReq struct {
	Name     *string `json:"name"`
	Preset   *string `json:"preset"`
	Port     *int    `json:"port"`
	Greeting *bool   `json:"greeting"`
	Interval *int    `json:"interval"`
	Retries  *int    `json:"retries"`
	Timeout  *int    `json:"timeout"`
	Reverse  *bool   `json:"reverse"`

	Method    *string `json:"method"`
	Host      *string `json:"host"`
	Path      *string `json:"path"`
	Codes     *string `json:"codes"`
	CertValid *bool   `json:"cert_valid"`
	CertDays  *int    `json:"cert_days"`
	RType     *string `json:"rtype"`
	Expect    *string `json:"expect"`
	MaxMs     *int    `json:"max_ms"`
	CertTime  *bool   `json:"cert_time"`

	metricReq
}

// setMethod switches sv to a check method; a method it did not have yet
// starts with its default settings (and drops those of the old one).
func (sv *Service) setMethod(m string, certValid bool) {
	if m == probe.MethodTCP {
		m = ""
	}
	if m == sv.Method {
		return
	}
	if sv.isMetric() {
		// From a measured service to a check from outside: its own defaults.
		sv.Mount, sv.Limit, sv.LimitKind, sv.Readings = "", 0, "", 0
		sv.Interval, sv.Retries, sv.Timeout = defaultSvcInterval, defaultSvcRetries, defaultSvcTimeout
	}
	if sv.Method == probe.MethodCert && sv.Interval == certInterval {
		sv.Interval = defaultSvcInterval // leaving a certificate check: the usual heartbeat
	}
	sv.Method = m
	sv.Host, sv.Path, sv.Codes, sv.CertValid, sv.CertDays, sv.RType, sv.Expect, sv.MaxMs = "", "", "", false, 0, "", "", 0
	sv.CertTime, sv.Cert = false, nil
	switch m {
	case probe.MethodCert:
		sv.Greeting, sv.Reverse = false, false
		sv.CertValid, sv.CertTime, sv.CertDays, sv.Interval = true, true, probe.CertCheckDays, certInterval
	case probe.MethodHTTPS:
		sv.Greeting, sv.Reverse = false, false
		sv.Path, sv.Codes, sv.CertValid, sv.CertDays = "/", probe.DefaultCodes, certValid, probe.DefaultCertDays
	case probe.MethodDNS:
		sv.Greeting, sv.Reverse = false, false
		sv.RType, sv.MaxMs = probe.DefaultDNSType, probe.DefaultMaxMs
	}
}

// apply checks and applies a request to sv; a default service keeps its
// port and greeting.
func (req *serviceReq) apply(sv *Service) error {
	if req.Preset != nil {
		if *req.Preset == "" {
			sv.Preset = ""
			if sv.isMetric() {
				sv.setMethod("", false) // a custom port is checked from outside
			}
		} else if p := presetByKey(*req.Preset); p != nil {
			changed := p.Key != sv.Preset
			if changed {
				if p.Agent {
					sv.setMetric(p.Method)
				} else {
					sv.setMethod(p.Method, p.CertValid) // a new default service starts with its own method
				}
			}
			// A certificate check keeps the port of its web address.
			if changed || p.Method != probe.MethodCert {
				sv.Port = p.Port
			}
			sv.Preset, sv.Greeting = p.Key, p.Greeting
			if sv.Name == "" && req.Name == nil {
				sv.Name = p.Name
			}
		} else {
			return errors.New("unknown default service")
		}
	}
	if sv.isMetric() {
		// Measured by the agent: only the name and the limits apply.
		if req.Name != nil {
			n, err := checkName("service name", *req.Name, maxServiceName)
			if err != nil {
				return err
			}
			sv.Name = n
		}
		if sv.Name == "" {
			return errors.New("a service name is required")
		}
		return req.applyMetric(sv)
	}
	if req.Method != nil {
		if m := *req.Method; m != "" && m != probe.MethodTCP && m != probe.MethodHTTPS && m != probe.MethodDNS && m != probe.MethodCert {
			return errors.New("unknown check method")
		}
		sv.setMethod(*req.Method, true)
	}
	if p := presetByKey(sv.Preset); p != nil && !p.allows(sv.Method) {
		return fmt.Errorf("%s cannot be checked with %s", p.Name, strings.ToUpper(sv.kind()))
	}
	if req.Name != nil {
		n, err := checkName("service name", *req.Name, maxServiceName)
		if err != nil {
			return err
		}
		sv.Name = n
	}
	if sv.Name == "" {
		return errors.New("a service name is required")
	}
	if sv.Preset == "" {
		if req.Port != nil {
			sv.Port = *req.Port
		}
		if req.Greeting != nil {
			sv.Greeting = *req.Greeting
		}
	}
	if sv.Method == probe.MethodHTTPS {
		sv.Greeting = false // an HTTPS check does not wait for a greeting
		if req.Host != nil {
			sv.Host = strings.ToLower(strings.TrimSpace(*req.Host))
		}
		if req.Path != nil {
			sv.Path = strings.TrimSpace(*req.Path)
			if sv.Path == "" {
				sv.Path = "/"
			}
		}
		if req.Codes != nil {
			sv.Codes = probe.CleanCodes(strings.TrimSpace(*req.Codes))
		}
		if req.CertValid != nil {
			sv.CertValid = *req.CertValid
		}
		if req.CertDays != nil {
			sv.CertDays = *req.CertDays
		}
	}
	if sv.Method == probe.MethodCert {
		sv.Greeting = false
		if req.Host != nil {
			h, port, err := probe.ParseTarget(*req.Host)
			if err != nil {
				return err
			}
			sv.Host, sv.Port = h, port
		}
		if req.CertValid != nil {
			sv.CertValid = *req.CertValid
		}
		if req.CertTime != nil {
			sv.CertTime = *req.CertTime
		}
		if req.CertDays != nil {
			sv.CertDays = *req.CertDays
		}
	}
	if sv.Method == probe.MethodDNS {
		sv.Greeting = false // a DNS check does not wait for a greeting
		if req.Host != nil {
			sv.Host = strings.ToLower(strings.TrimSpace(*req.Host))
		}
		if req.RType != nil {
			sv.RType = strings.ToUpper(strings.TrimSpace(*req.RType))
		}
		if req.Expect != nil {
			sv.Expect = strings.TrimSpace(*req.Expect)
		}
		if req.MaxMs != nil {
			sv.MaxMs = *req.MaxMs
		}
	}
	if err := probe.Validate(sv.spec()); err != nil {
		return err
	}
	if req.Interval != nil {
		sv.Interval = *req.Interval
	}
	if req.Retries != nil {
		sv.Retries = *req.Retries
	}
	if req.Timeout != nil {
		sv.Timeout = *req.Timeout
	}
	if req.Reverse != nil {
		sv.Reverse = *req.Reverse && sv.Method == "" // reverse mode is for TCP only
	}
	switch {
	case sv.Interval < minSvcInterval || sv.Interval > maxSvcInterval:
		return fmt.Errorf("the heartbeat interval must be %d-%d seconds", minSvcInterval, maxSvcInterval)
	case sv.Retries < 0 || sv.Retries > maxSvcRetries:
		return fmt.Errorf("retries must be 0-%d", maxSvcRetries)
	case sv.Timeout < minSvcTimeout || sv.Timeout > maxSvcTimeout:
		return fmt.Errorf("the request timeout must be %d-%d seconds", minSvcTimeout, maxSvcTimeout)
	}
	return nil
}

func (sv *Service) describe() string {
	if sv.isMetric() {
		return sv.describeMetric()
	}
	d := fmt.Sprintf("%s port %d, every %ds, %d retries, timeout %ds", sv.Name, sv.Port, sv.Interval, sv.Retries, sv.Timeout)
	if sv.Method == probe.MethodHTTPS {
		d += fmt.Sprintf(", HTTPS %s%s status %s", sv.Host, sv.Path, sv.Codes)
		if sv.CertValid {
			d += ", certificate must be valid"
		}
		if sv.CertDays > 0 {
			d += fmt.Sprintf(", certificate valid ≥ %d days", sv.CertDays)
		}
	}
	if sv.Method == probe.MethodCert {
		d = fmt.Sprintf("%s certificate %s:%d, every %ds, %d retries, timeout %ds", sv.Name, sv.Host, sv.Port, sv.Interval, sv.Retries, sv.Timeout)
		if sv.CertValid {
			d += ", valid chain"
		}
		if sv.CertTime {
			d += ", valid in time"
		}
		if sv.CertDays > 0 {
			d += fmt.Sprintf(", down %d days before it expires", sv.CertDays)
		}
	}
	if sv.Method == probe.MethodDNS {
		d += fmt.Sprintf(", DNS %s %s", sv.RType, sv.Host)
		if sv.Expect != "" {
			d += fmt.Sprintf(", expects %q", sv.Expect)
		}
		if sv.MaxMs > 0 {
			d += fmt.Sprintf(", answer within %d ms", sv.MaxMs)
		}
	}
	if sv.Greeting {
		d += ", greeting required"
	}
	if sv.Reverse {
		d += ", reverse"
	}
	return d
}

// resetService: a changed check starts over.
func (s *Server) resetService(sv *Service) {
	sv.Checked, sv.Up, sv.Since, sv.LastCheck, sv.LastOK, sv.RTTUs, sv.Fails, sv.LastError, sv.Banner, sv.Failed, sv.Hist = false, false, 0, 0, 0, 0, 0, "", "", "", nil
	sv.Value, sv.Vals, sv.Cert = 0, nil, nil
	s.loc.mu.Lock()
	delete(s.loc.next, sv.ID)
	s.loc.mu.Unlock()
}

func (s *Server) handleServiceCreate(w http.ResponseWriter, r *http.Request) {
	var req serviceReq
	if !readJSON(w, r, &req, 4096) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	d := s.checkDevOr404(w, r, true)
	if d == nil {
		return
	}
	if len(d.Services) >= maxServices {
		http.Error(w, fmt.Sprintf("at most %d services per device", maxServices), http.StatusBadRequest)
		return
	}
	if err := s.probeRoom(d.Checks); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	sv := &Service{ID: "s_" + randHex(8), Interval: defaultSvcInterval, Retries: defaultSvcRetries, Timeout: defaultSvcTimeout}
	if err := req.apply(sv); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if sv.isMetric() {
		// Measured by the device's own agent: no address needed.
		if d.Agent == nil {
			http.Error(w, "disk, memory and load are measured by an agent: only for devices with an agent", http.StatusBadRequest)
			return
		}
		if msg := d.metricTaken(sv); msg != "" {
			http.Error(w, msg, http.StatusConflict)
			return
		}
		if d.Agent.Checks == nil {
			d.Agent.Checks = d.Checks
		}
	} else if code, err := s.attach(d); err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	d.Services = append(d.Services, sv)
	s.st.markDirty()
	s.writeDevAudit(r, "device.service_added", d.ID, d.Name, sv.describe())
	writeJSON(w, s.devView(d, true))
}

func (s *Server) handleServiceUpdate(w http.ResponseWriter, r *http.Request) {
	var req serviceReq
	if !readJSON(w, r, &req, 4096) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	d := s.checkDevOr404(w, r, false)
	if d == nil {
		return
	}
	sv := d.service(r.PathValue("sid"))
	if sv == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	n := *sv
	if err := req.apply(&n); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if n.isMetric() {
		if d.Agent == nil {
			http.Error(w, "disk, memory and load are measured by an agent: only for devices with an agent", http.StatusBadRequest)
			return
		}
		if msg := d.metricTaken(&n); msg != "" {
			http.Error(w, msg, http.StatusConflict)
			return
		}
	} else if code, err := s.attach(d); err != nil {
		http.Error(w, err.Error(), code)
		return
	}
	before := sv.describe()
	restart := n.spec() != sv.spec() || n.Reverse != sv.Reverse || n.Mount != sv.Mount || n.LimitKind != sv.LimitKind
	*sv = n
	if restart {
		s.resetService(sv)
	} else {
		s.loc.mu.Lock()
		if next, ok := s.loc.next[sv.ID]; ok && sv.Fails == 0 && next.After(time.Now().Add(time.Duration(sv.Interval)*time.Second)) {
			s.loc.next[sv.ID] = time.Now().Add(time.Duration(sv.Interval) * time.Second)
		}
		s.loc.mu.Unlock()
	}
	s.st.markDirty()
	s.markChecks()
	if after := sv.describe(); after != before {
		s.writeDevAudit(r, "device.service_changed", d.ID, d.Name, before+" → "+after)
	}
	writeJSON(w, s.devView(d, true))
}

func (s *Server) handleServiceDelete(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	d := s.checkDevOr404(w, r, false)
	if d == nil {
		return
	}
	sid := r.PathValue("sid")
	keep := d.Services[:0:0]
	var gone *Service
	for _, sv := range d.Services {
		if sv.ID == sid {
			gone = sv
			continue
		}
		keep = append(keep, sv)
	}
	if gone == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	if !gone.Paused && d.lastCheck() {
		http.Error(w, noCheckMsg, http.StatusConflict)
		return
	}
	d.Services = keep
	s.st.markDirty()
	s.markChecks()
	s.writeDevAudit(r, "device.service_removed", d.ID, d.Name, gone.describe())
	writeJSON(w, s.devView(d, true))
}

func (s *Server) handleServiceCheck(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	d := s.checkDevOr404(w, r, false)
	if d == nil {
		s.st.mu.Unlock()
		return
	}
	sid := r.PathValue("sid")
	if d.service(sid) == nil {
		s.st.mu.Unlock()
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	if d.service(sid).isMetric() {
		s.st.mu.Unlock()
		http.Error(w, "measured by the device's own agent every minute: it cannot be checked now", http.StatusConflict)
		return
	}
	if d.Probe != "" {
		s.st.mu.Unlock()
		http.Error(w, remoteCheckMsg, http.StatusConflict)
		return
	}
	devID := d.ID
	s.st.mu.Unlock()
	s.loc.mu.Lock()
	if s.loc.busy[sid] {
		s.loc.mu.Unlock()
		http.Error(w, "this service is being checked right now", http.StatusConflict)
		return
	}
	s.loc.busy[sid] = true
	s.loc.mu.Unlock()
	s.checkService(devID, sid)
	s.loc.mu.Lock()
	delete(s.loc.busy, sid)
	s.loc.mu.Unlock()
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	if d = s.checkDevByID(devID); d == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	writeJSON(w, s.devView(d, true))
}

const remoteCheckMsg = "this device is checked by a remote probe agent, on that agent's own schedule: the master cannot check it now"

const noCheckMsg = "nothing would be checked on this device any more: keep the ping or at least one service running"

func (s *Server) handleServicePause(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Paused bool `json:"paused"`
	}
	if !readJSON(w, r, &req, 1024) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	d := s.checkDevOr404(w, r, false)
	if d == nil {
		return
	}
	sv := d.service(r.PathValue("sid"))
	if sv == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	if req.Paused == sv.Paused {
		writeJSON(w, s.devView(d, true))
		return
	}
	if req.Paused && d.lastCheck() {
		http.Error(w, noCheckMsg, http.StatusConflict)
		return
	}
	if !req.Paused {
		if err := s.probeRoom(d.Checks); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
	}
	sv.Paused = req.Paused
	if !sv.Paused {
		sv.Fails = 0 // resumed: decide afresh from the next checks
	}
	s.loc.mu.Lock()
	delete(s.loc.next, sv.ID)
	s.loc.mu.Unlock()
	s.st.markDirty()
	s.markChecks()
	s.writeDevAudit(r, map[bool]string{true: "device.service_paused", false: "device.service_resumed"}[sv.Paused], d.ID, d.Name, sv.describe())
	writeJSON(w, s.devView(d, true))
}
