package agent

import (
	"net"
	"os"
	"runtime"

	"coremon/internal/proto"
)

func baseFacts() proto.Facts {
	f := proto.Facts{OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU()}
	f.Hostname, _ = os.Hostname()
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 || ifc.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLinkLocalUnicast() {
				f.IPs = append(f.IPs, ipn.IP.String())
			}
		}
	}
	return f
}
