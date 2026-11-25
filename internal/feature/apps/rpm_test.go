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
	for _, a := range scanInfo(info, true, sys, func([]string) map[string]string { return origin }) {
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
	// Every application has its RPM names (or is Debian-only on purpose).
	for _, d := range defs {
		r, ok := rpmDefs[d.name]
		if !ok {
			t.Errorf("%s: no RPM definition", d.name)
		}
		if r.re == nil && d.name != "Proxmox VE" && d.name != "Proxmox Backup Server" {
			t.Errorf("%s: no RPM names", d.name)
		}
	}
	if len(rpmDefs) != len(defs) {
		t.Errorf("%d RPM definitions for %d applications", len(rpmDefs), len(defs))
	}
}
