package probe

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeDNS answers on one port over UDP and TCP, by the asked name.
func fakeDNS(t *testing.T) int {
	t.Helper()
	var udp net.PacketConn
	var tcp net.Listener
	for i := 0; i < 20; i++ {
		u, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := u.LocalAddr().(*net.UDPAddr).Port
		if l, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port))); err == nil {
			udp, tcp = u, l
			break
		}
		u.Close()
	}
	if udp == nil {
		t.Fatal("no free port for UDP and TCP")
	}
	t.Cleanup(func() { udp.Close(); tcp.Close() })
	go func() {
		buf := make([]byte, 4096)
		for {
			n, from, err := udp.ReadFrom(buf)
			if err != nil {
				return
			}
			q := append([]byte(nil), buf[:n]...)
			go func() {
				if a := fakeAnswer(q, false); a != nil {
					udp.WriteTo(a, from)
				}
			}()
		}
	}()
	go func() {
		for {
			c, err := tcp.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				var h [2]byte
				if _, err := io.ReadFull(c, h[:]); err != nil {
					return
				}
				q := make([]byte, binary.BigEndian.Uint16(h[:]))
				if _, err := io.ReadFull(c, q); err != nil {
					return
				}
				a := fakeAnswer(q, true)
				c.Write(append(binary.BigEndian.AppendUint16(nil, uint16(len(a))), a...))
			}()
		}
	}()
	return tcp.Addr().(*net.TCPAddr).Port
}

func encName(n string) []byte {
	var b []byte
	for _, l := range strings.Split(n, ".") {
		b = append(append(b, byte(len(l))), l...)
	}
	return append(b, 0)
}

func fakeAnswer(q []byte, overTCP bool) []byte {
	name, qend, err := readName(q, 12)
	if err != nil {
		return nil
	}
	type rr struct {
		typ  uint16
		data []byte
	}
	var rrs []rr
	rcode, tc := 0, false
	switch strings.ToLower(name) {
	case "a.test":
		rrs = []rr{{1, []byte{192, 0, 2, 10}}, {1, []byte{192, 0, 2, 11}}}
	case "mx.test":
		rrs = []rr{{15, append([]byte{0, 10}, encName("mail.a.test")...)}}
	case "txt.test":
		rrs = []rr{{16, append(append([]byte{7}, "v=spf1 "...), append([]byte{4}, "-all"...)...)}}
	case "nx.test":
		rcode = 3
	case "cname.test":
		rrs = []rr{{5, encName("a.test")}}
	case "big.test":
		if !overTCP {
			tc = true
		} else {
			rrs = []rr{{1, []byte{192, 0, 2, 20}}}
		}
	case "slow.test":
		time.Sleep(60 * time.Millisecond)
		rrs = []rr{{1, []byte{192, 0, 2, 30}}}
	}
	h := make([]byte, 12)
	copy(h, q[:2])
	h[2], h[3] = 0x81, 0x80|byte(rcode)
	if tc {
		h[2] |= 0x02
	}
	binary.BigEndian.PutUint16(h[4:], 1)
	binary.BigEndian.PutUint16(h[6:], uint16(len(rrs)))
	m := append(h, q[12:qend+4]...)
	for _, r := range rrs {
		m = append(m, 0xc0, 0x0c)
		m = binary.BigEndian.AppendUint16(m, r.typ)
		m = append(m, 0, 1, 0, 0, 1, 0)
		m = binary.BigEndian.AppendUint16(m, uint16(len(r.data)))
		m = append(m, r.data...)
	}
	return m
}

func TestCheckDNS(t *testing.T) {
	port := fakeDNS(t)
	run := func(name, typ, expect string, maxMs int) Outcome {
		s := Service{Method: MethodDNS, Port: port, Host: name, RType: typ, Expect: expect, MaxMs: maxMs}
		if err := Validate(s); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return Check(context.Background(), "127.0.0.1", s, time.Second)
	}
	for _, c := range []struct {
		name, typ, expect string
		maxMs             int
		failed, detail    string
	}{
		{"a.test", "A", "", 3000, "", "A 192.0.2.10, 192.0.2.11"},
		{"A.Test.", "A", "0.2.11", 0, "", "A 192.0.2.10, 192.0.2.11"},
		{"a.test", "A", "10.0.0.1", 0, StepExpect, "A 192.0.2.10, 192.0.2.11"},
		{"mx.test", "MX", "MAIL.a.test", 0, "", "MX 10 mail.a.test"},
		{"txt.test", "TXT", "spf1 -all", 0, "", "TXT v=spf1 -all"},
		{"nx.test", "A", "", 0, StepRcode, ""},
		{"empty.test", "MX", "", 0, StepRecords, ""},
		{"cname.test", "A", "", 0, StepRecords, ""},
		{"cname.test", "CNAME", "a.test", 0, "", "CNAME a.test"},
		{"big.test", "A", "", 0, "", "A 192.0.2.20 (TCP)"},
		{"slow.test", "A", "", 20, StepTime, "A 192.0.2.30"},
	} {
		o := run(c.name, c.typ, c.expect, c.maxMs)
		if o.Failed != c.failed || (c.detail != "" && o.Detail != c.detail) || (c.failed == "") != (o.Err == nil) {
			t.Errorf("%s %s %q: failed %q detail %q err %v", c.name, c.typ, c.expect, o.Failed, o.Detail, o.Err)
		}
	}
	if o := run("empty.test", "MX", "", 0); !strings.Contains(o.Err.Error(), "no MX record") {
		t.Errorf("empty: %v", o.Err)
	}
	if o := run("nx.test", "A", "", 0); !strings.Contains(o.Err.Error(), "NXDOMAIN") {
		t.Errorf("nx: %v", o.Err)
	}
	// Nothing listening: the query step fails.
	l, _ := net.ListenPacket("udp", "127.0.0.1:0")
	dead := l.LocalAddr().(*net.UDPAddr).Port
	l.Close()
	o := Check(context.Background(), "127.0.0.1", Service{Method: MethodDNS, Port: dead, Host: "a.test", RType: "A"}, 300*time.Millisecond)
	if o.Failed != StepQuery || o.Err == nil {
		t.Errorf("no server: %+v", o)
	}
}

func TestValidDNS(t *testing.T) {
	ok := Service{Method: MethodDNS, Port: 53, Host: "_dmarc.example.com", RType: "TXT", MaxMs: 3000}
	if err := Validate(ok); err != nil {
		t.Errorf("valid: %v", err)
	}
	for _, s := range []Service{
		{Method: MethodDNS, Port: 53, Host: "", RType: "A"},
		{Method: MethodDNS, Port: 53, Host: "bad name.com", RType: "A"},
		{Method: MethodDNS, Port: 53, Host: "-x.com", RType: "A"},
		{Method: MethodDNS, Port: 53, Host: "example.com", RType: "SOA"},
		{Method: MethodDNS, Port: 53, Host: "example.com", RType: "A", MaxMs: 60001},
		{Method: MethodDNS, Port: 53, Host: "example.com", RType: "A", Expect: "a\nb"},
		{Method: MethodDNS, Port: 53, Host: "example.com", RType: "A", Greeting: true},
	} {
		if Validate(s) == nil {
			t.Errorf("accepted: %+v", s)
		}
	}
	if st := Steps(Service{Method: MethodDNS, Expect: "x", MaxMs: 5}); strings.Join(st, ",") != "query,rcode,records,expect,time" {
		t.Errorf("steps: %v", st)
	}
}
