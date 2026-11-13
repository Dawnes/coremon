package agent

// Network facts: default route and DNS servers.

import (
	"encoding/binary"
	"encoding/hex"
	"net"
	"strconv"
	"strings"
)

// Network facts: parsed from /proc/net/route and resolv.conf files. The
// parsers take file contents so they can be tested on any platform.

func defaultGateway(route string) (gw, iface string) {
	best := -1
	for i, line := range strings.Split(route, "\n") {
		f := strings.Fields(line)
		if i == 0 || len(f) < 8 {
			continue
		}

		if f[1] != "00000000" || f[7] != "00000000" {
			continue
		}
		flags, err := strconv.ParseUint(f[3], 16, 32)
		if err != nil || flags&0x1 == 0 || flags&0x2 == 0 {
			continue
		}
		metric, err := strconv.Atoi(f[6])
		if err != nil {
			continue
		}
		b, err := hex.DecodeString(f[2])
		if err != nil || len(b) != 4 {
			continue
		}

		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, binary.LittleEndian.Uint32(b))
		if best == -1 || metric < best {
			best, gw, iface = metric, ip.String(), f[0]
		}
	}
	return gw, iface
}

const maxDNS = 8

func nameservers(conf string) []string {
	var out []string
	for _, line := range strings.Split(conf, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[0] != "nameserver" {
			continue
		}
		ip := net.ParseIP(f[1])
		if ip == nil || ip.To4() == nil {
			continue
		}
		out = append(out, ip.String())
		if len(out) == maxDNS {
			break
		}
	}
	return out
}

func onlyResolvedStub(servers []string) bool {
	if len(servers) == 0 {
		return false
	}
	for _, s := range servers {
		if s != "127.0.0.53" && s != "127.0.0.54" {
			return false
		}
	}
	return true
}

func dnsServers(readFile func(string) string) (servers []string, via string) {
	servers = nameservers(readFile("/etc/resolv.conf"))
	if onlyResolvedStub(servers) {
		if real := nameservers(readFile("/run/systemd/resolve/resolv.conf")); len(real) > 0 {
			return real, "systemd-resolved"
		}
	}
	return servers, ""
}
