package apps

import (
	"strings"
	"testing"
)

func TestDHCPServers(t *testing.T) {
	status := `Package: isc-dhcp-server
Status: install ok installed
Source: isc-dhcp
Version: 4.4.3-P1-2

Package: isc-kea-dhcp4-server
Status: install ok installed
Version: 2.6.1-isc20240725
`
	sys := &System{
		Active:  map[string]bool{"isc-dhcp-server.service": true, "isc-dhcp-server6.service": false, "isc-kea-dhcp4-server.service": true},
		Enabled: map[string]bool{"isc-dhcp-server.service": true},
		Ports: map[string][]Port{
			"isc-dhcp-server.service":      {{Port: 67, Proto: "udp", Addr: "0.0.0.0"}},
			"isc-kea-dhcp4-server.service": {{Port: 67, Proto: "udp", Addr: "192.168.1.1"}},
		},
	}
	by := map[string]App{}
	for _, a := range scan(strings.NewReader(status), sys, nil) {
		by[a.Name] = a
	}
	isc, kea := by["ISC DHCP server"], by["Kea DHCP"]
	if !isc.Installed || isc.Source != "isc-dhcp" || isc.Running != "running" || isc.Boot != "enabled" || len(isc.Ports) != 1 || isc.Ports[0].Port != 67 {
		t.Errorf("ISC DHCP: %+v", isc)
	}
	if !kea.Installed || kea.Package != "isc-kea-dhcp4-server" || kea.Version != "2.6.1" || kea.Running != "running" || kea.Boot != "disabled" {
		t.Errorf("Kea: %+v", kea)
	}
	for _, p := range []string{"kea-dhcp4-server", "kea-dhcp6-server", "isc-kea-dhcp6-server", "isc-kea-dhcp4"} {
		if !defs[keaIndex()].re.MatchString(p) {
			t.Errorf("Kea package %s not recognised", p)
		}
	}
	if defs[keaIndex()].re.MatchString("kea-common") || defs[keaIndex()].re.MatchString("isc-dhcp-client") {
		t.Error("Kea pattern too wide")
	}
}

func keaIndex() int {
	for i, d := range defs {
		if d.name == "Kea DHCP" {
			return i
		}
	}
	return -1
}
