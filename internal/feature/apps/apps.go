// Package apps reports which common applications are installed (read-only).
package apps

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"coremon/internal/feature"
	"coremon/internal/proto"
)

type App struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
	Full      string `json:"full,omitempty"`
	Package   string `json:"package,omitempty"`

	Source        string `json:"source,omitempty"`
	SourceVersion string `json:"source_version,omitempty"`
	Origin        string `json:"origin,omitempty"`
	Running       string `json:"running,omitempty"`
	Boot          string `json:"boot,omitempty"`
	Ports         []Port `json:"ports,omitempty"`
}

type Port struct {
	Port  int    `json:"port"`
	Proto string `json:"proto"`
	Local bool   `json:"local"`
	Addr  string `json:"addr,omitempty"`
}

type System struct {
	Active  map[string]bool
	Enabled map[string]bool
	Ports   map[string][]Port
}

type State struct {
	ScannedAt int64 `json:"scanned_at"`
	Apps      []App `json:"apps"`
}

type def struct {
	name  string
	re    *regexp.Regexp
	multi bool

	units []string
}

// rpmDef: the RPM package names of an application (Rocky Linux, AlmaLinux,
// RHEL, EPEL), and the extra systemd units there. ver: for an application
// with versions side by side (multi), the key is taken from the package
// version when the name has none: 1 = major, 2 = major.minor.
type rpmDef struct {
	re    *regexp.Regexp
	ver   int
	units []string
}

var rpmDefs = map[string]rpmDef{
	"Apache":                {regexp.MustCompile(`^httpd$`), 0, []string{"httpd.service"}},
	"BIND9":                 {regexp.MustCompile(`^bind$`), 0, nil},
	"Certbot":               {regexp.MustCompile(`^certbot$`), 0, nil},
	"Docker":                {regexp.MustCompile(`^(?:docker-ce|moby-engine)$`), 0, nil},
	"Fail2ban":              {regexp.MustCompile(`^fail2ban(?:-server)?$`), 0, nil},
	"ISC DHCP server":       {regexp.MustCompile(`^dhcp-server$`), 0, []string{"dhcpd.service", "dhcpd6.service"}},
	"Kea DHCP":              {regexp.MustCompile(`^kea$`), 0, nil},
	"Java":                  {regexp.MustCompile(`^java-(\d+(?:\.\d+\.\d+)?)-openjdk(?:-headless|-devel)?$`), 0, nil},
	"MariaDB/MySQL":         {regexp.MustCompile(`^(?:mariadb-server|mysql-server|mysql-community-server)$`), 0, []string{"mysqld.service"}},
	"Nginx":                 {regexp.MustCompile(`^nginx(?:-core)?$`), 0, nil},
	"Node.js":               {regexp.MustCompile(`^nodejs$`), 0, nil},
	"OpenSSL":               {regexp.MustCompile(`^openssl-libs$`), 0, nil},
	"OpenVPN":               {regexp.MustCompile(`^openvpn$`), 0, nil},
	"PHP":                   {regexp.MustCompile(`^php-(?:cli|fpm|common)$`), 2, []string{"php-fpm.service"}},
	"Podman":                {regexp.MustCompile(`^podman$`), 0, nil},
	"PostgreSQL":            {regexp.MustCompile(`^postgresql(\d+)?-server$`), 1, []string{"postgresql.service", "postgresql-{v}.service"}},
	"Postfix":               {regexp.MustCompile(`^postfix$`), 0, nil},
	"Python":                {regexp.MustCompile(`^(?:python(3\.\d+)|python3)$`), 2, nil},
	"Redis":                 {regexp.MustCompile(`^redis$`), 0, nil},
	"Samba":                 {regexp.MustCompile(`^samba$`), 0, []string{"smb.service", "nmb.service"}},
	"SSH server":            {regexp.MustCompile(`^openssh-server$`), 0, nil},
	"sudo":                  {regexp.MustCompile(`^sudo$`), 0, nil},
	"WireGuard":             {regexp.MustCompile(`^wireguard-tools$`), 0, nil},
	"Exim":                  {regexp.MustCompile(`^exim$`), 0, []string{"exim.service"}},
	"ProFTPD":               {regexp.MustCompile(`^proftpd$`), 0, nil},
	"Tomcat":                {regexp.MustCompile(`^tomcat$`), 1, []string{"tomcat.service"}},
	"Mosquitto":             {regexp.MustCompile(`^mosquitto$`), 0, nil},
	"Grafana":               {regexp.MustCompile(`^grafana(?:-enterprise)?$`), 0, nil},
	"Tailscale":             {regexp.MustCompile(`^tailscale$`), 0, nil},
	"QEMU":                  {regexp.MustCompile(`^qemu-kvm(?:-core)?$`), 0, nil},
	"Unbound":               {regexp.MustCompile(`^unbound$`), 0, nil},
	"HAProxy":               {regexp.MustCompile(`^haproxy$`), 0, nil},
	"Squid":                 {regexp.MustCompile(`^squid$`), 0, nil},
	"Chrony":                {regexp.MustCompile(`^chrony$`), 0, nil},
	"NTPsec":                {regexp.MustCompile(`^ntpsec$`), 0, nil},
	"vsftpd":                {regexp.MustCompile(`^vsftpd$`), 0, nil},
	"ClamAV":                {regexp.MustCompile(`^clamav$`), 0, []string{"clamd@*.service"}},
	"Dovecot":               {regexp.MustCompile(`^dovecot$`), 0, nil},
	"InfluxDB 1":            {regexp.MustCompile(`^influxdb$`), 0, nil},
	"InfluxDB 2":            {regexp.MustCompile(`^influxdb2$`), 0, nil},
	"Webmin":                {regexp.MustCompile(`^webmin$`), 0, nil},
	"Proxmox VE":            {},
	"Proxmox Backup Server": {},
}

var defs = []def{
	{"Apache", regexp.MustCompile(`^apache2$`), false, []string{"apache2.service"}},
	{"BIND9", regexp.MustCompile(`^bind9$`), false, []string{"named.service", "bind9.service"}},
	{"Certbot", regexp.MustCompile(`^certbot$`), false, nil},
	{"Docker", regexp.MustCompile(`^(?:docker-ce|docker\.io)$`), false, []string{"docker.service"}},
	{"Fail2ban", regexp.MustCompile(`^fail2ban$`), false, []string{"fail2ban.service"}},
	{"ISC DHCP server", regexp.MustCompile(`^isc-dhcp-server$`), false, []string{"isc-dhcp-server.service", "isc-dhcp-server6.service"}},

	{"Kea DHCP", regexp.MustCompile(`^(?:isc-)?kea-dhcp[46](?:-server)?$`), false, []string{
		"kea-dhcp4-server.service", "kea-dhcp6-server.service", "isc-kea-dhcp4-server.service", "isc-kea-dhcp6-server.service", "kea-dhcp4.service", "kea-dhcp6.service"}},
	{"Java", regexp.MustCompile(`^openjdk-(\d+)-(?:jre|jre-headless|jdk|jdk-headless)$`), true, nil},
	{"MariaDB/MySQL", regexp.MustCompile(`^(?:mariadb-server|mysql-server)(?:-core)?(?:-[\d.]+)?$`), false, []string{"mariadb.service", "mysql.service"}},
	{"Nginx", regexp.MustCompile(`^nginx(?:-(?:full|light|extras|core))?$`), false, []string{"nginx.service"}},
	{"Node.js", regexp.MustCompile(`^nodejs$`), false, nil},
	{"OpenSSL", regexp.MustCompile(`^libssl3t?6?4?$`), false, nil},
	{"OpenVPN", regexp.MustCompile(`^openvpn$`), false, []string{"openvpn@*.service", "openvpn-server@*.service", "openvpn-client@*.service"}},
	{"PHP", regexp.MustCompile(`^(?:php(\d+\.\d+)-(?:cli|fpm|common)|libapache2-mod-php(\d+\.\d+))$`), true, []string{"php{v}-fpm.service"}},
	{"Podman", regexp.MustCompile(`^podman$`), false, nil},
	{"PostgreSQL", regexp.MustCompile(`^postgresql-(\d+)$`), true, []string{"postgresql@{v}-*.service"}},
	{"Postfix", regexp.MustCompile(`^postfix$`), false, []string{"postfix@*.service", "postfix.service"}},
	{"Python", regexp.MustCompile(`^python(3\.\d+)$`), true, nil},
	{"Redis", regexp.MustCompile(`^redis-server$`), false, []string{"redis-server.service", "redis.service"}},
	{"Samba", regexp.MustCompile(`^samba$`), false, []string{"smbd.service", "nmbd.service"}},
	{"SSH server", regexp.MustCompile(`^openssh-server$`), false, []string{"ssh.service", "ssh.socket", "sshd.service"}},
	{"sudo", regexp.MustCompile(`^sudo$`), false, nil},
	{"WireGuard", regexp.MustCompile(`^wireguard-tools$`), false, []string{"wg-quick@*.service"}},
	{"Exim", regexp.MustCompile(`^exim4-daemon-(?:light|heavy)$`), false, []string{"exim4.service"}},
	{"ProFTPD", regexp.MustCompile(`^proftpd-(?:core|basic)$`), false, []string{"proftpd.service"}},
	{"Tomcat", regexp.MustCompile(`^tomcat(\d+)$`), true, []string{"tomcat{v}.service"}},
	{"Mosquitto", regexp.MustCompile(`^mosquitto$`), false, []string{"mosquitto.service"}},
	{"Grafana", regexp.MustCompile(`^grafana(?:-enterprise)?$`), false, []string{"grafana-server.service"}},
	{"Tailscale", regexp.MustCompile(`^tailscale$`), false, []string{"tailscaled.service"}},
	{"QEMU", regexp.MustCompile(`^(?:qemu-system-[a-z0-9]+|pve-qemu-kvm)$`), false, nil},
	{"Unbound", regexp.MustCompile(`^unbound$`), false, []string{"unbound.service"}},
	{"HAProxy", regexp.MustCompile(`^haproxy$`), false, []string{"haproxy.service"}},
	{"Squid", regexp.MustCompile(`^squid(?:-openssl)?$`), false, []string{"squid.service"}},
	{"Chrony", regexp.MustCompile(`^chrony$`), false, []string{"chrony.service", "chronyd.service"}},
	{"NTPsec", regexp.MustCompile(`^ntpsec$`), false, []string{"ntpsec.service"}},
	{"vsftpd", regexp.MustCompile(`^vsftpd$`), false, []string{"vsftpd.service"}},
	// ClamAV: one row for the scanner and its signature updates.
	{"ClamAV", regexp.MustCompile(`^clamav(?:-daemon)?$`), false, []string{"clamav-daemon.service", "clamav-freshclam.service"}},
	{"Dovecot", regexp.MustCompile(`^dovecot-core$`), false, []string{"dovecot.service"}},
	// InfluxDB 1 and 2 are different products: a row each.
	{"InfluxDB 1", regexp.MustCompile(`^influxdb$`), false, []string{"influxdb.service"}},
	{"InfluxDB 2", regexp.MustCompile(`^influxdb2$`), false, []string{"influxdb.service"}},
	{"Webmin", regexp.MustCompile(`^webmin$`), false, []string{"webmin.service"}},
	// Proxmox: not in the Debian security data; the master looks them up in NVD.
	{"Proxmox VE", regexp.MustCompile(`^pve-manager$`), false, []string{"pveproxy.service"}},
	{"Proxmox Backup Server", regexp.MustCompile(`^proxmox-backup-server$`), false, []string{"proxmox-backup-proxy.service"}},
}

// platforms: packages that make a device a platform, the first found wins.
var platforms = []struct{ pkg, id, name string }{
	{"pve-manager", "pve", "Proxmox VE"},
	{"proxmox-backup-server", "pbs", "Proxmox Backup Server"},
}

// PlatformOf: the platform the dpkg status describes, or nil.
func PlatformOf(r io.Reader) *proto.Platform {
	info := installed(r)
	for _, p := range platforms {
		if i, ok := info[p.pkg]; ok {
			return &proto.Platform{ID: p.id, Name: p.name, Version: Readable(i.ver)}
		}
	}
	return nil
}

var platformCache struct {
	mu    sync.Mutex
	mtime time.Time
	size  int64
	p     *proto.Platform
}

// Platform: the platform of this device, read again only when the dpkg
// status file changes.
func Platform() *proto.Platform {
	fi, err := os.Stat(statusFile)
	if err != nil {
		return nil
	}
	c := &platformCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if !fi.ModTime().Equal(c.mtime) || fi.Size() != c.size {
		f, err := os.Open(statusFile)
		if err != nil {
			return nil
		}
		c.p = PlatformOf(f)
		f.Close()
		c.mtime, c.size = fi.ModTime(), fi.Size()
	}
	if c.p == nil {
		return nil
	}
	p := *c.p
	return &p
}

func UnitPatterns() []string {
	var out []string
	for _, d := range defs {
		out = append(out, d.units...)
		out = append(out, rpmDefs[d.name].units...)
	}
	return out
}

// forRPM: the definition with the RPM names and units.
func (d def) forRPM() (def, int) {
	r := rpmDefs[d.name]
	d.re, d.units = r.re, append(append([]string{}, d.units...), r.units...)
	return d, r.ver
}

// verKey: the first n parts of a version (major or major.minor).
func verKey(v string, n int) string {
	parts := strings.SplitN(v, ".", n+1)
	if len(parts) < n {
		return ""
	}
	return strings.Join(parts[:n], ".")
}

func unitMatch(p, v, unit string) bool {
	p = strings.ReplaceAll(p, "{v}", v)
	ok, _ := path.Match(p, unit)
	return ok
}

func (d def) service(a *App, key string, sys *System) {
	if len(d.units) == 0 || sys == nil {
		return
	}
	var units []string
	for u := range sys.Active {
		for _, p := range d.units {
			if unitMatch(p, key, u) {
				units = append(units, u)
				break
			}
		}
	}
	if len(units) == 0 {
		return
	}
	sort.Strings(units)
	a.Running, a.Boot = "stopped", "disabled"
	seen := map[string]int{}
	for _, u := range units {
		if sys.Active[u] {
			a.Running = "running"
		}
		if sys.Enabled[u] {
			a.Boot = "enabled"
		}
		for _, p := range sys.Ports[u] {
			k := fmt.Sprintf("%d/%s", p.Port, p.Proto)
			if i, ok := seen[k]; ok {
				q := &a.Ports[i]
				q.Local = q.Local && p.Local
				if !strings.Contains(q.Addr, p.Addr) {
					q.Addr += ", " + p.Addr
				}
				continue
			}
			seen[k] = len(a.Ports)
			a.Ports = append(a.Ports, p)
		}
	}
	sort.Slice(a.Ports, func(i, j int) bool {
		if a.Ports[i].Port != a.Ports[j].Port {
			return a.Ports[i].Port < a.Ports[j].Port
		}
		return a.Ports[i].Proto < a.Ports[j].Proto
	})
}

const statusFile = "/var/lib/dpkg/status"

func Readable(v string) string {
	if i := strings.Index(v, ":"); i >= 0 {
		v = v[i+1:]
	}
	if i := strings.LastIndex(v, "-"); i > 0 {
		v = v[:i]
	}
	if i := strings.IndexAny(v, "+~"); i > 0 {
		v = v[:i]
	}
	return v
}

type pkgInfo struct {
	ver, src, srcVer string
}

func installed(r io.Reader) map[string]pkgInfo {
	out := map[string]pkgInfo{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	var pkg, ver, status, src string
	flush := func() {
		if pkg != "" && strings.HasSuffix(status, " installed") {
			p := pkgInfo{ver: ver, src: pkg, srcVer: ver}
			if f := strings.Fields(src); len(f) > 0 {
				p.src = f[0]
				if len(f) > 1 {
					p.srcVer = strings.Trim(f[1], "()")
				}
			}
			out[pkg] = p
		}
		pkg, ver, status, src = "", "", "", ""
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "Package: "):
			pkg = strings.TrimSpace(line[9:])
		case strings.HasPrefix(line, "Version: "):
			ver = strings.TrimSpace(line[9:])
		case strings.HasPrefix(line, "Status: "):
			status = strings.TrimSpace(line[8:])
		case strings.HasPrefix(line, "Source: "):
			src = strings.TrimSpace(line[8:])
		}
	}
	flush()
	return out
}

func scan(r io.Reader, sys *System, originOf func([]string) map[string]string) []App {
	return scanInfo(installed(r), false, sys, originOf)
}

// scanInfo: the applications among the installed packages; rpm: the
// packages are RPMs (their names differ from Debian's).
func scanInfo(info map[string]pkgInfo, rpm bool, sys *System, originOf func([]string) map[string]string) []App {
	pkgs := make(map[string]string, len(info))
	for p, i := range info {
		pkgs[p] = i.ver
	}
	names := make([]string, 0, len(pkgs))
	for p := range pkgs {
		names = append(names, p)
	}
	sort.Strings(names)
	var out []App
	for _, d := range defs {
		ver := 0
		if rpm {
			d, ver = d.forRPM()
		}
		rows := map[string]App{}
		for _, p := range names {
			if d.re == nil {
				break
			}
			m := d.re.FindStringSubmatch(p)
			if m == nil {
				continue
			}
			key := ""
			if d.multi {
				for _, g := range m[1:] {
					if g != "" {
						key = g
					}
				}
				if key == "" && ver > 0 {
					key = verKey(Readable(pkgs[p]), ver)
				}
			}
			if _, seen := rows[key]; seen {
				continue
			}
			name := d.name
			if key != "" {
				name += " " + key
			}
			a := App{Name: name, Installed: true, Version: Readable(pkgs[p]), Full: pkgs[p], Package: p,
				Source: info[p].src, SourceVersion: info[p].srcVer}
			d.service(&a, key, sys)
			rows[key] = a
		}
		if len(rows) == 0 {
			out = append(out, App{Name: d.name})
			continue
		}
		for _, a := range rows {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return naturalLess(strings.ToLower(out[i].Name), strings.ToLower(out[j].Name))
	})
	if originOf != nil {
		var names []string
		for _, a := range out {
			if a.Installed {
				names = append(names, a.Package)
			}
		}
		if len(names) > 0 {
			orig := originOf(names)
			for i := range out {
				out[i].Origin = orig[out[i].Package]
			}
		}
	}
	return out
}

func parseSources(out string) map[string]string {
	m := map[string]string{}
	last := ""
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		switch {
		case len(f) >= 3 && isNumber(f[0]):
			last = sourceKey(f[1:])
		case len(f) >= 2 && f[0] == "release" && last != "":
			for _, kv := range strings.Split(strings.Join(f[1:], " "), ",") {
				if strings.HasPrefix(kv, "o=") {
					m[last] = strings.TrimSpace(kv[2:])
				}
			}
			last = ""
		}
	}
	return m
}

func sourceKey(f []string) string {
	if n := len(f); n > 0 && f[n-1] == "Packages" {
		f = f[:n-1]
	}
	return strings.Join(f, " ")
}

func isNumber(s string) bool {
	if s == "" || s[0] == '-' && len(s) == 1 {
		return false
	}
	for i, c := range s {
		if (c < '0' || c > '9') && !(i == 0 && c == '-') {
			return false
		}
	}
	return true
}

func parseOrigins(out string, sources map[string]string) map[string]string {
	res := map[string]string{}
	type ver struct {
		installed bool
		origins   []string
	}
	var pkg, cand string
	var vers map[string]*ver
	var cur *ver
	flush := func() {
		if pkg == "" {
			return
		}
		pick := func(v *ver) string {
			for _, o := range v.origins {
				if o != "" {
					return o
				}
			}
			return ""
		}
		o := ""
		for _, v := range vers {
			if v.installed {
				o = pick(v)
			}
		}
		if o == "" && vers[cand] != nil {
			o = pick(vers[cand])
		}
		res[pkg] = o
		pkg, cand, vers, cur = "", "", nil, nil
	}
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "":
		case !strings.HasPrefix(line, " ") && strings.HasSuffix(t, ":"):
			flush()
			pkg, vers = strings.TrimSuffix(t, ":"), map[string]*ver{}
		case pkg == "":
		case strings.HasPrefix(t, "Candidate:"):
			cand = strings.TrimSpace(strings.TrimPrefix(t, "Candidate:"))
		case strings.HasPrefix(t, "Installed:"), strings.HasPrefix(t, "Version table:"):
		default:
			f := strings.Fields(t)
			star := f[0] == "***"
			if star {
				f = f[1:]
			}
			if len(f) == 2 && isNumber(f[1]) {
				cur = &ver{installed: star}
				vers[f[0]] = cur
			} else if len(f) >= 2 && isNumber(f[0]) && cur != nil {
				cur.origins = append(cur.origins, sources[sourceKey(f[1:])])
			}
		}
	}
	flush()
	return res
}

func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		da, db := digits(a), digits(b)
		if da != "" && db != "" {
			if len(da) != len(db) {
				return len(da) < len(db)
			}
			if da != db {
				return da < db
			}
			a, b = a[len(da):], b[len(db):]
			continue
		}
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		a, b = a[1:], b[1:]
	}
	return len(a) < len(b)
}

func digits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

func Scan() *State {
	st := &State{ScannedAt: time.Now().Unix()}
	f, err := os.Open(statusFile)
	if err != nil {
		if os.Getenv("COREMON_FAKE") == "1" {
			status := fakeStatus
			if v := os.Getenv("COREMON_FAKE_PVE"); v != "" {
				status += "\nPackage: pve-manager\nStatus: install ok installed\nVersion: " + v + "\n"
			}
			st.Apps = scan(strings.NewReader(status), fakeSystem(), func(p []string) map[string]string {
				m := map[string]string{}
				for _, n := range p {
					m[n] = "Debian"
					if n == "pve-manager" {
						m[n] = "Proxmox"
					}
				}
				return m
			})
			return st
		}
		if out, ok := rpmQuery(); ok {
			info, origin := parseRPM(out)
			st.Apps = scanInfo(info, true, collectSystem(), func([]string) map[string]string { return origin })
			return st
		}
		st.Apps = scan(strings.NewReader(""), nil, nil)
		return st
	}
	defer f.Close()
	st.Apps = scan(f, collectSystem(), packageOrigins)
	return st
}

func fakeSystem() *System {
	return &System{
		Active:  map[string]bool{"apache2.service": true, "ssh.service": true, "mariadb.service": true, "php8.2-fpm.service": false},
		Enabled: map[string]bool{"apache2.service": true, "ssh.service": true, "mariadb.service": false},
		Ports: map[string][]Port{
			"apache2.service": {{Port: 80, Proto: "tcp", Addr: "0.0.0.0"}, {Port: 443, Proto: "tcp", Addr: "0.0.0.0"}, {Port: 80, Proto: "tcp", Addr: "::"}},
			"ssh.service":     {{Port: 22, Proto: "tcp", Addr: "0.0.0.0"}},
			"mariadb.service": {{Port: 3306, Proto: "tcp", Local: true, Addr: "127.0.0.1"}},
		},
	}
}

type Apps struct{}

func New() *Apps { return &Apps{} }

func (*Apps) Info() proto.FeatureInfo {
	return proto.FeatureInfo{Name: "apps", Title: "Applications",
		Actions: []proto.ActionSpec{{Name: "scan", Title: "Scan applications", Timeout: 120}}}
}

func (*Apps) Call(c *feature.Ctx, action string) (any, error) {
	if action != "scan" {
		return nil, errUnknown(action)
	}
	s := Scan()
	n := 0
	for _, a := range s.Apps {
		if a.Installed {
			n++
		}
	}
	c.Log("%d application(s) found", n)
	return map[string]any{"state": s}, nil
}

type errUnknown string

func (e errUnknown) Error() string { return "unknown action " + string(e) }

const fakeStatus = `Package: apache2
Status: install ok installed
Version: 2.4.62-1~deb12u2

Package: php8.2-cli
Status: install ok installed
Version: 8.2.26-1~deb12u1

Package: php7.4-cli
Status: install ok installed
Version: 7.4.33-1+deb11u5

Package: python3.11
Status: install ok installed
Version: 3.11.2-6+deb12u4

Package: openssh-server
Status: install ok installed
Source: openssh
Version: 1:9.2p1-2+deb12u3

Package: libssl3
Status: install ok installed
Source: openssl
Version: 3.0.15-1~deb12u1

Package: mariadb-server
Status: install ok installed
Source: mariadb
Version: 1:10.11.6-0+deb12u1

Package: nginx
Status: deinstall ok config-files
Version: 1.22.1-9
`
