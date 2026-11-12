package apps

import (
	"strings"
	"testing"
)

const policyAll = `Package files:
 100 /var/lib/dpkg/status
     release a=now
 500 https://download.docker.com/linux/debian bookworm/stable amd64 Packages
     release o=Docker,a=bookworm,l=Docker CE,c=stable,b=amd64
     origin download.docker.com
 500 http://security.debian.org/debian-security bookworm-security/main amd64 Packages
     release v=12,o=Debian,a=stable-security,n=bookworm,l=Debian-Security,c=main,b=amd64
     origin security.debian.org
 500 http://deb.debian.org/debian bookworm/main amd64 Packages
     release v=12.8,o=Debian,a=stable,n=bookworm,l=Debian,c=main,b=amd64
     origin deb.debian.org
Pinned packages:
`

const policyPkgs = `libssl3:
  Installed: 3.0.15-1~deb12u1
  Candidate: 3.0.17-1~deb12u2
  Version table:
     3.0.17-1~deb12u2 500
        500 http://security.debian.org/debian-security bookworm-security/main amd64 Packages
 *** 3.0.15-1~deb12u1 100
        100 /var/lib/dpkg/status
apache2:
  Installed: 2.4.62-1~deb12u2
  Candidate: 2.4.62-1~deb12u2
  Version table:
 *** 2.4.62-1~deb12u2 500
        500 http://deb.debian.org/debian bookworm/main amd64 Packages
        100 /var/lib/dpkg/status
docker-ce:
  Installed: 5:27.3.1-1~debian.12~bookworm
  Candidate: 5:27.3.1-1~debian.12~bookworm
  Version table:
 *** 5:27.3.1-1~debian.12~bookworm 500
        500 https://download.docker.com/linux/debian bookworm/stable amd64 Packages
        100 /var/lib/dpkg/status
mytool:
  Installed: 1.0
  Candidate: 1.0
  Version table:
 *** 1.0 100
        100 /var/lib/dpkg/status
`

func TestPackageOrigins(t *testing.T) {
	src := parseSources(policyAll)
	if src["http://deb.debian.org/debian bookworm/main amd64"] != "Debian" || src["https://download.docker.com/linux/debian bookworm/stable amd64"] != "Docker" {
		t.Fatalf("sources: %v", src)
	}
	got := parseOrigins(policyPkgs, src)
	want := map[string]string{
		"libssl3":   "Debian", // installed version superseded: the candidate's source counts
		"apache2":   "Debian",
		"docker-ce": "Docker",
		"mytool":    "", // installed by hand
	}
	for p, o := range want {
		if got[p] != o {
			t.Errorf("%s: origin %q, want %q", p, got[p], o)
		}
	}
}

func TestSourcePackages(t *testing.T) {
	status := fakeStatus + `
Package: python3.12
Status: install ok installed
Source: python3.12 (3.12.3-1ubuntu0.5)
Version: 3.12.3-1ubuntu0.5+b1
`
	rows := scan(strings.NewReader(status), nil, func(p []string) map[string]string {
		return map[string]string{"libssl3": "Debian"}
	})
	by := map[string]App{}
	for _, a := range rows {
		by[a.Name] = a
	}
	cases := []struct{ app, src, ver, origin string }{
		{"OpenSSL", "openssl", "3.0.15-1~deb12u1", "Debian"},
		{"Apache", "apache2", "2.4.62-1~deb12u2", ""},
		{"SSH server", "openssh", "1:9.2p1-2+deb12u3", ""},
		{"Python 3.12", "python3.12", "3.12.3-1ubuntu0.5", ""}, // binNMU: the source keeps its own version
	}
	for _, c := range cases {
		a := by[c.app]
		if a.Source != c.src || a.SourceVersion != c.ver || a.Origin != c.origin {
			t.Errorf("%s: source %q %q origin %q, want %q %q %q", c.app, a.Source, a.SourceVersion, a.Origin, c.src, c.ver, c.origin)
		}
	}
}
