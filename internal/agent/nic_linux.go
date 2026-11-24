//go:build linux

package agent

import (
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"coremon/internal/proto"
)

// Linux address attributes and flags (linux/if_addr.h).
const (
	ifaAddress   = 1
	ifaLocal     = 2
	ifaCacheinfo = 6
	ifaFlags     = 8

	ifaFTemporary = 0x01 // IPv6 privacy address (for IPv4 the same bit means "secondary")
	ifaFPermanent = 0x80 // no lifetime: set by hand or by a client that gives none
)

// netAddrs: every global address of the device with how it was set. Read
// from the kernel (netlink): an address with a lifetime came from DHCP (or,
// for IPv6, from router advertisements); one without is static unless the
// interface's configuration asks for DHCP (dhclient adds addresses without
// a lifetime).
func netAddrs(read func(string) string) []proto.AddrInfo {
	rib, err := syscall.NetlinkRIB(syscall.RTM_GETADDR, syscall.AF_UNSPEC)
	if err != nil {
		return nil
	}
	msgs, err := syscall.ParseNetlinkMessage(rib)
	if err != nil {
		return nil
	}
	names := map[uint32]string{}
	if ifs, err := net.Interfaces(); err == nil {
		for _, i := range ifs {
			names[uint32(i.Index)] = i.Name
		}
	}
	dhcp := dhcpIfaces(read)
	var out []proto.AddrInfo
	for i := range msgs {
		m := &msgs[i]
		if m.Header.Type != syscall.RTM_NEWADDR || len(m.Data) < syscall.SizeofIfAddrmsg {
			continue
		}
		fam, plen, flags := m.Data[0], int(m.Data[1]), uint32(m.Data[2])
		index := binary.NativeEndian.Uint32(m.Data[4:8])
		attrs, err := syscall.ParseNetlinkRouteAttr(m)
		if err != nil {
			continue
		}
		var ip net.IP
		var local net.IP
		valid := uint32(0xffffffff)
		for _, a := range attrs {
			switch a.Attr.Type {
			case ifaAddress:
				ip = net.IP(append([]byte(nil), a.Value...))
			case ifaLocal:
				local = net.IP(append([]byte(nil), a.Value...))
			case ifaFlags:
				if len(a.Value) >= 4 {
					flags = binary.NativeEndian.Uint32(a.Value[:4])
				}
			case ifaCacheinfo:
				if len(a.Value) >= 8 {
					valid = binary.NativeEndian.Uint32(a.Value[4:8])
				}
			}
		}
		if fam == syscall.AF_INET && local != nil {
			ip = local
		}
		name := names[index]
		if ip == nil || skipIface(name) || !ip.IsGlobalUnicast() {
			continue
		}
		v6 := fam == syscall.AF_INET6
		if v6 && flags&ifaFTemporary != 0 {
			continue
		}
		out = append(out, proto.AddrInfo{IP: ip.String(), Prefix: plen, Iface: name,
			Origin: addrOrigin(v6, plen, flags&ifaFPermanent != 0 && valid == 0xffffffff, dhcp[name])})
		if len(out) >= 32 {
			break
		}
	}
	return out
}

// addrOrigin: how an address was set. permanent: it has no lifetime.
func addrOrigin(v6 bool, prefix int, permanent, dhcpConf bool) string {
	switch {
	case v6 && permanent:
		return "static"
	case v6 && prefix == 128:
		return "dhcp"
	case v6:
		return "slaac"
	case !permanent || dhcpConf:
		return "dhcp"
	}
	return "static"
}

func skipIface(name string) bool {
	if name == "" || name == "lo" {
		return true
	}
	for _, p := range []string{"veth", "tap", "fwbr", "fwpr", "fwln"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// dhcpIfaces: interfaces that ifupdown configures with DHCP
// ("iface eth0 inet dhcp" in /etc/network/interfaces or interfaces.d).
func dhcpIfaces(read func(string) string) map[string]bool {
	out := map[string]bool{}
	files := []string{"/etc/network/interfaces"}
	if m, err := filepath.Glob("/etc/network/interfaces.d/*"); err == nil {
		files = append(files, m...)
	}
	for _, f := range files {
		parseIfupdown(read(f), out)
	}
	return out
}

func parseIfupdown(conf string, out map[string]bool) {
	for _, line := range strings.Split(conf, "\n") {
		f := strings.Fields(line)
		if len(f) >= 4 && f[0] == "iface" && f[2] == "inet" && f[3] == "dhcp" {
			out[f[1]] = true
		}
	}
}

// mainNIC: the card the default route goes through, followed through
// bridges, bonds and VLANs to the physical card below it.
func mainNIC(iface, virt string, addrs []proto.AddrInfo) *proto.NIC {
	if iface == "" {
		// No IPv4 default route: the first interface with an address.
		for _, a := range addrs {
			iface = a.Iface
			break
		}
	}
	if iface == "" {
		return nil
	}
	n := &proto.NIC{Iface: iface}
	phys := physOf(iface, 0)
	card := iface
	if phys != "" {
		card = phys
		if phys != iface {
			n.Phys = phys
		}
	}
	n.MAC = macOf(card)
	n.Virtual = phys == "" || (virt != "none" && virt != "" && virt != "unknown")
	if n.MAC == "" {
		return nil
	}
	return n
}

const sysNet = "/sys/class/net/"

func physOf(name string, depth int) string {
	if depth > 4 || skipIface(name) {
		return ""
	}
	if _, err := os.Stat(sysNet + name + "/device"); err == nil {
		return name
	}
	var below []string
	if m, err := filepath.Glob(sysNet + name + "/lower_*"); err == nil {
		for _, p := range m {
			below = append(below, strings.TrimPrefix(filepath.Base(p), "lower_"))
		}
	}
	if ents, err := os.ReadDir(sysNet + name + "/brif"); err == nil {
		for _, e := range ents {
			below = append(below, e.Name())
		}
	}
	for _, b := range below {
		if p := physOf(b, depth+1); p != "" {
			return p
		}
	}
	return ""
}

func macOf(name string) string {
	// A bond gives its members its own MAC address: the card's own one is kept here.
	for _, p := range []string{sysNet + name + "/bonding_slave/perm_hwaddr", sysNet + name + "/address"} {
		if b, err := os.ReadFile(p); err == nil {
			if hw, err := net.ParseMAC(strings.TrimSpace(string(b))); err == nil && len(hw) == 6 && hw.String() != "00:00:00:00:00:00" {
				return hw.String()
			}
		}
	}
	return ""
}
