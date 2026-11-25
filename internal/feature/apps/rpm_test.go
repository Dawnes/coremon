package apps

import "testing"

const testRPM = "httpd\t0\t2.4.63-1.el10\thttpd-2.4.63-1.el10.src.rpm\tRocky Enterprise Software Foundation\n" +
	"openssh-server\t0\t9.9p1-7.el10_3\topenssh-9.9p1-7.el10_3.src.rpm\tRocky Enterprise Software Foundation\n" +
	"openssl-libs\t1\t3.5.1-3.el10\topenssl-3.5.1-3.el10.src.rpm\tRocky Enterprise Software Foundation\n" +
	"openssl-libs\t1\t3.5.1-3.el10\topenssl-3.5.1-3.el10.src.rpm\tRocky Enterprise Software Foundation\n" +
	"python3\t0\t3.12.9-1.el10\tpython3.12-3.12.9-1.el10.src.rpm\tRocky Enterprise Software Foundation\n" +
	"python3.11\t0\t3.11.11-2.el10\tpython3.11-3.11.11-2.el10.src.rpm\tRocky Enterprise Software Foundation\n" +
	"php-cli\t0\t8.3.19-1.el10\tphp-8.3.19-1.el10.src.rpm\tRocky Enterprise Software Foundation\n" +
	"php-fpm\t0\t8.3.19-1.el10\tphp-8.3.19-1.el10.src.rpm\tRocky Enterprise Software Foundation\n" +
	"postgresql16-server\t0\t16.9-1PGDG.rhel10\tpostgresql16-16.9-1PGDG.rhel10.src.rpm\tPostgreSQL Global Development Group\n" +
	"java-21-openjdk-headless\t1\t21.0.7.0.6-1.el10\tjava-21-openjdk-21.0.7.0.6-1.el10.src.rpm\tRocky Enterprise Software Foundation\n" +
	"fail2ban-server\t0\t1.1.0-6.el10_0\tfail2ban-1.1.0-6.el10_0.src.rpm\tFedora Project\n" +
	"docker-ce\t3\t28.1.1-1.el10\tdocker-ce-28.1.1-1.el10.src.rpm\tDocker\n" +
	"gpg-pubkey\t0\t5a6340b3-6229229e\t(none)\t(none)\n" +
	"mytool\t0\t1.0-1\t(none)\t(none)\n"

func TestRPMScan(t *testing.T) {
	info, origin := parseRPM(testRPM)
	if _, ok := info["gpg-pubkey"]; ok {
		t.Error("gpg-pubkey listed")
	}
	if p := info["openssl-libs"]; p.src != "openssl" || p.srcVer != "1:3.5.1-3.el10" || p.ver != "1:3.5.1-3.el10" {
		t.Errorf("openssl-libs: %+v", p)
	}
	if origin["httpd"] != "Rocky Linux" || origin["fail2ban-server"] != "EPEL" || origin["docker-ce"] != "Docker" || origin["mytool"] != "" {
		t.Errorf("origins: %v", origin)
	}
	sys := &System{Active: map[string]bool{"httpd.service": true, "sshd.service": true, "php-fpm.service": true, "postgresql-16.service": false},
		Enabled: map[string]bool{"httpd.service": true, "sshd.service": true}, Ports: map[string][]Port{"httpd.service": {{Port: 80, Proto: "tcp"}}}}
	got := map[string]App{}
	for _, a := range scanInfo(info, famEL, sys, func([]string) map[string]string { return origin }) {
		got[a.Name] = a
	}
	want := map[string]string{"Apache": "2.4.63", "SSH server": "9.9p1", "OpenSSL": "3.5.1", "Python 3.12": "3.12.9", "Python 3.11": "3.11.11",
		"PHP 8.3": "8.3.19", "PostgreSQL 16": "16.9", "Java 21": "21.0.7.0.6", "Fail2ban": "1.1.0", "Docker": "28.1.1"}
	for n, v := range want {
		if a := got[n]; !a.Installed || a.Version != v {
			t.Errorf("%s: %+v, want version %s", n, a, v)
		}
	}
	if a := got["Apache"]; a.Running != "running" || a.Boot != "enabled" || len(a.Ports) != 1 || a.Origin != "Rocky Linux" || a.Source != "httpd" || a.SourceVersion != "2.4.63-1.el10" {
		t.Errorf("Apache: %+v", a)
	}
	if a := got["SSH server"]; a.Running != "running" || a.Source != "openssh" {
		t.Errorf("SSH server: %+v", a)
	}
	if a := got["PHP 8.3"]; a.Running != "running" {
		t.Errorf("PHP: %+v", a)
	}
	if a := got["PostgreSQL 16"]; a.Running != "stopped" {
		t.Errorf("PostgreSQL: %+v", a)
	}
	if a := got["Proxmox VE"]; a.Installed {
		t.Errorf("Proxmox on an RPM system: %+v", a)
	}
	if a := got["Nginx"]; a.Installed || a.Name != "Nginx" {
		t.Errorf("Nginx: %+v", a)
	}
	// Every family lists every application (no names: not packaged there).
	for fam, fd := range familyDefs {
		for _, d := range defs {
			if _, ok := fd[d.name]; !ok {
				t.Errorf("%s: %s missing", fam, d.name)
			}
		}
		if len(fd) != len(defs) {
			t.Errorf("%s: %d definitions for %d applications", fam, len(fd), len(defs))
		}
	}
	for _, d := range defs {
		if rpmDefs[d.name].re == nil && d.name != "Proxmox VE" && d.name != "Proxmox Backup Server" {
			t.Errorf("%s: no RPM names", d.name)
		}
	}
}

func TestSUSEScan(t *testing.T) {
	out := "apache2\t0\t2.4.58-150600.5.29.1\tapache2-2.4.58-150600.5.29.1.src.rpm\tSUSE LLC <https://www.suse.com/>\n" +
		"openssh-server\t0\t9.6p1-150600.6.15.2\topenssh-9.6p1-150600.6.15.2.src.rpm\tSUSE LLC <https://www.suse.com/>\n" +
		"libopenssl3\t0\t3.1.4-150600.5.21.1\topenssl-3-3.1.4-150600.5.21.1.src.rpm\tSUSE LLC <https://www.suse.com/>\n" +
		"python311\t0\t3.11.10-150600.3.10.1\tpython311-3.11.10-150600.3.10.1.src.rpm\tSUSE LLC <https://www.suse.com/>\n" +
		"php8-fpm\t0\t8.2.20-150600.3.6.1\tphp8-8.2.20-150600.3.6.1.src.rpm\topenSUSE\n" +
		"mariadb\t0\t10.11.9-150600.4.6.1\tmariadb-10.11.9-150600.4.6.1.src.rpm\tSUSE LLC <https://www.suse.com/>\n"
	info, origin := parseRPM(out)
	if origin["apache2"] != "SUSE" || origin["php8-fpm"] != "openSUSE" {
		t.Errorf("origins: %v", origin)
	}
	sys := &System{Active: map[string]bool{"apache2.service": true, "sshd.service": true, "php-fpm.service": true}, Enabled: map[string]bool{}, Ports: map[string][]Port{}}
	got := map[string]App{}
	for _, a := range scanInfo(info, famSUSE, sys, func([]string) map[string]string { return origin }) {
		got[a.Name] = a
	}
	for n, v := range map[string]string{"Apache": "2.4.58", "SSH server": "9.6p1", "OpenSSL": "3.1.4", "Python 3.11": "3.11.10", "PHP 8.2": "8.2.20", "MariaDB/MySQL": "10.11.9"} {
		if a := got[n]; !a.Installed || a.Version != v {
			t.Errorf("%s: %+v, want %s", n, a, v)
		}
	}
	if a := got["OpenSSL"]; a.Source != "openssl-3" {
		t.Errorf("OpenSSL source: %+v", a)
	}
	if a := got["Apache"]; a.Running != "running" {
		t.Errorf("Apache: %+v", a)
	}
	if !suseID("opensuse-leap") || !suseID("sles") || !suseID("opensuse-tumbleweed") || suseID("rocky") {
		t.Error("suseID")
	}
}

func TestApkScan(t *testing.T) {
	db := "C:Q1abc=\nP:openssh-server\nV:9.7_p1-r4\nA:x86_64\no:openssh\nm:Natanael Copa\n\nP:nginx\nV:1.26.2-r0\no:nginx\n\nP:php83-fpm\nV:8.3.13-r0\no:php83\n\nP:python3\nV:3.12.8-r1\no:python3\n\nP:postgresql16\nV:16.6-r0\no:postgresql16\n\nP:busybox\nV:1.36.1-r29\no:busybox\n"
	info, origin := parseApkDB(db)
	if p := info["openssh-server"]; p.src != "openssh" || p.srcVer != "9.7_p1-r4" || origin["openssh-server"] != "Alpine" {
		t.Errorf("openssh-server: %+v", p)
	}
	sys := &System{Active: map[string]bool{"sshd.service": true, "nginx.service": false, "php-fpm83.service": true}, Enabled: map[string]bool{"sshd.service": true}, Ports: map[string][]Port{}}
	got := map[string]App{}
	for _, a := range scanInfo(info, famApk, sys, func([]string) map[string]string { return origin }) {
		got[a.Name] = a
	}
	for n, v := range map[string]string{"SSH server": "9.7_p1", "Nginx": "1.26.2", "PHP 8.3": "8.3.13", "Python 3.12": "3.12.8", "PostgreSQL 16": "16.6"} {
		if a := got[n]; !a.Installed || a.Version != v {
			t.Errorf("%s: %+v, want %s", n, a, v)
		}
	}
	if a := got["SSH server"]; a.Running != "running" || a.Boot != "enabled" || a.Source != "openssh" {
		t.Errorf("SSH server: %+v", a)
	}
	if a := got["Nginx"]; a.Running != "stopped" {
		t.Errorf("Nginx: %+v", a)
	}
	if a := got["PHP 8.3"]; a.Running != "running" {
		t.Errorf("PHP: %+v", a)
	}
	// OpenRC's status lists.
	act := map[string]bool{}
	parseRCStatus(" sshd                                         [  started  ]\n nginx                                        [  stopped  ]\n crond                                        [  started 00:10:05 (0) ]\n", act)
	if !act["sshd.service"] || act["nginx.service"] || !act["crond.service"] {
		t.Errorf("rc-status: %v", act)
	}
	en := parseRCUpdate("             sshd |      default\n            nginx |\n          hwclock | boot\n")
	if !en["sshd.service"] || en["nginx.service"] || !en["hwclock.service"] {
		t.Errorf("rc-update: %v", en)
	}
	if u := cgroupUnit("0::/openrc.sshd\n"); u != "sshd.service" {
		t.Errorf("OpenRC cgroup: %q", u)
	}
}
