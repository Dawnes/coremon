package master

// Services of agentless devices: the master opens a TCP connection to each
// service's port (and, where the service greets first, reads the greeting)
// to see whether it is up. Nothing is sent and nothing is logged in to.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Service struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Preset   string `json:"preset,omitempty"` // key of a default service; "" = custom
	Port     int    `json:"port"`
	Greeting bool   `json:"greeting,omitempty"` // the service sends a greeting first: wait for it
	Interval int    `json:"interval"`
	Retries  int    `json:"retries"`
	Timeout  int    `json:"timeout"`
	Reverse  bool   `json:"reverse,omitempty"` // reachable counts as down
	Paused   bool   `json:"paused,omitempty"`  // not checked until resumed

	// Results: kept in checks.dat, not in state.dat.
	Checked   bool       `json:"-"`
	Up        bool       `json:"-"`
	Since     int64      `json:"-"`
	LastCheck int64      `json:"-"`
	LastOK    int64      `json:"-"`
	RTTUs     int64      `json:"-"`
	Fails     int        `json:"-"`
	LastError string     `json:"-"`
	Banner    string     `json:"-"`
	Hist      []histSlot `json:"-"`
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
	Key      string `json:"key"`
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Greeting bool   `json:"greeting"`
	Group    string `json:"group"`
}

// The default services: their port and whether they greet first are known.
var svcPresets = []svcPreset{
	{"ssh", "SSH", 22, true, "Remote access"},
	{"telnet", "Telnet", 23, false, "Remote access"},
	{"rdp", "RDP", 3389, false, "Remote access"},
	{"vnc", "VNC", 5900, true, "Remote access"},
	{"http", "HTTP", 80, false, "Web"},
	{"https", "HTTPS", 443, false, "Web"},
	{"proxmox", "Proxmox", 8006, false, "Web"},
	{"smtp", "SMTP", 25, true, "Mail"},
	{"smtps", "SMTPS", 465, false, "Mail"},
	{"submission", "Submission", 587, true, "Mail"},
	{"pop3", "POP3", 110, true, "Mail"},
	{"pop3s", "POP3S", 995, false, "Mail"},
	{"imap", "IMAP", 143, true, "Mail"},
	{"imaps", "IMAPS", 993, false, "Mail"},
	{"ftp", "FTP", 21, true, "Files and directories"},
	{"smb", "SMB", 445, false, "Files and directories"},
	{"nfs", "NFS", 2049, false, "Files and directories"},
	{"ldap", "LDAP", 389, false, "Files and directories"},
	{"ldaps", "LDAPS", 636, false, "Files and directories"},
	{"mysql", "MySQL/MariaDB", 3306, true, "Databases"},
	{"postgresql", "PostgreSQL", 5432, false, "Databases"},
	{"redis", "Redis", 6379, false, "Databases"},
	{"mongodb", "MongoDB", 27017, false, "Databases"},
	{"dns", "DNS", 53, false, "Other"},
	{"mqtt", "MQTT", 1883, false, "Other"},
	{"ipp", "Printer IPP", 631, false, "Other"},
	{"jetdirect", "Printer raw", 9100, false, "Other"},
}

func presetByKey(k string) *svcPreset {
	for i := range svcPresets {
		if svcPresets[i].Key == k {
			return &svcPresets[i]
		}
	}
	return nil
}

// svcCheckFunc connects to addr:port; with greeting it waits for the first
// line the service sends. It returns the time to connect and that line.
type svcCheckFunc func(ctx context.Context, addr string, port int, greeting bool, timeout time.Duration) (time.Duration, string, error)

func tcpCheck(ctx context.Context, addr string, port int, greeting bool, timeout time.Duration) (time.Duration, string, error) {
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(addr, strconv.Itoa(port)))
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return 0, "", fmt.Errorf("no connection within %s", timeout)
		}
		return 0, "", dialError(err)
	}
	defer c.Close()
	rtt := time.Since(start)
	if !greeting {
		return rtt, "", nil
	}
	c.SetReadDeadline(time.Now().Add(timeout))
	line, err := bufio.NewReaderSize(c, 512).ReadSlice('\n')
	if len(line) == 0 && err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return 0, "", fmt.Errorf("connected, but no greeting within %s", timeout)
		}
		return 0, "", fmt.Errorf("connected, but no greeting: %v", unwrapOp(err))
	}
	return rtt, cleanText(string(line), maxBanner), nil
}

// dialError: a plain reason for a failed connection.
func dialError(err error) error {
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return errors.New("connection refused: nothing listens on this port")
	case errors.Is(err, syscall.EHOSTUNREACH):
		return errors.New("host unreachable")
	case errors.Is(err, syscall.ENETUNREACH):
		return errors.New("network unreachable")
	}
	var se *os.SyscallError
	if errors.As(err, &se) {
		return se.Err
	}
	return unwrapOp(err)
}

// ---- view ----

type serviceView struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Preset    string     `json:"preset,omitempty"`
	Port      int        `json:"port"`
	Greeting  bool       `json:"greeting"`
	Interval  int        `json:"interval"`
	Retries   int        `json:"retries"`
	Timeout   int        `json:"timeout"`
	Reverse   bool       `json:"reverse"`
	Paused    bool       `json:"paused"`
	Checked   bool       `json:"checked"`
	Up        bool       `json:"up"`
	Since     int64      `json:"since"`
	LastCheck int64      `json:"last_check"`
	LastOK    int64      `json:"last_ok"`
	RTTUs     int64      `json:"rtt_us"`
	Fails     int        `json:"fails"`
	LastError string     `json:"last_error,omitempty"`
	Banner    string     `json:"banner,omitempty"`
	Uptime    float64    `json:"uptime"`
	Hist      []histSlot `json:"hist,omitempty"`
}

func (s *Server) serviceView(sv *Service, now int64, full bool) serviceView {
	h, up := recentHist(sv.Hist, now)
	v := serviceView{ID: sv.ID, Name: sv.Name, Preset: sv.Preset, Port: sv.Port, Greeting: sv.Greeting, Interval: sv.Interval,
		Retries: sv.Retries, Timeout: sv.Timeout, Reverse: sv.Reverse, Paused: sv.Paused, Checked: sv.Checked, Up: sv.Up, Since: sv.Since,
		LastCheck: sv.LastCheck, LastOK: sv.LastOK, RTTUs: sv.RTTUs, Fails: sv.Fails, LastError: sv.LastError, Banner: sv.Banner, Uptime: up}
	if full {
		v.Hist = h
	}
	return v
}

// ---- results and scheduling ----

// recordService stores one check. Retries follow 30 s apart; after the
// last one fails the service is down, and it is then checked every 30 s
// until it is up again. Caller holds s.st.mu and s.loc.mu.
func (s *Server) recordService(sv *Service, rtt time.Duration, banner string, reached bool, cerr error, now time.Time) {
	t := now.Unix()
	ok := reached != sv.Reverse
	sv.LastCheck = t
	if ok {
		sv.LastOK, sv.Fails, sv.LastError = t, 0, ""
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
		s.loc.next[sv.ID] = now.Add(time.Duration(sv.Interval) * time.Second)
	} else {
		sv.Fails++
		if reached {
			sv.RTTUs, sv.LastError = rtt.Microseconds(), "reachable, but reverse mode expects it not to be"
		} else {
			sv.LastError = clip(cerr.Error(), 200)
		}
		if sv.Fails > sv.Retries && (!sv.Checked || sv.Up) {
			sv.Checked, sv.Up, sv.Since = true, false, t
		}
		s.loc.next[sv.ID] = now.Add(svcRetryEvery)
	}
	sv.Hist = addHist(sv.Hist, ok, t)
	s.markChecks()
}

type svcKey struct{ dev, svc string }

// dueServices: services whose next check is due, marked busy. Caller holds
// s.st.mu and s.loc.mu.
func (s *Server) dueServices(now time.Time) []svcKey {
	var due []svcKey
	known := map[string]bool{}
	i := 0
	for _, d := range s.st.st.Locals {
		for _, sv := range d.Services {
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

func (d *LocalDevice) service(id string) *Service {
	for _, sv := range d.Services {
		if sv.ID == id {
			return sv
		}
	}
	return nil
}

// checkService checks one service and records the result. Without locks held.
func (s *Server) checkService(devID, svcID string) {
	s.st.mu.Lock()
	d := s.st.st.Locals[devID]
	var sv *Service
	if d != nil {
		sv = d.service(svcID)
	}
	if sv == nil {
		s.st.mu.Unlock()
		return
	}
	addr, port, greet, timeout := d.Address, sv.Port, sv.Greeting, time.Duration(sv.Timeout)*time.Second
	s.st.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*timeout+time.Second)
	rtt, banner, err := s.svcCheck(ctx, addr, port, greet, timeout)
	cancel()
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	d = s.st.st.Locals[devID]
	if d == nil || d.Address != addr {
		return
	}
	if sv = d.service(svcID); sv == nil || sv.Port != port || sv.Greeting != greet {
		return
	}
	s.loc.mu.Lock()
	s.recordService(sv, rtt, banner, err == nil, err, time.Now())
	s.loc.mu.Unlock()
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
}

// apply checks and applies a request to sv; a default service keeps its
// port and greeting.
func (req *serviceReq) apply(sv *Service) error {
	if req.Preset != nil {
		if *req.Preset == "" {
			sv.Preset = ""
		} else if p := presetByKey(*req.Preset); p != nil {
			sv.Preset, sv.Port, sv.Greeting = p.Key, p.Port, p.Greeting
			if sv.Name == "" && req.Name == nil {
				sv.Name = p.Name
			}
		} else {
			return errors.New("unknown default service")
		}
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
	if sv.Port < 1 || sv.Port > 65535 {
		return errors.New("the port must be 1-65535")
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
		sv.Reverse = *req.Reverse
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
	d := fmt.Sprintf("%s port %d, every %ds, %d retries, timeout %ds", sv.Name, sv.Port, sv.Interval, sv.Retries, sv.Timeout)
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
	sv.Checked, sv.Up, sv.Since, sv.LastCheck, sv.LastOK, sv.RTTUs, sv.Fails, sv.LastError, sv.Banner, sv.Hist = false, false, 0, 0, 0, 0, 0, "", "", nil
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
	d := s.localOr404(w, r)
	if d == nil {
		return
	}
	if len(d.Services) >= maxServices {
		http.Error(w, fmt.Sprintf("at most %d services per device", maxServices), http.StatusBadRequest)
		return
	}
	sv := &Service{ID: "s_" + randHex(8), Interval: defaultSvcInterval, Retries: defaultSvcRetries, Timeout: defaultSvcTimeout}
	if err := req.apply(sv); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	d.Services = append(d.Services, sv)
	s.st.markDirty()
	s.writeLocalAudit(r, "device.service_added", d, sv.describe())
	writeJSON(w, s.localDeviceView(d, true))
}

func (s *Server) handleServiceUpdate(w http.ResponseWriter, r *http.Request) {
	var req serviceReq
	if !readJSON(w, r, &req, 4096) {
		return
	}
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	d := s.localOr404(w, r)
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
	before := sv.describe()
	restart := n.Port != sv.Port || n.Greeting != sv.Greeting || n.Reverse != sv.Reverse
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
		s.writeLocalAudit(r, "device.service_changed", d, before+" → "+after)
	}
	writeJSON(w, s.localDeviceView(d, true))
}

func (s *Server) handleServiceDelete(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	d := s.localOr404(w, r)
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
	if !gone.Paused && d.activeChecks() <= 1 {
		http.Error(w, noCheckMsg, http.StatusConflict)
		return
	}
	d.Services = keep
	s.st.markDirty()
	s.markChecks()
	s.writeLocalAudit(r, "device.service_removed", d, gone.describe())
	writeJSON(w, s.localDeviceView(d, true))
}

func (s *Server) handleServiceCheck(w http.ResponseWriter, r *http.Request) {
	s.st.mu.Lock()
	d := s.localOr404(w, r)
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
	if d = s.st.st.Locals[devID]; d == nil {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	writeJSON(w, s.localDeviceView(d, true))
}

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
	d := s.localOr404(w, r)
	if d == nil {
		return
	}
	sv := d.service(r.PathValue("sid"))
	if sv == nil {
		http.Error(w, "service not found", http.StatusNotFound)
		return
	}
	if req.Paused == sv.Paused {
		writeJSON(w, s.localDeviceView(d, true))
		return
	}
	if req.Paused && d.activeChecks() <= 1 {
		http.Error(w, noCheckMsg, http.StatusConflict)
		return
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
	s.writeLocalAudit(r, map[bool]string{true: "device.service_paused", false: "device.service_resumed"}[sv.Paused], d, sv.describe())
	writeJSON(w, s.localDeviceView(d, true))
}
