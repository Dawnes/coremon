package agent

import (
	"encoding/binary"
	"encoding/hex"
	"net"
	"strconv"
	"strings"
)

// Network facts: parsed from /proc/net/route and resolv.conf files. The
// parsers take file contents so they can be tested on any platform.

// defaultGateway returns the IPv4 default route with the lowest metric
// from the contents of /proc/net/route.
func defaultGateway(route string) (gw, iface string) {
	best := -1
	for i, line := range strings.Split(route, "\n") {
		f := strings.Fields(line)
		if i == 0 || len(f) < 8 {
			continue // header or malformed
		}
		// Iface Destination Gateway Flags RefCnt Use Metric Mask ...
		if f[1] != "00000000" || f[7] != "00000000" {
			continue // not a default route
		}
		flags, err := strconv.ParseUint(f[3], 16, 32)
		if err != nil || flags&0x1 == 0 || flags&0x2 == 0 {
			continue // not up, or no gateway
		}
		metric, err := strconv.Atoi(f[6])
		if err != nil {
			continue
		}
		b, err := hex.DecodeString(f[2])
		if err != nil || len(b) != 4 {
			continue
		}
		// The kernel prints the address in host byte order (little endian
		// on every platform coremon is built for).
		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, binary.LittleEndian.Uint32(b))
		if best == -1 || metric < best {
			best, gw, iface = metric, ip.String(), f[0]
		}
	}
	return gw, iface
}

const maxDNS = 8

// nameservers returns the IPv4 name servers in resolv.conf contents.
func nameservers(conf string) []string {
	var out []string
	for _, line := range strings.Split(conf, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[0] != "nameserver" {
			continue
		}
		ip := net.ParseIP(f[1])
		if ip == nil || ip.To4() == nil {
			continue // IPv4 only
		}
		out = append(out, ip.String())
		if len(out) == maxDNS {
			break
		}
	}
	return out
}

// onlyResolvedStub reports whether the servers are just systemd-resolved's
// local forwarder (127.0.0.53, or 127.0.0.54 in proxy mode).
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

// dnsServers picks the DNS servers to report: those in /etc/resolv.conf, or
// systemd-resolved's real servers when that file only names its forwarder.
// readFile returns "" for a missing file.
func dnsServers(readFile func(string) string) (servers []string, via string) {
	servers = nameservers(readFile("/etc/resolv.conf"))
	if onlyResolvedStub(servers) {
		if real := nameservers(readFile("/run/systemd/resolve/resolv.conf")); len(real) > 0 {
			return real, "systemd-resolved"
		}
	}
	return servers, ""
}
