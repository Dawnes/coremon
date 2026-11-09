package agent

import (
	"reflect"
	"testing"
	"time"
)

func TestFailedUnits(t *testing.T) {
	out := "nginx.service loaded failed failed A high performance web server\nbackup.timer  loaded failed failed Nightly backup\n\n"
	n, names := parseFailedUnits(out)
	if n != 2 || !reflect.DeepEqual(names, []string{"nginx.service", "backup.timer"}) {
		t.Fatalf("got %d %v", n, names)
	}
	if n, names := parseFailedUnits(""); n != 0 || names != nil {
		t.Fatalf("none failed: %d %v", n, names)
	}
}

func TestAutoUpdates(t *testing.T) {
	status := "Package: apt\nStatus: install ok installed\n\nPackage: unattended-upgrades\nStatus: install ok installed\nVersion: 2.9\n\n"
	if !dpkgInstalled(status, "unattended-upgrades") || dpkgInstalled(status, "nginx") {
		t.Fatal("dpkg status")
	}
	if dpkgInstalled("Package: unattended-upgrades\nStatus: deinstall ok config-files\n", "unattended-upgrades") {
		t.Fatal("removed package counted as installed")
	}
	if !aptPeriodicOn(`APT::Periodic::Unattended-Upgrade "1";`+"\n") || aptPeriodicOn(`APT::Periodic::Unattended-Upgrade "0";`) || aptPeriodicOn("") {
		t.Fatal("apt-config value")
	}
	for _, c := range []struct {
		i, p, a bool
		want    string
	}{{true, true, true, "on"}, {true, false, true, "off"}, {true, true, false, "off"}, {false, true, true, "not_installed"}} {
		if got := autoUpdates(c.i, c.p, c.a); got != c.want {
			t.Errorf("autoUpdates(%v,%v,%v) = %s", c.i, c.p, c.a, got)
		}
	}
}

func TestLastUpgrade(t *testing.T) {
	old := `Start-Date: 2026-10-01  08:00:00
Commandline: /usr/bin/unattended-upgrade
Upgrade: libssl3:amd64 (3.0.1, 3.0.2), openssl:amd64 (3.0.1, 3.0.2)
End-Date: 2026-10-01  08:01:10
`
	cur := `Start-Date: 2026-11-03  14:10:00
Commandline: apt-get install --only-upgrade -q -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold -o DPkg::Lock::Timeout=300 -o Coremon::Run=1 -- curl tzdata
Upgrade: curl:amd64 (8.1, 8.2), tzdata:amd64 (2024a, 2025a)
End-Date: 2026-11-03  14:12:01

Start-Date: 2026-11-05  10:00:00
Commandline: apt install htop
Requested-By: peter (1000)
Install: htop:amd64 (3.2)
End-Date: 2026-11-05  10:00:05
`
	u := lastUpgrade([]string{old, cur}, time.UTC)
	want := time.Date(2026, 11, 3, 14, 12, 1, 0, time.UTC).Unix()
	if u == nil || u.Time != want || u.Packages != 2 || u.By != "coremon" {
		t.Fatalf("got %+v (installs only must not count)", u)
	}
	if u := lastUpgrade([]string{old}, time.UTC); u == nil || u.By != "automatic" || u.Packages != 2 {
		t.Fatalf("unattended: %+v", u)
	}
	byUser := "Start-Date: 2026-11-06  09:00:00\nCommandline: apt upgrade\nRequested-By: peter (1000)\nUpgrade: vim:amd64 (1, 2)\nEnd-Date: 2026-11-06  09:00:30\n"
	if u := lastUpgrade([]string{cur, byUser}, time.UTC); u.By != "user:peter" {
		t.Fatalf("by user: %+v", u)
	}
	if u := lastUpgrade([]string{"", ""}, time.UTC); u != nil {
		t.Fatalf("empty logs: %+v", u)
	}
	if upgradedBy("apt upgrade", "") != "root" {
		t.Fatal("root")
	}
}
