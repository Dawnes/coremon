// Package apps reports which common applications are installed and their
// versions, from the package manager's own records (/var/lib/dpkg/status).
// Nothing is started or changed: the scan only reads.
package apps

import (
	"bufio"
	"io"
	"os"
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
}

var defs = []def{
	{"Apache", regexp.MustCompile(`^apache2$`), false},
	{"Docker", regexp.MustCompile(`^(?:docker-ce|docker\.io)$`), false},
	{"Fail2ban", regexp.MustCompile(`^fail2ban$`), false},
	{"Java", regexp.MustCompile(`^openjdk-(\d+)-(?:jre|jre-headless|jdk|jdk-headless)$`), true},
	{"MariaDB/MySQL", regexp.MustCompile(`^(?:mariadb-server|mysql-server)(?:-core)?(?:-[\d.]+)?$`), false},
	{"Nginx", regexp.MustCompile(`^nginx(?:-(?:full|light|extras|core))?$`), false},
	{"Node.js", regexp.MustCompile(`^nodejs$`), false},
	{"OpenSSL", regexp.MustCompile(`^libssl3t?6?4?$`), false},
	{"PHP", regexp.MustCompile(`^(?:php(\d+\.\d+)-(?:cli|fpm|common)|libapache2-mod-php(\d+\.\d+))$`), true},
	{"PostgreSQL", regexp.MustCompile(`^postgresql-(\d+)$`), true},
	{"Postfix", regexp.MustCompile(`^postfix$`), false},
	{"Python", regexp.MustCompile(`^python(3\.\d+)$`), true},
	{"Redis", regexp.MustCompile(`^redis-server$`), false},
	{"Samba", regexp.MustCompile(`^samba$`), false},
	{"SSH server", regexp.MustCompile(`^openssh-server$`), false},
	{"WireGuard", regexp.MustCompile(`^wireguard-tools$`), false},
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

// installed reads dpkg status contents: installed package -> version.
func installed(r io.Reader) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	var pkg, ver, status string
	flush := func() {
		if pkg != "" && strings.HasSuffix(status, " installed") {
			out[pkg] = ver
		}
		pkg, ver, status = "", "", ""
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
		}
	}
	flush()
	return out
}

// scan builds the rows from dpkg status contents: a-z, one row per version,
// and a "not detected" row for applications that are absent.
func scan(r io.Reader) []App {
	pkgs := installed(r)
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
			rows[key] = App{Name: name, Installed: true, Version: Readable(pkgs[p]), Full: pkgs[p], Package: p}
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
	return out
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
			st.Apps = scan(strings.NewReader(fakeStatus))
			return st
		}
		st.Apps = scan(strings.NewReader(""))
		return st
	}
	defer f.Close()
	st.Apps = scan(f)
	return st
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
Version: 1:9.2p1-2+deb12u3

Package: libssl3
Status: install ok installed
Version: 3.0.15-1~deb12u1

Package: mariadb-server
Status: install ok installed
Version: 1:10.11.6-0+deb12u1

Package: nginx
Status: deinstall ok config-files
Version: 1.22.1-9
`
