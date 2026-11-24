//go:build linux

package agent

import "testing"

func TestAddrOrigin(t *testing.T) {
	for _, c := range []struct {
		v6         bool
		prefix     int
		perm, conf bool
		want       string
	}{
		{false, 24, true, false, "static"},
		{false, 24, false, false, "dhcp"},
		{false, 24, true, true, "dhcp"}, // dhclient: no lifetime, but configured for DHCP
		{true, 64, true, false, "static"},
		{true, 64, false, false, "slaac"},
		{true, 128, false, false, "dhcp"},
	} {
		if got := addrOrigin(c.v6, c.prefix, c.perm, c.conf); got != c.want {
			t.Errorf("%+v: %s", c, got)
		}
	}
	m := map[string]bool{}
	parseIfupdown("auto vmbr0\niface vmbr0 inet static\n\taddress 192.168.5.2/24\niface eth1 inet dhcp\n", m)
	if m["vmbr0"] || !m["eth1"] {
		t.Errorf("ifupdown: %v", m)
	}
}
