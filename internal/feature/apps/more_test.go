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
	if len(rows) != 32 { // 31 applications, Tomcat twice
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
