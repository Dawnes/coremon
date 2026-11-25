package updates

import "testing"

func TestRPMVerCmp(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.0", "1.0", 0},
		{"1.0", "1.1", -1},
		{"1.10", "1.9", 1},
		{"9.9p1-7.el10", "9.9p1-34.el10_3.alma.1", -1},
		{"1:3.0.7-27.el9", "1:3.0.7-28.el9_4", -1},
		{"1:3.0.7-28.el9_4", "3.5.0-1.el10", 1}, // epoch wins
		{"0:1.2-1", "1.2-1", 0},
		{"1.0~rc1", "1.0", -1},
		{"1.0^git1", "1.0", 1},
		{"1.0^git1", "1.0.1", -1},
		{"1.0a", "1.0", 1},
		{"1.0a", "1.0.1", -1}, // digits newer than letters
		{"2.4.62-1.el10", "2.4.62-1.el10_0", -1},
		{"6.12.0-264.16.1.el10_3", "6.12.0-264.9.1.el10_3", 1},
		{"1.2", "1.2-5", 0}, // no release on one side
	} {
		if got := RPMVerCmp(c.a, c.b); got != c.want {
			t.Errorf("%s vs %s: %d, want %d", c.a, c.b, got, c.want)
		}
		if got := RPMVerCmp(c.b, c.a); got != -c.want {
			t.Errorf("%s vs %s: %d, want %d", c.b, c.a, got, -c.want)
		}
	}
}

func TestParseDnf(t *testing.T) {
	inst := "openssl|x86_64|1:3.2.2-6.el10|\nopenssl-libs|x86_64|1:3.2.2-6.el10\nkernel|x86_64|0:6.12.0-55.el10\nkernel|x86_64|0:6.12.0-124.el10\nbash|x86_64|0:5.2.26-6.el10\ntzdata|noarch|0:2025a-1.el10\n"
	// dnf 4 adds its own newline: lines may end in a literal \n or be empty.
	up := "openssl|x86_64|1:3.2.2-7.el10_1|baseos\\n\n\nopenssl-libs|x86_64|1:3.2.2-7.el10_1|baseos\nkernel|x86_64|6.12.0-130.el10|baseos\nbash|x86_64|5.2.26-7.el10|baseos\ntzdata|noarch|2025b-1.el10|appstream\nnewdep|x86_64|1.0-1.el10|appstream\n"
	adv := `RLSA-2025:1234 Important/Sec. openssl-1:3.2.2-7.el10_1.x86_64
RLSA-2025:1234 Important/Sec. openssl-libs-1:3.2.2-7.el10_1.x86_64
RLSA-2025:0999 Moderate/Sec.  kernel-6.12.0-124.el10.x86_64
RLSA-2025:1300 Low/Sec.       kernel-6.12.0-130.el10.x86_64
`
	s := parseDnf(inst, up, adv)
	got := map[string]Package{}
	for _, p := range s.Packages {
		got[p.Name] = p
	}
	if len(s.Packages) != 6 {
		t.Fatalf("packages: %+v", s.Packages)
	}
	if p := got["openssl"]; !p.Security || p.Current != "1:3.2.2-6.el10" || p.Available != "1:3.2.2-7.el10_1" || p.Origin != "baseos" || p.Arch != "x86_64" {
		t.Errorf("openssl: %+v", p)
	}
	if p := got["openssl-libs"]; !p.Security {
		t.Errorf("openssl-libs: %+v", p)
	}
	// The newest installed kernel counts; the advisory for 124 is installed already, 130 is new.
	if p := got["kernel"]; p.Current != "6.12.0-124.el10" || !p.Security {
		t.Errorf("kernel: %+v", p)
	}
	if p := got["bash"]; p.Security || p.New {
		t.Errorf("bash: %+v", p)
	}
	if p := got["newdep"]; !p.New {
		t.Errorf("newdep: %+v", p)
	}
	if s.Packages[0].Name != "kernel" || !s.Packages[0].Security { // security first, by name
		t.Errorf("order: %+v", s.Packages[0])
	}
	// No advisories: nothing is security.
	for _, p := range parseDnf(inst, up, "").Packages {
		if p.Security {
			t.Errorf("security without an advisory: %+v", p)
		}
	}
	// An advisory for openssl-libs only must not mark openssl.
	if p := parseDnf(inst, up, "X Important/Sec. openssl-libs-1:3.2.2-7.el10_1.x86_64").Packages; func() bool {
		for _, q := range p {
			if q.Name == "openssl" && q.Security {
				return true
			}
		}
		return false
	}() {
		t.Error("openssl marked by the openssl-libs advisory")
	}
}

func TestDnfReboot(t *testing.T) {
	out := `Core libraries or services have been updated since boot-up:
  * glibc
  * kernel

Reboot is required to fully utilize these updates.
More information: https://access.redhat.com/solutions/27943
`
	if r, pkgs := dnfReboot(out); !r || len(pkgs) != 2 || pkgs[0] != "glibc" || pkgs[1] != "kernel" {
		t.Errorf("reboot: %v %v", r, pkgs)
	}
	if r, _ := dnfReboot("No core libraries or services have been updated since boot-up.\nReboot should not be necessary.\n"); r {
		t.Error("no reboot needed")
	}
	if r, _ := dnfReboot(""); r {
		t.Error("no output: no reboot")
	}
}
