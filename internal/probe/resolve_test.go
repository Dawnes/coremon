package probe

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestPickIP(t *testing.T) {
	v4, v6 := net.IPAddr{IP: net.ParseIP("192.0.2.7")}, net.IPAddr{IP: net.ParseIP("2001:db8::7")}
	yes, no := func() bool { return true }, func() bool { return false }
	cases := []struct {
		ips  []net.IPAddr
		v6   func() bool
		want string
		err  string
	}{
		{[]net.IPAddr{v6, v4}, yes, "192.0.2.7", ""}, // IPv4 first, also when listed later
		{[]net.IPAddr{v6}, yes, "2001:db8::7", ""},
		{[]net.IPAddr{v6}, no, "", "no IPv4 address"},
		{nil, yes, "", "no address"},
	}
	for i, c := range cases {
		ip, err := pickIP("x.example", c.ips, c.v6)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%d: error %v, want %q", i, err, c.err)
			}
			continue
		}
		if err != nil || ip.String() != c.want {
			t.Errorf("%d: %v %v, want %s", i, ip, err, c.want)
		}
	}
}

func TestResolveAndCheckLookup(t *testing.T) {
	old := lookupIP
	defer func() { lookupIP = old }()
	lookupIP = func(ctx context.Context, name string) ([]net.IPAddr, error) {
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	}
	if ip, err := Resolve(context.Background(), "192.0.2.1", time.Second); err != nil || ip.String() != "192.0.2.1" {
		t.Fatalf("an IP address is used as it is: %v %v", ip, err)
	}
	o := Check(context.Background(), "gone.example", Service{Port: 443}, time.Second)
	if o.Failed != StepLookup || o.Err == nil || !strings.Contains(o.Err.Error(), "not found in DNS") || !KnownStep(o.Failed) {
		t.Fatalf("lookup failure: %+v", o)
	}
	if !IsName("nas") || IsName("10.0.0.1") || IsName("") {
		t.Fatal("IsName")
	}
	if ValidAddress("nas.lan") != nil || ValidAddress("fd00::1") != nil || ValidAddress("bad name") == nil {
		t.Fatal("ValidAddress")
	}
}

func TestDefaultV6Route(t *testing.T) {
	z := strings.Repeat("0", 32)
	def := z + " 00 " + z + " 00 fe800000000000000000000000000001 00000400 00000001 00000000 00000003 eth0"
	lo := z + " 00 " + z + " 00 " + z + " ffffffff 00000001 00000000 00200200 lo"
	other := "20010db8000000000000000000000000 40 " + z + " 00 " + z + " 00000100 00000001 00000000 00000001 eth0"
	if !hasDefaultV6(other+"\n"+def) || hasDefaultV6(lo+"\n"+other) {
		t.Fatal("default route")
	}
}
