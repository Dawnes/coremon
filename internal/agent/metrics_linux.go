//go:build linux

package agent

import (
	"os"
	"runtime"
	"syscall"

	"coremon/internal/proto"
)

// readMetrics: load from /proc/loadavg, memory from /proc/meminfo and the
// local filesystems from /proc/self/mounts with statfs.
func readMetrics() ([]proto.DiskInfo, *proto.MetricSample) {
	s := &proto.MetricSample{CPUs: runtime.NumCPU()}
	ok := false
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if l1, l5, l15, good := parseLoadavg(string(b)); good {
			s.Load1, s.Load5, s.Load15, ok = l1, l5, l15, true
		}
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		if t, a, good := parseMeminfo(string(b)); good {
			s.MemTotal, s.MemAvail, ok = t, a, true
		}
	}
	var disks []proto.DiskInfo
	if b, err := os.ReadFile("/proc/self/mounts"); err == nil {
		disks = []proto.DiskInfo{}
		for _, m := range parseMounts(string(b)) {
			var st syscall.Statfs_t
			if syscall.Statfs(m.mount, &st) != nil || st.Blocks == 0 {
				continue
			}
			bs := uint64(st.Bsize)
			size := st.Blocks * bs
			disks = append(disks, proto.DiskInfo{Mount: m.mount, FS: m.fs, Size: size})
			s.Disks = append(s.Disks, proto.DiskUse{Mount: m.mount, Size: size, Free: st.Bfree * bs, Avail: st.Bavail * bs})
		}
		ok = true
	}
	if !ok {
		return disks, nil
	}
	return disks, s
}
