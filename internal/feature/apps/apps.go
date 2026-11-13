// Package apps reports which common applications are installed and their
// versions, from the package manager's own records (/var/lib/dpkg/status).
// Nothing is started or changed: the scan only reads.
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
	"time"

	"coremon/internal/feature"
	"coremon/internal/proto"
)

// App is one row of the Applications tab: an application, or one version
// of it when several are installed side by side.
type App struct {
	Name      string `json:"name"`              // "PHP", "PHP 8.2"
	Installed bool   `json:"installed"`         //
	Version   string `json:"version,omitempty"` // readable: "8.2.26"
	Full      string `json:"full,omitempty"`    // package version: "8.2.26-1~deb12u1"
	Package   string `json:"package,omitempty"` // the package it was found in
	// The source package and its version (how Debian and Ubuntu publish
	// security advisories), and the package source the installed version
	// comes from: the "o=" of its apt source ("Debian", "Ubuntu", "Docker",
	// ...); "" when no configured source has it (installed by hand). 0.3.6+.
	Source        string `json:"source,omitempty"`
	SourceVersion string `json:"source_version,omitempty"`
	Origin        string `json:"origin,omitempty"`
	Running       string `json:"running,omitempty"` // running | stopped | "" (no service of its own)
	Boot          string `json:"boot,omitempty"`    // enabled | disabled | "" (starts at boot or not)
	Ports         []Port `json:"ports,omitempty"`   // what its service listens on
}

// Port is one listening port of an application's service.
type Port struct {
	Port  int    `json:"port"`
	Proto string `json:"proto"`          // tcp | udp
	Local bool   `json:"local"`          // only reachable from the machine itself
	Addr  string `json:"addr,omitempty"` // the addresses, e.g. "0.0.0.0, ::"
}

// System is what the scan knows about services: which units exist, which
// are active, which start at boot and what they listen on.
type System struct {
	Active  map[string]bool   // unit -> active (only units that exist)
	Enabled map[string]bool   // unit -> starts at boot
	Ports   map[string][]Port // unit -> listening ports
}

// State is the scan result.
type State struct {
	ScannedAt int64 `json:"scanned_at"`
	Apps      []App `json:"apps"`
}

// def describes how an application is found. With multi, every captured
// version (e.g. PHP 7.4 and 8.2) is its own row; the regexp's first group
// is that version.
type def struct {
	name  string
	re    *regexp.Regexp
	multi bool
	// units are its services; "{v}" is the version of a multi row, "*" a
	// wildcard (instances such as openvpn@site.service). None: no service
	// of its own (Running shows "—").
	units []string
}

var defs = []def{
	{"Apache", regexp.MustCompile(`^apache2$`), false, []string{"apache2.service"}},
	{"BIND9", regexp.MustCompile(`^bind9$`), false, []string{"named.service", "bind9.service"}},
	{"Certbot", regexp.MustCompile(`^certbot$`), false, nil},
	{"Docker", regexp.MustCompile(`^(?:docker-ce|docker\.io)$`), false, []string{"docker.service"}},
	{"Fail2ban", regexp.MustCompile(`^fail2ban$`), false, []string{"fail2ban.service"}},
	{"ISC DHCP server", regexp.MustCompile(`^isc-dhcp-server$`), false, []string{"isc-dhcp-server.service", "isc-dhcp-server6.service"}},
	// Kea: Debian/Ubuntu packages, or ISC's own repository (isc-kea-...).
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
}

// UnitPatterns are all service patterns, for collecting their details.
func UnitPatterns() []string {
	var out []string
	for _, d := range defs {
		out = append(out, d.units...)
	}
	return out
}

// unitMatch reports whether unit matches pattern p ("*" = any characters;
// "{v}" is replaced by the row's version first).
func unitMatch(p, v, unit string) bool {
	p = strings.ReplaceAll(p, "{v}", v)
	ok, _ := path.Match(p, unit)
	return ok
}

// service fills Running, Boot and Ports of a row from the units of d.
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
		return // no service of its own on this system (e.g. PHP without FPM)
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

// Readable turns a Debian package version into what people call the version:
// "1:10.11.6-0+deb12u1" -> "10.11.6", "8.2.26-1~deb12u1" -> "8.2.26".
func Readable(v string) string {
	if i := strings.Index(v, ":"); i >= 0 {
		v = v[i+1:] // epoch
	}
	if i := strings.LastIndex(v, "-"); i > 0 {
		v = v[:i] // Debian revision
	}
	if i := strings.IndexAny(v, "+~"); i > 0 {
		v = v[:i] // +dfsg, ~rc, ...
	}
	return v
}

// pkgInfo is an installed package from the dpkg status.
type pkgInfo struct {
	ver, src, srcVer string
}

// installed reads dpkg status contents: installed package -> version and
// source package. "Source: name" or "Source: name (version)"; without it
// the source is the package itself, and without a version the package's.
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

// scan builds the rows from dpkg status contents: a-z, one row per version,
// and a "not detected" row for applications that are absent. sys (may be
// nil) adds the service state and ports; originOf (may be nil) tells for
// the found packages which package source they come from.
func scan(r io.Reader, sys *System, originOf func([]string) map[string]string) []App {
	info := installed(r)
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
		rows := map[string]App{} // version key -> row
		for _, p := range names {
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

// parseSources reads the "Package files:" part of `apt-cache policy`: each
// source ("<url> <suite/component> <arch>") and the origin ("o=") of its
// release, e.g. "http://deb.debian.org/debian bookworm/main amd64" -> "Debian".
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

// sourceKey: "<url> <suite/component> <arch> [Packages]" without the word
// Packages, so both parts of the policy output give the same key.
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

// parseOrigins reads `apt-cache policy <pkg>...`: for each package the
// origin of the installed version's sources. When the installed version
// is no longer offered (an update is waiting), the candidate's sources
// count; with neither (installed by hand) the origin stays "".
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
			if len(f) == 2 && isNumber(f[1]) { // "<version> <priority>"
				cur = &ver{installed: star}
				vers[f[0]] = cur
			} else if len(f) >= 2 && isNumber(f[0]) && cur != nil { // "<priority> <source>"
				cur.origins = append(cur.origins, sources[sourceKey(f[1:])])
			}
		}
	}
	flush()
	return res
}

// naturalLess compares so that "php 7.4" < "php 8.2" < "php 10.0".
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

// Scan reads the package manager's records now.
func Scan() *State {
	st := &State{ScannedAt: time.Now().Unix()}
	f, err := os.Open(statusFile)
	if err != nil {
		if os.Getenv("COREMON_FAKE") == "1" { // development builds
			st.Apps = scan(strings.NewReader(fakeStatus), fakeSystem(), func(p []string) map[string]string {
				m := map[string]string{}
				for _, n := range p {
					m[n] = "Debian"
				}
				return m
			})
			return st
		}
		st.Apps = scan(strings.NewReader(""), nil, nil)
		return st
	}
	defer f.Close()
	st.Apps = scan(f, collectSystem(), packageOrigins)
	return st
}

// fakeSystem: services for development builds (COREMON_FAKE=1).
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

// Apps implements feature.Feature: the "Refresh now" scan.
type Apps struct{}

// New returns the applications feature.
func New() *Apps { return &Apps{} }

// Info describes the feature. The scan only reads, so the master allows it
// on report-only systems too.
func (*Apps) Info() proto.FeatureInfo {
	return proto.FeatureInfo{Name: "apps", Title: "Applications",
		Actions: []proto.ActionSpec{{Name: "scan", Title: "Scan applications", Timeout: 120}}}
}

// Call runs the scan.
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
