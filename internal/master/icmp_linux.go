package master

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

// icmpPing pings ip. It first uses a ping socket, which an ordinary user may
// open when net.ipv4.ping_group_range allows it (Debian's default); if that
// is refused it falls back to a raw socket, which needs CAP_NET_RAW.
func icmpPing(ctx context.Context, ip net.IP, timeout time.Duration) (time.Duration, error) {
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) < timeout {
		timeout = time.Until(dl)
	}
	v6 := ip.To4() == nil
	if c, err := pingSocket(v6); err == nil {
		defer c.Close()
		lastMethod.Store("ping socket")
		return exchange(c, &net.UDPAddr{IP: ip}, ip, v6, 0, false, timeout)
	} else if !denied(err) {
		return 0, &pingSetupError{fmt.Errorf("cannot open a ping socket: %w", err)}
	}
	network, laddr := "ip4:icmp", "0.0.0.0"
	if v6 {
		network, laddr = "ip6:ipv6-icmp", "::"
	}
	c, err := net.ListenPacket(network, laddr)
	if err != nil {
		return 0, &pingSetupError{errors.New("the master may not send pings: the system refuses ping sockets for its user " +
			"(sysctl net.ipv4.ping_group_range) and the service lacks CAP_NET_RAW. Run the master install command once as root on the master: it updates the service")}
	}
	defer c.Close()
	lastMethod.Store("raw socket")
	id := uint16(os.Getpid()) ^ uint16(pingSeq.Load()>>16)
	return exchange(c, &net.IPAddr{IP: ip}, ip, v6, id, true, timeout)
}

func denied(err error) bool {
	return errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EPROTONOSUPPORT)
}

func pingSocket(v6 bool) (net.PacketConn, error) {
	family, proto := syscall.AF_INET, syscall.IPPROTO_ICMP
	var sa syscall.Sockaddr = &syscall.SockaddrInet4{}
	if v6 {
		family, proto, sa = syscall.AF_INET6, syscall.IPPROTO_ICMPV6, &syscall.SockaddrInet6{}
	}
	fd, err := syscall.Socket(family, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, proto)
	if err != nil {
		return nil, err
	}
	if err := syscall.Bind(fd, sa); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "icmp")
	defer f.Close()
	return net.FilePacketConn(f)
}
