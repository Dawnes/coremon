package apps

import (
	"reflect"
	"strings"
	"testing"
)

func TestSystemParsers(t *testing.T) {
	active := map[string]bool{}
	parseListUnits("nginx.service loaded active running A high performance web server\nphp8.2-fpm.service loaded inactive dead PHP\npostgresql@15-main.service loaded active running PostgreSQL\n", active)
	parseUnitFiles("nginx.service enabled enabled\nredis-server.service disabled enabled\npostgresql@.service indirect enabled\n", active)
	if !active["nginx.service"] || active["php8.2-fpm.service"] || active["redis-server.service"] {
		t.Fatalf("active: %v", active)
	}
	if _, ok := active["postgresql@.service"]; ok {
		t.Fatal("a template was taken as a service")
	}
	en := parseIsEnabled([]string{"a.service", "b.service", "c.socket"}, "enabled\ndisabled\nindirect\n")
	if !en["a.service"] || en["b.service"] || !en["c.socket"] {
		t.Fatalf("is-enabled: %v", en)
	}

	tcp := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1111 1 0 100 0 0 10 0
   1: 0100007F:0CEA 00000000:0000 0A 00000000:00000000 00:00000000 00000000   112        0 2222 1 0 100 0 0 10 0
   2: 2905A8C0:0016 3205A8C0:C350 01 00000000:00000000 02:000A0A0A 00000000     0        0 3333 4 0 20 4 30 10 -1
`
	s := parseNetProc(tcp, "tcp")
	if len(s) != 2 || s[0].port != 22 || s[0].toPort().Addr != "0.0.0.0" || s[0].toPort().Local ||
		s[1].port != 3306 || !s[1].toPort().Local || s[1].toPort().Addr != "127.0.0.1" {
		t.Fatalf("tcp: %+v", s)
	}
	tcp6 := "  sl  local_address                         remote_address                        st\n   0: 00000000000000000000000000000000:0050 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 4444 1\n   1: 00000000000000000000000001000000:1F90 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 5555 1\n"
	s6 := parseNetProc(tcp6, "tcp")
	if len(s6) != 2 || s6[0].toPort().Addr != "::" || s6[1].toPort().Addr != "::1" || !s6[1].toPort().Local {
		t.Fatalf("tcp6: %+v", s6)
	}
	udp := "  sl  local_address rem_address   st\n 1: 00000000:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000 101 0 6666 2\n 2: 2905A8C0:A1B2 08080808:0035 01 00000000:00000000 00:00000000 00000000 0 0 7777 2\n"
	su := parseNetProc(udp, "udp")
	if len(su) != 1 || su[0].port != 53 || su[0].proto != "udp" {
		t.Fatalf("udp: %+v", su)
	}

	if u := cgroupUnit("0::/system.slice/system-postgresql.slice/postgresql@15-main.service\n"); u != "postgresql@15-main.service" {
		t.Fatalf("cgroup: %q", u)
	}
	if u := cgroupUnit("0::/user.slice/user-1000.slice/session-3.scope\n"); u != "" {
		t.Fatalf("cgroup user: %q", u)
	}
	l := parseListen("Id=ssh.socket\nListen=[::]:22 (Stream)\n\nId=dns.socket\nListen=127.0.0.1:53 (Datagram)\nListen=/run/x.sock (Stream)\n")
	if !reflect.DeepEqual(l["ssh.socket"], []Port{{Port: 22, Proto: "tcp", Addr: "::"}}) ||
		!reflect.DeepEqual(l["dns.socket"], []Port{{Port: 53, Proto: "udp", Local: true, Addr: "127.0.0.1"}}) {
		t.Fatalf("listen: %+v", l)
	}
}

func TestServiceColumns(t *testing.T) {
	status := "Package: nginx\nStatus: install ok installed\nVersion: 1.26.3-3\n\n" +
		"Package: php8.2-cli\nStatus: install ok installed\nVersion: 8.2.26-1\n\n" +
		"Package: postgresql-15\nStatus: install ok installed\nVersion: 15.10-0+deb12u1\n\n" +
		"Package: openssh-server\nStatus: install ok installed\nVersion: 1:9.2p1-2\n\n" +
		"Package: sudo\nStatus: install ok installed\nVersion: 1.9.13p3-1\n"
	sys := &System{
		Active:  map[string]bool{"nginx.service": true, "postgresql@15-main.service": false, "ssh.service": false, "ssh.socket": true},
		Enabled: map[string]bool{"nginx.service": false, "postgresql@15-main.service": true, "ssh.socket": true},
		Ports: map[string][]Port{
			"nginx.service": {{Port: 443, Proto: "tcp", Addr: "0.0.0.0"}, {Port: 80, Proto: "tcp", Addr: "0.0.0.0"}, {Port: 80, Proto: "tcp", Addr: "::"}},
			"ssh.socket":    {{Port: 22, Proto: "tcp", Addr: "::"}},
		},
	}
	rows := map[string]App{}
	for _, a := range scan(strings.NewReader(status), sys, nil) {
		rows[a.Name] = a
	}
	n := rows["Nginx"]
	if n.Running != "running" || n.Boot != "disabled" || len(n.Ports) != 2 || n.Ports[0].Port != 80 || n.Ports[0].Addr != "0.0.0.0, ::" || n.Ports[1].Port != 443 {
		t.Fatalf("nginx (started by hand, ports merged and sorted): %+v", n)
	}
	if p := rows["PostgreSQL 15"]; p.Running != "stopped" || p.Boot != "enabled" {
		t.Fatalf("postgresql instance: %+v", p)
	}
	if s := rows["SSH server"]; s.Running != "running" || s.Boot != "enabled" || len(s.Ports) != 1 || s.Ports[0].Port != 22 {
		t.Fatalf("ssh via socket activation: %+v", s)
	}
	if p := rows["PHP 8.2"]; p.Running != "" || p.Ports != nil {
		t.Fatalf("PHP without FPM has no service of its own: %+v", p)
	}
	if s := rows["sudo"]; s.Running != "" || !s.Installed {
		t.Fatalf("sudo: %+v", s)
	}
}
