package apps

import (
	"strings"
	"testing"
)

func TestMoreApplications(t *testing.T) {
	status := `Package: exim4-daemon-light
Status: install ok installed
Source: exim4
Version: 4.96-15+deb12u7

Package: proftpd-core
Status: install ok installed
Source: proftpd-dfsg
Version: 1.3.8+dfsg-4+deb12u4

Package: tomcat10
Status: install ok installed
Version: 10.1.34-0+deb12u1

Package: tomcat9
Status: install ok installed
Version: 9.0.43-2~deb11u10

Package: mosquitto
Status: install ok installed
Version: 2.0.11-1.2+deb12u1

Package: grafana
Status: install ok installed
Version: 11.4.0

Package: tailscale
Status: install ok installed
Version: 1.78.1

Package: pve-qemu-kvm
Status: install ok installed
Version: 9.0.2-4

Package: qemu-system-x86
Status: install ok installed
Source: qemu
Version: 1:7.2+dfsg-7+deb12u12

Package: unbound
Status: install ok installed
Version: 1.17.1-2+deb12u2
`
	sys := &System{
		Active:  map[string]bool{"exim4.service": true, "tomcat10.service": true, "grafana-server.service": true, "tailscaled.service": true, "unbound.service": false},
		Enabled: map[string]bool{"exim4.service": true, "tailscaled.service": true},
		Ports:   map[string][]Port{"tailscaled.service": {{Port: 41641, Proto: "udp", Addr: "0.0.0.0"}}},
	}
	rows := scan(strings.NewReader(status), sys, nil)
	if len(rows) != 42 { // 41 applications, Tomcat twice
		t.Fatalf("%d rows", len(rows))
	}
	by := map[string]App{}
	for _, a := range rows {
		by[a.Name] = a
	}
	for name, want := range map[string]string{
		"Exim": "4.96", "ProFTPD": "1.3.8", "Tomcat 10": "10.1.34", "Tomcat 9": "9.0.43", "Mosquitto": "2.0.11",
		"Grafana": "11.4.0", "Tailscale": "1.78.1", "QEMU": "9.0.2", "Unbound": "1.17.1",
	} {
		if a := by[name]; !a.Installed || a.Version != want {
			t.Errorf("%s: %+v, want version %s", name, a, want)
		}
	}
	if by["Exim"].Source != "exim4" || by["Exim"].Running != "running" || by["Exim"].Boot != "enabled" {
		t.Errorf("Exim: %+v", by["Exim"])
	}
	if by["Tomcat 10"].Running != "running" || by["Tomcat 9"].Running != "" {
		t.Errorf("Tomcat service per version: %+v / %+v", by["Tomcat 10"], by["Tomcat 9"])
	}
	if p := by["Tailscale"].Ports; len(p) != 1 || p[0].Port != 41641 {
		t.Errorf("Tailscale ports: %+v", p)
	}
	if by["QEMU"].Running != "" || by["Unbound"].Running != "stopped" {
		t.Errorf("QEMU %q, Unbound %q", by["QEMU"].Running, by["Unbound"].Running)
	}
}

// Agent 0.3.14: HAProxy, Squid, Chrony, NTPsec, vsftpd, ClamAV, Dovecot,
// InfluxDB 1 and 2, Webmin.
func TestApplications0314(t *testing.T) {
	pkg := func(name, ver string) string {
		return "Package: " + name + "\nStatus: install ok installed\nVersion: " + ver + "\n\n"
	}
	status := pkg("haproxy", "2.6.12-1+deb12u2") + pkg("squid-openssl", "5.7-2+deb12u3") + pkg("chrony", "4.3-2+deb12u1") +
		pkg("ntpsec", "1.2.2+dfsg1-1+deb12u1") + pkg("vsftpd", "3.0.3-13+b2") + pkg("clamav", "1.0.7+dfsg-1~deb12u1") +
		pkg("clamav-daemon", "1.0.7+dfsg-1~deb12u1") + pkg("dovecot-core", "1:2.3.19.1+dfsg1-2.1+deb12u1") +
		pkg("influxdb2", "2.7.11-1") + pkg("webmin", "2.302") + pkg("dovecot-imapd", "1:2.3.19.1+dfsg1-2.1+deb12u1")
	sys := &System{
		Active:  map[string]bool{"haproxy.service": true, "clamav-daemon.service": false, "clamav-freshclam.service": true, "influxdb.service": true, "chrony.service": true},
		Enabled: map[string]bool{"haproxy.service": true, "clamav-freshclam.service": true},
	}
	by := map[string]App{}
	for _, a := range scan(strings.NewReader(status), sys, nil) {
		by[a.Name] = a
	}
	for name, want := range map[string]string{
		"HAProxy": "2.6.12", "Squid": "5.7", "Chrony": "4.3", "NTPsec": "1.2.2", "vsftpd": "3.0.3",
		"ClamAV": "1.0.7", "Dovecot": "2.3.19.1", "InfluxDB 2": "2.7.11", "Webmin": "2.302",
	} {
		if a := by[name]; !a.Installed || a.Version != want {
			t.Errorf("%s: %+v, want version %s", name, a, want)
		}
	}
	if by["InfluxDB 1"].Installed || by["InfluxDB 2"].Running != "running" {
		t.Errorf("InfluxDB: %+v / %+v", by["InfluxDB 1"], by["InfluxDB 2"])
	}
	if c := by["ClamAV"]; c.Running != "running" || c.Boot != "enabled" { // freshclam runs, the scanner does not
		t.Errorf("ClamAV: %+v", c)
	}
	if by["HAProxy"].Running != "running" || by["Dovecot"].Package != "dovecot-core" {
		t.Errorf("HAProxy %+v, Dovecot %+v", by["HAProxy"], by["Dovecot"])
	}
}
