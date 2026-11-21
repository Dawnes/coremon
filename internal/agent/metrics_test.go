package agent

import (
	"testing"
	"time"

	"coremon/internal/proto"
)

func TestParseCPUList(t *testing.T) {
	for in, want := range map[string]int{"0": 1, "\t2\n": 1, "0-19": 20, "0-3,6,8-9": 7, "": 0, "x": 0, "3-1": 0, "0-": 0} {
		if got := parseCPUList(in); got != want {
			t.Errorf("%q: %d, want %d", in, got, want)
		}
	}
	if cpuCount() < 1 {
		t.Error("cpuCount below 1")
	}
}

func TestParseMetrics(t *testing.T) {
	if l1, l5, l15, ok := parseLoadavg("0.52 0.31 0.20 1/123 4567\n"); !ok || l1 != 0.52 || l5 != 0.31 || l15 != 0.20 {
		t.Errorf("loadavg: %v %v %v %v", l1, l5, l15, ok)
	}
	if _, _, _, ok := parseLoadavg("garbage"); ok {
		t.Error("bad loadavg accepted")
	}
	mi := "MemTotal:       16303428 kB\nMemFree:          812344 kB\nMemAvailable:    9876544 kB\nSwapTotal:       2097148 kB\n"
	if tot, av, ok := parseMeminfo(mi); !ok || tot != 16303428*1024 || av != 9876544*1024 {
		t.Errorf("meminfo: %d %d %v", tot, av, ok)
	}
	mounts := `rpool/data/subvol-101-disk-0 / zfs rw,relatime,xattr,posixacl 0 0
proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0
tmpfs /dev/shm tmpfs rw,nosuid,nodev 0 0
/dev/sda2 /boot ext4 rw,relatime 0 0
/dev/sdb1 /srv/my\040data xfs rw,relatime 0 0
/dev/sdb1 /mnt/bind xfs rw,relatime 0 0
/dev/loop3 /snap/core/123 squashfs ro 0 0
overlay /var/lib/docker/overlay2/x/merged overlay rw 0 0
/dev/sdc1 /var/lib/docker/volumes ext4 rw 0 0
server:/export /mnt/nfs nfs4 rw 0 0
`
	got := parseMounts(mounts)
	want := []string{"/", "/boot", "/mnt/bind"}
	if len(got) != len(want) {
		t.Fatalf("mounts: %+v", got)
	}
	for i, m := range got {
		if m.mount != want[i] {
			t.Errorf("mount %d: %q, want %q", i, m.mount, want[i])
		}
	}
	if s := unescapeMount(`/srv/my\040data\134x`); s != `/srv/my data\x` {
		t.Errorf("unescape: %q", s)
	}
}

// Readings wait until a poll delivers them, and come back when it fails.
func TestMeterQueue(t *testing.T) {
	n := 0
	m := &meter{read: func() ([]proto.DiskInfo, *proto.MetricSample) {
		n++
		return []proto.DiskInfo{{Mount: "/", Size: 10}}, &proto.MetricSample{CPUs: n}
	}}
	t0 := time.Now()
	m.measure(t0)
	m.measure(t0.Add(time.Minute))
	got, taken := m.take(t0.Add(90 * time.Second))
	if got == nil || len(got.Samples) != 2 || got.Samples[0].AgoMs != 90000 || got.Samples[1].CPUs != 2 || len(got.Disks) != 1 {
		t.Fatalf("take: %+v", got)
	}
	if again, _ := m.take(t0); again == nil || len(again.Samples) != 0 {
		t.Errorf("queue not emptied: %+v", again)
	}
	m.putBack(taken)
	m.measure(t0.Add(2 * time.Minute))
	if got, _ := m.take(t0.Add(2 * time.Minute)); len(got.Samples) != 3 || got.Samples[2].CPUs != 3 {
		t.Errorf("after put back: %+v", got.Samples)
	}
	for i := 0; i < metricKeep+10; i++ {
		m.measure(t0)
	}
	if got, _ := m.take(t0); len(got.Samples) != metricKeep {
		t.Errorf("kept %d readings", len(got.Samples))
	}
	if got, _ := (&meter{read: func() ([]proto.DiskInfo, *proto.MetricSample) { return nil, nil }}).take(t0); got != nil {
		t.Errorf("nothing measured: %+v", got)
	}
}
