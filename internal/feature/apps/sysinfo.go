package apps

// Parsers for service, socket and port information.

import (
	"encoding/hex"
	"net"
	"strconv"
	"strings"
)

// Parsers for service and socket information. They take command output and
// file contents, so they can be tested on any platform; collecting is in
// sysinfo_linux.go.

func parseListUnits(out string, into map[string]bool) {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		into[f[0]] = f[2] == "active"
	}
}

func parseUnitFiles(out string, into map[string]bool) {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || strings.Contains(f[0], "@.") {
			continue
		}
		if _, ok := into[f[0]]; !ok {
			into[f[0]] = false
		}
	}
}

var bootStates = map[string]bool{"enabled": true, "enabled-runtime": true, "alias": true, "indirect": true, "generated": true}

func parseIsEnabled(units []string, out string) map[string]bool {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	m := map[string]bool{}
	for i, u := range units {
		if i < len(lines) {
			m[u] = bootStates[strings.TrimSpace(lines[i])]
		}
	}
	return m
}

type sock struct {
	inode string
	port  int
	proto string
	ip    net.IP
}

func procIP(h string) net.IP {
	b, err := hex.DecodeString(h)
	if err != nil || (len(b) != 4 && len(b) != 16) {
		return nil
	}
	for i := 0; i < len(b); i += 4 {
		b[i], b[i+1], b[i+2], b[i+3] = b[i+3], b[i+2], b[i+1], b[i]
	}
	return net.IP(b)
}

func parseNetProc(content, proto string) []sock {
	want := "0A"
	if proto == "udp" {
		want = "07"
	}
	var out []sock
	for i, line := range strings.Split(content, "\n") {
		f := strings.Fields(line)
		if i == 0 || len(f) < 10 || f[3] != want {
			continue
		}
		local := strings.SplitN(f[1], ":", 2)
		if len(local) != 2 {
			continue
		}
		port, err := strconv.ParseUint(local[1], 16, 16)
		ip := procIP(local[0])
		if err != nil || port == 0 || ip == nil || f[9] == "0" {
			continue
		}
		if proto == "udp" && !strings.HasSuffix(f[2], ":0000") {
			continue
		}
		out = append(out, sock{inode: f[9], port: int(port), proto: proto, ip: ip})
	}
	return out
}

func (s sock) toPort() Port {
	addr := s.ip.String()
	if s.ip.IsUnspecified() {
		if s.ip.To4() != nil {
			addr = "0.0.0.0"
		} else {
			addr = "::"
		}
	}
	return Port{Port: s.port, Proto: s.proto, Local: s.ip.IsLoopback(), Addr: addr}
}

func cgroupUnit(content string) string {
	for _, line := range strings.Split(content, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
		if len(parts) != 3 {
			continue
		}
		elems := strings.Split(parts[2], "/")
		for i := len(elems) - 1; i >= 0; i-- {
			if strings.HasSuffix(elems[i], ".service") {
				return elems[i]
			}
		}
	}
	return ""
}

func parseListen(out string) map[string][]Port {
	m := map[string][]Port{}
	id := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			id = ""
		case strings.HasPrefix(line, "Id="):
			id = line[3:]
		case strings.HasPrefix(line, "Listen=") && id != "":
			v := strings.TrimPrefix(line, "Listen=")
			addrPart, kind, _ := strings.Cut(v, " ")
			proto := "tcp"
			if strings.Contains(kind, "Datagram") {
				proto = "udp"
			} else if !strings.Contains(kind, "Stream") {
				continue
			}
			host, portStr, err := net.SplitHostPort(addrPart)
			if err != nil {
				portStr, host = addrPart, ""
			}
			p, err := strconv.Atoi(portStr)
			if err != nil || p <= 0 || p > 65535 {
				continue
			}
			ip := net.ParseIP(host)
			if host == "" || ip == nil {
				ip = net.IPv6unspecified
			}
			m[id] = append(m[id], sock{port: p, proto: proto, ip: ip}.toPort())
		}
	}
	return m
}
