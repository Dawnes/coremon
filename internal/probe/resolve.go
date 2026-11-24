package probe

// Devices known by a name instead of an IP address: the system that checks
// looks the name up before every check, with its own (the OS) DNS setup.
// IPv4 comes first; an IPv6 address is used only when the name has no IPv4
// address and this system has IPv6 connectivity.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// StepLookup: the device's name was looked up in DNS.
const StepLookup = "lookup"

// IsName: addr is a name to look up, not an IP address.
func IsName(addr string) bool { return addr != "" && net.ParseIP(addr) == nil }

// lookupIP is the resolver (replaced in tests).
var lookupIP = func(ctx context.Context, name string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, name)
}

// Resolve: the address to connect to. An IP address is returned as it is;
// a name is looked up (within timeout).
func Resolve(ctx context.Context, addr string, timeout time.Duration) (net.IP, error) {
	if ip := net.ParseIP(addr); ip != nil {
		return ip, nil
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ips, err := lookupIP(ctx, addr)
	if err != nil {
		return nil, lookupError(addr, err, timeout)
	}
	return pickIP(addr, ips, HasIPv6)
}

// pickIP: the first IPv4 address, else the first IPv6 address when v6()
// says this system has IPv6 connectivity.
func pickIP(name string, ips []net.IPAddr, v6 func() bool) (net.IP, error) {
	var six net.IP
	for _, a := range ips {
		if v4 := a.IP.To4(); v4 != nil {
			return v4, nil
		}
		if six == nil {
			six = a.IP
		}
	}
	switch {
	case six == nil:
		return nil, fmt.Errorf("%s has no address in DNS", name)
	case !v6():
		return nil, fmt.Errorf("%s has no IPv4 address (only IPv6, and this system has no IPv6 connectivity)", name)
	}
	return six, nil
}

func lookupError(name string, err error, timeout time.Duration) error {
	var de *net.DNSError
	if errors.As(err, &de) {
		switch {
		case de.IsNotFound:
			return fmt.Errorf("%s is not found in DNS", name)
		case de.IsTimeout:
			return fmt.Errorf("no DNS answer for %s within %s", name, timeout)
		}
		if de.Err != "" {
			return fmt.Errorf("looking up %s: %s", name, de.Err)
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("no DNS answer for %s within %s", name, timeout)
	}
	return fmt.Errorf("looking up %s: %v", name, UnwrapOp(err))
}

// HasIPv6: this system has IPv6 connectivity: a global IPv6 address on an
// interface that is up, and an IPv6 default route (where the system shows
// its routes). Looked at once a minute.
func HasIPv6() bool {
	v6c.mu.Lock()
	defer v6c.mu.Unlock()
	if time.Since(v6c.at) > time.Minute {
		v6c.ok, v6c.at = globalV6() && v6DefaultRoute(), time.Now()
	}
	return v6c.ok
}

var v6c struct {
	mu sync.Mutex
	ok bool
	at time.Time
}

func globalV6() bool {
	ifs, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() == nil && n.IP.IsGlobalUnicast() {
				return true
			}
		}
	}
	return false
}

// hasDefaultV6: a default route in /proc/net/ipv6_route text: destination
// ::/0 on an interface other than lo.
func hasDefaultV6(routes string) bool {
	for _, l := range strings.Split(routes, "\n") {
		f := strings.Fields(l)
		if len(f) >= 10 && f[0] == strings.Repeat("0", 32) && f[1] == "00" && f[9] != "lo" {
			return true
		}
	}
	return false
}
