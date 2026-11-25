package master

// Disk, memory and load of an agent device: services like the others on
// its Services tab, but measured by the device's own agent every 60 s and
// sent with its regular contact (agent 0.3.12 and later), not checked from
// outside. So they have no address, port, "Checked by" or "Check now".
// Each reading past the limit counts as a failed check; after the set
// number of readings in a row the service is down. The readings also go
// into a 24-hour graph (five-minute slots).

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"coremon/internal/proto"
)

const (
	metricDisk = "disk"
	metricMem  = "memory"
	metricLoad = "load"

	diskUsed = "used" // disk limit: percent used
	diskFree = "free" // disk limit: GB free

	metricEverySec  = 60
	maxReadings     = 30
	maxMetricBatch  = 240
	maxAgentDisks   = 64
	gib             = 1 << 30
	defaultDiskPct  = 90.0 // space critically low at 90% used
	defaultDiskFree = 10.0 // GB
	defaultMemPct   = 90.0 // high memory use: 90% for 5 minutes
	defaultLoadCore = 1.5  // 1-minute load per CPU above 1.5 for 5 minutes
)

// metricDefaults: limit and readings in a row of a new service.
var metricDefaults = map[string]struct {
	limit    float64
	readings int
}{
	metricDisk: {defaultDiskPct, 3},
	metricMem:  {defaultMemPct, 5},
	metricLoad: {defaultLoadCore, 5},
}

// isMetric: measured by the device's own agent, not checked from outside.
func (sv *Service) isMetric() bool {
	return sv.Method == metricDisk || sv.Method == metricMem || sv.Method == metricLoad
}

// outside: the services checked from outside (by the master or a remote probe).
func (c *Checks) outside() []*Service {
	out := make([]*Service, 0, len(c.Services))
	for _, sv := range c.Services {
		if !sv.isMetric() && !sv.isOSSupport() {
			out = append(out, sv)
		}
	}
	return out
}

// metricUnit: the unit of the service's value and limit.
func (sv *Service) metricUnit() string {
	switch {
	case sv.Method == metricLoad:
		return "per core"
	case sv.Method == metricDisk && sv.LimitKind == diskFree:
		return "GB free"
	case sv.isMetric():
		return "% used"
	}
	return ""
}

// valSlot: the readings of five minutes, for the graph.
type valSlot struct {
	T   int64   `json:"t"`
	N   int     `json:"n"`
	Sum float64 `json:"sum"`
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

type valPoint struct {
	T   int64   `json:"t"`
	Avg float64 `json:"avg"`
	Min float64 `json:"min"`
	Max float64 `json:"max"`
	N   int     `json:"n"`
}

func addVal(vs []valSlot, v float64, t int64) []valSlot {
	slot := t - t%rttSlotSec
	if n := len(vs); n == 0 || vs[n-1].T != slot {
		vs = append(vs, valSlot{T: slot, Min: v, Max: v})
	}
	x := &vs[len(vs)-1]
	x.N++
	x.Sum += v
	x.Min, x.Max = math.Min(x.Min, v), math.Max(x.Max, v)
	cut := 0
	for cut < len(vs) && vs[cut].T <= t-histSpan {
		cut++
	}
	if cut > 0 {
		vs = append([]valSlot(nil), vs[cut:]...)
	}
	return vs
}

func recentVals(vs []valSlot, now int64) []valPoint {
	out := []valPoint{}
	for _, x := range vs {
		if x.T > now-histSpan && x.N > 0 {
			out = append(out, valPoint{T: x.T, Avg: round2(x.Sum / float64(x.N)), Min: round2(x.Min), Max: round2(x.Max), N: x.N})
		}
	}
	return out
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// gb: bytes as GB with one decimal (below 100) or none.
func gb(b uint64) string {
	v := float64(b) / gib
	if v < 100 {
		return fmt.Sprintf("%.1f GB", v)
	}
	return fmt.Sprintf("%.0f GB", v)
}

func num(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s
}

// reading: what one sample means for the service: the graph value, a text
// and, when past the limit (or not measurable), why it failed.
func (sv *Service) reading(m *proto.MetricSample) (val float64, has bool, text, fail string) {
	switch sv.Method {
	case metricDisk:
		for _, d := range m.Disks {
			if d.Mount != sv.Mount {
				continue
			}
			used := d.Size - min(d.Free, d.Size)
			pct := 0.0
			if used+d.Avail > 0 {
				pct = float64(used) / float64(used+d.Avail) * 100 // as df: the root reserve counts as used
			}
			text = fmt.Sprintf("%.0f%% used · %s free of %s", pct, gb(d.Avail), gb(d.Size))
			if sv.LimitKind == diskFree {
				val = round2(float64(d.Avail) / gib)
				if val < sv.Limit {
					fail = fmt.Sprintf("%s free on %s, less than %s GB", gb(d.Avail), sv.Mount, num(sv.Limit))
				}
				return val, true, text, fail
			}
			val = round2(pct)
			if pct > sv.Limit {
				fail = fmt.Sprintf("%s %.0f%% used, more than %s%%", sv.Mount, pct, num(sv.Limit))
			}
			return val, true, text, fail
		}
		return 0, false, "", sv.Mount + " is not mounted"
	case metricMem:
		if m.MemTotal == 0 {
			return 0, false, "", "the agent could not read the memory"
		}
		pct := float64(m.MemTotal-min(m.MemAvail, m.MemTotal)) / float64(m.MemTotal) * 100
		val, text = round2(pct), fmt.Sprintf("%.0f%% used · %s available of %s", pct, gb(m.MemAvail), gb(m.MemTotal))
		if pct > sv.Limit {
			fail = fmt.Sprintf("%.0f%% of the memory used, more than %s%%", pct, num(sv.Limit))
		}
		return val, true, text, fail
	case metricLoad:
		cpus := max(m.CPUs, 1)
		per := m.Load1 / float64(cpus)
		val = round2(per)
		text = fmt.Sprintf("%s per core · load %s / %s / %s on %d core%s", num(per), num(m.Load1), num(m.Load5), num(m.Load15), cpus, map[bool]string{true: "", false: "s"}[cpus == 1])
		if per > sv.Limit {
			fail = fmt.Sprintf("load %s per core, more than %s", num(per), num(sv.Limit))
		}
		return val, true, text, fail
	}
	return 0, false, "", "not a measured service"
}

// storeMetric stores one reading. Caller holds s.st.mu.
func (s *Server) storeMetric(sv *Service, val float64, has bool, text, fail string, at time.Time) {
	t := at.Unix()
	sv.LastCheck = t
	if has {
		sv.Value, sv.Banner = val, text
		sv.Vals = addVal(sv.Vals, val, t)
	}
	ok := fail == ""
	if ok {
		sv.LastOK, sv.Fails, sv.LastError = t, 0, ""
		if !sv.Checked || !sv.Up {
			sv.Checked, sv.Up, sv.Since = true, true, t
		}
	} else {
		sv.Fails++
		sv.LastError = clip(fail, 200)
		if sv.Fails >= sv.Readings && (!sv.Checked || sv.Up) {
			sv.Checked, sv.Up, sv.Since = true, false, t
		}
	}
	sv.Hist = addHist(sv.Hist, ok, "", t)
	s.markChecks()
}

// metricIntake: the filesystems and readings an agent sent with its poll.
// In maintenance the readings are dropped. Caller holds s.st.mu.
func (s *Server) metricIntake(a *Agent, m *proto.Metrics, now time.Time) {
	if m == nil {
		return
	}
	if m.Disks != nil { // the filesystems now; kept as they were when not sent
		disks := make([]proto.DiskInfo, 0, len(m.Disks))
		for _, d := range m.Disks {
			if len(disks) == maxAgentDisks {
				break
			}
			if mt := cleanText(d.Mount, 256); strings.HasPrefix(mt, "/") {
				disks = append(disks, proto.DiskInfo{Mount: mt, FS: cleanText(d.FS, 32), Size: d.Size})
			}
		}
		a.Disks = disks
	}
	if !agentHasChecks(a) || a.Maint != nil {
		return
	}
	samples := m.Samples
	if len(samples) > maxMetricBatch {
		samples = samples[len(samples)-maxMetricBatch:]
	}
	for i := range samples {
		smp := &samples[i]
		ago := min(max(smp.AgoMs, 0), histSpan*1000)
		at := now.Add(-time.Duration(ago) * time.Millisecond)
		for _, sv := range a.Checks.Services {
			if !sv.isMetric() || sv.Paused || at.Unix() <= sv.LastCheck {
				continue
			}
			val, has, text, fail := sv.reading(smp)
			s.storeMetric(sv, val, has, text, fail, at)
		}
	}
}

type metricReq struct {
	Mount     *string  `json:"mount"`
	Limit     *float64 `json:"limit"`
	LimitKind *string  `json:"limit_kind"`
	Readings  *int     `json:"readings"`
}

// setMetric: a service that becomes measured starts with the defaults.
func (sv *Service) setMetric(m string) {
	sv.Method, sv.Port, sv.Greeting, sv.Reverse = m, 0, false, false
	sv.Host, sv.Path, sv.Codes, sv.CertValid, sv.CertDays, sv.RType, sv.Expect, sv.MaxMs = "", "", "", false, 0, "", "", 0
	sv.Mount, sv.LimitKind = "", ""
	if m == metricDisk {
		sv.Mount, sv.LimitKind = "/", diskUsed
	}
	d := metricDefaults[m]
	sv.Limit, sv.Readings = d.limit, d.readings
	sv.Interval, sv.Retries, sv.Timeout = metricEverySec, 0, 0
}

// applyMetric checks and applies the settings of a measured service.
func (req *serviceReq) applyMetric(sv *Service) error {
	if req.Mount != nil && sv.Method == metricDisk {
		sv.Mount = strings.TrimSpace(*req.Mount)
	}
	if req.LimitKind != nil && sv.Method == metricDisk {
		if k := *req.LimitKind; k != diskUsed && k != diskFree {
			return errors.New("the disk limit is on percent used or GB free")
		} else if k != sv.LimitKind {
			sv.LimitKind = k
			if req.Limit == nil {
				sv.Limit = map[string]float64{diskUsed: defaultDiskPct, diskFree: defaultDiskFree}[k]
			}
		}
	}
	if req.Limit != nil {
		sv.Limit = round2(*req.Limit)
	}
	if req.Readings != nil {
		sv.Readings = *req.Readings
	}
	switch {
	case sv.Method == metricDisk && (!strings.HasPrefix(sv.Mount, "/") || len(sv.Mount) > 256):
		return errors.New("choose the filesystem (mount point) to watch")
	case sv.Method == metricLoad && (sv.Limit <= 0 || sv.Limit > 100):
		return errors.New("the load limit per core must be more than 0 and at most 100")
	case sv.Method == metricDisk && sv.LimitKind == diskFree && (sv.Limit <= 0 || sv.Limit > 1e6):
		return errors.New("the free space limit must be more than 0 GB")
	case sv.Method != metricLoad && sv.LimitKind != diskFree && (sv.Limit <= 0 || sv.Limit >= 100):
		return errors.New("the limit must be between 0 and 100%")
	case sv.Readings < 1 || sv.Readings > maxReadings:
		return fmt.Errorf("the readings in a row must be 1-%d", maxReadings)
	}
	sv.Interval, sv.Retries, sv.Timeout = metricEverySec, 0, 0
	return nil
}

func (sv *Service) describeMetric() string {
	what := map[string]string{metricDisk: "disk " + sv.Mount, metricMem: "memory", metricLoad: "load"}[sv.Method]
	lim := fmt.Sprintf("more than %s%% used", num(sv.Limit))
	switch {
	case sv.Method == metricLoad:
		lim = "load above " + num(sv.Limit) + " per core"
	case sv.LimitKind == diskFree:
		lim = "less than " + num(sv.Limit) + " GB free"
	}
	return fmt.Sprintf("%s (%s), down after %d reading%s with %s", sv.Name, what, sv.Readings, map[bool]string{true: "", false: "s"}[sv.Readings == 1], lim)
}

// metricTaken: another measured service of the device already watches the
// same thing (one memory and one load service; one per filesystem).
func (c *Checks) metricTaken(sv *Service) string {
	for _, x := range c.Services {
		if x.ID == sv.ID || x.Method != sv.Method {
			continue
		}
		if sv.Method != metricDisk {
			return x.Name + " already watches this"
		}
		if x.Mount == sv.Mount {
			return x.Name + " already watches " + sv.Mount
		}
	}
	return ""
}
