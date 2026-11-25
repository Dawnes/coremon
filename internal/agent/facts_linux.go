//go:build linux

package agent

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"coremon/internal/feature/apps"
	"coremon/internal/proto"
)

func collectFacts() proto.Facts {
	f := baseFacts()
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		f.Kernel = strings.TrimSpace(string(b))
	}
	if osr, err := os.Open("/etc/os-release"); err == nil {
		sc := bufio.NewScanner(osr)
		for sc.Scan() {
			k, v, ok := strings.Cut(sc.Text(), "=")
			if !ok {
				continue
			}
			v = strings.Trim(v, `"'`)
			switch k {
			case "PRETTY_NAME":
				f.OSName = v
			case "ID":
				f.OSID = v
			case "VERSION_ID":
				f.OSVersion = v
			}
		}
		osr.Close()
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		if fs := strings.Fields(string(b)); len(fs) > 0 {
			v, _ := strconv.ParseFloat(fs[0], 64)
			f.UptimeSec = int64(v)
		}
	}

	if m, err := os.Open("/proc/meminfo"); err == nil {
		sc := bufio.NewScanner(m)
		for sc.Scan() {
			fs := strings.Fields(sc.Text())
			if len(fs) >= 2 && fs[0] == "MemTotal:" {
				v, _ := strconv.ParseUint(fs[1], 10, 64)
				f.MemTotal = v * 1024
				break
			}
		}
		m.Close()
	}
	f.Virt = detectVirt()
	read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }
	f.Gateway, f.GatewayIf = defaultGateway(read("/proc/net/route"))
	f.DNS, f.DNSVia = dnsServers(read)
	f.Addrs = netAddrs(read)
	f.NIC = mainNIC(f.GatewayIf, f.Virt, f.Addrs)
	f.Platform = apps.Platform()
	f.Health = currentHealth()
	return f
}

var virtCache string

func detectVirt() string {
	if virtCache != "" {
		return virtCache
	}
	v := "unknown"
	if out, err := exec.Command("systemd-detect-virt").Output(); err == nil || len(out) > 0 {
		if s := strings.TrimSpace(string(out)); s != "" {
			v = s
		}
	} else { // no systemd (Alpine): the usual signs
		read := func(p string) string { b, _ := os.ReadFile(p); return string(b) }
		v = guessVirt(read("/proc/1/environ"), read("/sys/class/dmi/id/sys_vendor"), read("/sys/class/dmi/id/product_name"), read("/proc/cpuinfo"))
	}
	virtCache = v
	return v
}

// guessVirt: what systemd-detect-virt would say, from PID 1's environment
// (containers), the DMI vendor and the CPU's hypervisor flag.
func guessVirt(environ, vendor, product, cpuinfo string) string {
	for _, kv := range strings.Split(environ, "\x00") {
		if c, ok := strings.CutPrefix(kv, "container="); ok && strings.TrimSpace(c) != "" {
			return strings.TrimSpace(c)
		}
	}
	dmi := strings.ToLower(vendor + " " + product)
	switch {
	case strings.Contains(dmi, "qemu"), strings.Contains(dmi, "kvm"):
		return "kvm"
	case strings.Contains(dmi, "vmware"):
		return "vmware"
	case strings.Contains(dmi, "virtualbox"), strings.Contains(dmi, "innotek"):
		return "oracle"
	case strings.Contains(dmi, "microsoft"):
		return "microsoft"
	case strings.Contains(dmi, "xen"):
		return "xen"
	}
	if strings.Contains(cpuinfo, " hypervisor") {
		return "vm-other"
	}
	if vendor != "" {
		return "none"
	}
	return "unknown"
}
