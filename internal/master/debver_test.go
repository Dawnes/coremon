package master

import "testing"

func TestDebVerCmp(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0", "1.0", 0},
		{"1.0", "1.1", -1},
		{"1.10", "1.9", 1},
		{"1:1.0", "2.0", 1}, // epoch wins
		{"1.0~rc1", "1.0", -1},
		{"1.0~rc1", "1.0~rc2", -1},
		{"1.0", "1.0+deb12u1", -1},
		{"3.0.15-1~deb12u1", "3.0.17-1~deb12u2", -1},
		{"3.0.17-1~deb12u2", "3.0.17-1~deb12u3", -1},
		{"2.4.62-1~deb12u2", "2.4.62-1", -1}, // ~ sorts before the end
		{"1:9.2p1-2+deb12u3", "1:9.2p1-2+deb12u6", -1},
		{"1:10.11.6-0+deb12u1", "1:10.11.11-0+deb12u1", -1},
		{"3.0.13-0ubuntu3.1", "3.0.13-0ubuntu3.5", -1},
		{"3.0.13-0ubuntu3.5", "3.0.13-0ubuntu3.5+esm1", -1},
		{"1.0a", "1.0", 1}, // a letter sorts after the end
		{"1.0-1", "1.0-1.1", -1},
		{"0", "0.1", -1},
		{"1.00", "1.0", 0},
	}
	for _, c := range cases {
		if got := debVerCmp(c.a, c.b); got != c.want {
			t.Errorf("debVerCmp(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := debVerCmp(c.b, c.a); got != -c.want {
			t.Errorf("debVerCmp(%q, %q) = %d, want %d", c.b, c.a, got, -c.want)
		}
	}
}

func TestCVSS3(t *testing.T) {
	cases := []struct {
		v    string
		want float64
	}{
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", 9.8},
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H", 10.0},
		{"CVSS:3.1/AV:P/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N", 4.6},
		{"CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:H/I:N/A:N", 5.9},
		{"CVSS:3.1/AV:L/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H", 7.8},
		{"CVSS:3.0/AV:N/AC:L/PR:L/UI:R/S:C/C:L/I:L/A:N", 5.4},
		{"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:N/I:N/A:N", 0},
	}
	for _, c := range cases {
		got, ok := cvss3Score(c.v)
		if !ok || got != c.want {
			t.Errorf("%s: %v %v, want %v", c.v, got, ok, c.want)
		}
	}
	for _, bad := range []string{"", "CVSS:2.0/AV:N", "CVSS:3.1/AV:X/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", "CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"} {
		if _, ok := cvss3Score(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
	if scoreLevel(9.8) != 4 || scoreLevel(7.0) != 3 || scoreLevel(6.9) != 2 || scoreLevel(3.9) != 1 {
		t.Error("score levels")
	}
}
