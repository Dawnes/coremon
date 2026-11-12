package apps

import (
	"encoding/hex"
	"net"
	"strconv"
	"strings"
)

// Parsers for service and socket information. They take command output and
// file contents, so they can be tested on any platform; collecting is in
// sysinfo_linux.go.

// parseListUnits reads `systemctl list-units --all --plain --no-legend`:
// unit -> active.
func parseListUnits(out string, into map[string]bool) {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		into[f[0]] = f[2] == "active"
	}
}

// parseUnitFiles reads `systemctl list-unit-files --plain --no-legend`; units
// that exist but are not loaded are added as inactive. Templates
// ("foo@.service") are skipped: only their instances are real services.
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

// bootStates are is-enabled answers that mean "starts at boot".
var bootStates = map[string]bool{"enabled": true, "enabled-runtime": true, "alias": true, "indirect": true, "generated": true}

// parseIsEnabled reads `systemctl is-enabled u1 u2 ...` (one line per unit,
// in order).
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

// sock is a listening socket from /proc/net/{tcp,udp}{,6}.
type sock struct {
	inode string
	port  int
	proto string
	ip    net.IP
}

// procIP decodes the kernel's hex address (32-bit words in host order,
// little endian on every platform Coremon is built for).
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

// parseNetProc reads one of /proc/net/{tcp,tcp6,udp,udp6}: TCP sockets in
// LISTEN, UDP sockets that are not connected.
func parseNetProc(content, proto string) []sock {
	want := "0A" // TCP_LISTEN
	if proto == "udp" {
		want = "07" // unconnected
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
			continue // connected UDP socket: not a listening service
		}
		out = append(out, sock{inode: f[9], port: int(port), proto: proto, ip: ip})
	}
	return out
}

// toPort turns a socket into a Port; loopback addresses are local only.
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

// cgroupUnit returns the systemd service a process belongs to, from
// /proc/<pid>/cgroup ("0::/system.slice/nginx.service" -> nginx.service).
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

// parseListen reads `systemctl show -p Id -p Listen a.socket b.socket`:
// socket-activated services (the system holds the port, e.g. ssh.socket).
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
			// Listen=[::]:22 (Stream) | 0.0.0.0:53 (Datagram)
			v := strings.TrimPrefix(line, "Listen=")
			addrPart, kind, _ := strings.Cut(v, " ")
			proto := "tcp"
			if strings.Contains(kind, "Datagram") {
				proto = "udp"
			} else if !strings.Contains(kind, "Stream") {
				continue // a file or other kind of socket
			}
			host, portStr, err := net.SplitHostPort(addrPart)
			if err != nil {
				portStr, host = addrPart, "" // just a port: all addresses
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
