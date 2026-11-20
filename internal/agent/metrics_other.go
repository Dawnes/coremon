//go:build !linux

package agent

import (
	"math"
	"os"
	"strconv"
	"time"

	"coremon/internal/proto"
)

// readMetrics: only the development build measures something, with made-up
// figures (COREMON_FAKE=1). COREMON_FAKE_DISK (percent used of /),
// COREMON_FAKE_MEM (percent used) and COREMON_FAKE_LOAD (load per core)
// force a value, for testing the limits.
func readMetrics() ([]proto.DiskInfo, *proto.MetricSample) {
	if os.Getenv("COREMON_FAKE") != "1" {
		return nil, nil
	}
	const gb = 1 << 30
	t := float64(time.Now().Unix())
	wave := func(period, amp float64) float64 { return amp * math.Sin(2*math.Pi*t/period) }
	force := func(env string, def float64) float64 {
		if v, err := strconv.ParseFloat(os.Getenv(env), 64); err == nil {
			return v
		}
		return def
	}
	cpus := 4
	rootPct := force("COREMON_FAKE_DISK", 62+wave(7200, 3))
	memPct := force("COREMON_FAKE_MEM", 45+wave(3600, 15))
	load := force("COREMON_FAKE_LOAD", 0.35+wave(1800, 0.25)) * float64(cpus)
	disks := []proto.DiskInfo{{Mount: "/", FS: "ext4", Size: 100 * gb}, {Mount: "/var", FS: "xfs", Size: 400 * gb}}
	use := func(size uint64, pct float64) proto.DiskUse {
		free := uint64(float64(size) * (100 - pct) / 100)
		return proto.DiskUse{Size: size, Free: free, Avail: free}
	}
	root, varUse := use(100*gb, rootPct), use(400*gb, 38)
	root.Mount, varUse.Mount = "/", "/var"
	total := uint64(16 * gb)
	return disks, &proto.MetricSample{CPUs: cpus, Load1: load, Load5: load * 0.9, Load15: load * 0.8,
		MemTotal: total, MemAvail: uint64(float64(total) * (100 - memPct) / 100), Disks: []proto.DiskUse{root, varUse}}
}
