package agent

// Metrics: every 60 s the agent reads its own filesystems, memory and load.
// The readings wait in memory until a poll delivers them (at most two hours'
// worth); the master decides what they mean (the services and limits are
// set there), so the agent only measures.

import (
	"context"
	"sync"
	"time"

	"coremon/internal/proto"
)

const (
	metricEvery  = 60 * time.Second
	metricKeep   = 120 // readings kept while the master cannot be reached
	metricsBatch = 120
)

type metricReading struct {
	at time.Time
	s  proto.MetricSample
}

type meter struct {
	mu    sync.Mutex
	disks []proto.DiskInfo
	queue []metricReading
	read  func() ([]proto.DiskInfo, *proto.MetricSample) // the system's own figures; nil sample: cannot measure
}

func newMeter() *meter { return &meter{read: readMetrics} }

func (m *meter) loop(ctx context.Context) {
	m.measure(time.Now())
	t := time.NewTicker(metricEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			m.measure(now)
		}
	}
}

func (m *meter) measure(now time.Time) {
	disks, s := m.read()
	m.mu.Lock()
	defer m.mu.Unlock()
	if disks != nil {
		m.disks = disks
	}
	if s == nil {
		return
	}
	m.queue = append(m.queue, metricReading{at: now, s: *s})
	if len(m.queue) > metricKeep {
		m.queue = append([]metricReading(nil), m.queue[len(m.queue)-metricKeep:]...)
	}
}

// take: the filesystems and the waiting readings for a poll (nil when the
// system cannot be measured at all). The readings are handed back with
// putBack when the poll fails.
func (m *meter) take(now time.Time) (*proto.Metrics, []metricReading) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.disks == nil && len(m.queue) == 0 {
		return nil, nil
	}
	n := len(m.queue)
	if n > metricsBatch {
		n = metricsBatch
	}
	taken := append([]metricReading(nil), m.queue[:n]...)
	m.queue = append([]metricReading(nil), m.queue[n:]...)
	out := &proto.Metrics{Disks: append([]proto.DiskInfo{}, m.disks...)}
	for _, r := range taken {
		s := r.s
		s.AgoMs = now.Sub(r.at).Milliseconds()
		if s.AgoMs < 0 {
			s.AgoMs = 0
		}
		out.Samples = append(out.Samples, s)
	}
	return out, taken
}

func (m *meter) putBack(taken []metricReading) {
	if len(taken) == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queue = append(taken, m.queue...)
	if len(m.queue) > metricKeep {
		m.queue = append([]metricReading(nil), m.queue[len(m.queue)-metricKeep:]...)
	}
}
