//go:build linux

package agent

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

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
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if fs := strings.Fields(string(b)); len(fs) > 0 {
			f.Load1, _ = strconv.ParseFloat(fs[0], 64)
		}
	}
	if m, err := os.Open("/proc/meminfo"); err == nil {
		sc := bufio.NewScanner(m)
		for sc.Scan() {
			fs := strings.Fields(sc.Text())
			if len(fs) < 2 {
				continue
			}
			v, _ := strconv.ParseUint(fs[1], 10, 64)
			switch fs[0] {
			case "MemTotal:":
				f.MemTotal = v * 1024
			case "MemAvailable:":
				f.MemAvail = v * 1024
			}
		}
		m.Close()
	}
	var st syscall.Statfs_t
	if syscall.Statfs("/", &st) == nil {
		f.DiskTotal = st.Blocks * uint64(st.Bsize)
		f.DiskFree = st.Bavail * uint64(st.Bsize)
	}
	f.Virt = detectVirt()
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
	}
	virtCache = v
	return v
}
