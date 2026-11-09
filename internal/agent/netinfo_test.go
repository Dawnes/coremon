package agent

import (
	"reflect"
	"testing"
)

func TestDefaultGateway(t *testing.T) {
	route := `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
eth1	00000000	FE0010AC	0003	0	0	600	00000000	0	0	0
eth0	00000000	0105A8C0	0003	0	0	100	00000000	0	0	0
eth0	0005A8C0	00000000	0001	0	0	100	00FFFFFF	0	0	0
wg0	00000000	0100000A	0001	0	0	1	00000000	0	0	0
`
	gw, ifc := defaultGateway(route)
	if gw != "192.168.5.1" || ifc != "eth0" {
		t.Fatalf("gateway = %q on %q, want 192.168.5.1 on eth0 (lowest metric, with the gateway flag)", gw, ifc)
	}
	if gw, _ := defaultGateway("Iface\tDestination\n"); gw != "" {
		t.Fatalf("no default route: %q", gw)
	}
}

func TestDNSServers(t *testing.T) {
	files := map[string]string{
		"/etc/resolv.conf":                 "# managed\nnameserver 127.0.0.53\noptions edns0 trust-ad\nsearch lan\n",
		"/run/systemd/resolve/resolv.conf": "nameserver 192.168.5.1\nnameserver 2001:db8::1\nnameserver 1.1.1.1\n",
	}
	read := func(p string) string { return files[p] }
	s, via := dnsServers(read)
	if !reflect.DeepEqual(s, []string{"192.168.5.1", "1.1.1.1"}) || via != "systemd-resolved" {
		t.Fatalf("resolved: %v %q", s, via)
	}
	files["/etc/resolv.conf"] = "nameserver 9.9.9.9\nnameserver fe80::1\n"
	if s, via := dnsServers(read); !reflect.DeepEqual(s, []string{"9.9.9.9"}) || via != "" {
		t.Fatalf("plain resolv.conf: %v %q", s, via)
	}
	// Stub only and no resolved file: keep the stub rather than nothing.
	files["/etc/resolv.conf"] = "nameserver 127.0.0.53\n"
	delete(files, "/run/systemd/resolve/resolv.conf")
	if s, via := dnsServers(read); !reflect.DeepEqual(s, []string{"127.0.0.53"}) || via != "" {
		t.Fatalf("stub only: %v %q", s, via)
	}
}
