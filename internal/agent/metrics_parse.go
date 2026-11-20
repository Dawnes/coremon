package agent

import (
	"bufio"
	"sort"
	"strconv"
	"strings"
)

// parseLoadavg: the 1, 5 and 15 minute load averages of /proc/loadavg.
func parseLoadavg(s string) (l1, l5, l15 float64, ok bool) {
	f := strings.Fields(s)
	if len(f) < 3 {
		return 0, 0, 0, false
	}
	var err1, err2, err3 error
	l1, err1 = strconv.ParseFloat(f[0], 64)
	l5, err2 = strconv.ParseFloat(f[1], 64)
	l15, err3 = strconv.ParseFloat(f[2], 64)
	return l1, l5, l15, err1 == nil && err2 == nil && err3 == nil
}

// parseMeminfo: MemTotal and MemAvailable of /proc/meminfo, in bytes. Swap
// is not looked at.
func parseMeminfo(s string) (total, avail uint64, ok bool) {
	var haveT, haveA bool
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 {
			continue
		}
		v, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil {
			continue
		}
		switch f[0] {
		case "MemTotal:":
			total, haveT = v*1024, true
		case "MemAvailable:":
			avail, haveA = v*1024, true
		}
	}
	return total, avail, haveT && haveA && total > 0
}

// localFS: filesystem types that hold data on this system. Temporary,
// virtual, read-only images (squashfs), container layers (overlay) and
// network filesystems (they can hang) are left out.
var localFS = map[string]bool{
	"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true, "zfs": true, "f2fs": true,
	"jfs": true, "reiserfs": true, "vfat": true, "exfat": true, "ntfs": true, "ntfs3": true, "fuseblk": true,
}

type mountEntry struct{ source, mount, fs string }

// parseMounts: the local filesystems of /proc/self/mounts, one per source
// (bind mounts of the same filesystem are left out), sorted by mount point.
// Snap and container paths are skipped.
func parseMounts(s string) []mountEntry {
	bySrc := map[string]mountEntry{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 || !localFS[f[2]] {
			continue
		}
		e := mountEntry{source: f[0], mount: unescapeMount(f[1]), fs: f[2]}
		if strings.HasPrefix(e.mount, "/snap/") || strings.HasPrefix(e.mount, "/var/lib/docker/") || strings.HasPrefix(e.mount, "/run/") {
			continue
		}
		if old, seen := bySrc[e.source]; !seen || len(e.mount) < len(old.mount) {
			bySrc[e.source] = e
		}
	}
	out := make([]mountEntry, 0, len(bySrc))
	for _, e := range bySrc {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].mount < out[j].mount })
	return out
}

// unescapeMount: /proc/mounts writes space, tab, newline and backslash as \040 and so on.
func unescapeMount(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(v))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
