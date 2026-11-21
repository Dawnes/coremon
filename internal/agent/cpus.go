package agent

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// cpuCount: the CPUs this process may run on now. Read fresh every time
// (one small file), because Go's runtime.NumCPU is counted once at start
// and Proxmox can move a container to other cores while it runs; a count
// from the start can then be the host's. Falls back to runtime.NumCPU.
func cpuCount() int {
	if b, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(line, "Cpus_allowed_list:"); ok {
				if n := parseCPUList(v); n > 0 {
					return n
				}
				break
			}
		}
	}
	return runtime.NumCPU()
}

// parseCPUList counts the CPUs in a Linux CPU list such as "0-3,6,8-9"
// (7); 0 when it cannot be read.
func parseCPUList(s string) int {
	n := 0
	for _, part := range strings.Split(strings.TrimSpace(s), ",") {
		if part == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.Atoi(lo)
		if err != nil || a < 0 {
			return 0
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(hi); err != nil || b < a {
				return 0
			}
		}
		n += b - a + 1
	}
	return n
}
