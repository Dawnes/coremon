package probe

import "os"

func v6DefaultRoute() bool {
	b, err := os.ReadFile("/proc/net/ipv6_route")
	return err == nil && hasDefaultV6(string(b))
}
