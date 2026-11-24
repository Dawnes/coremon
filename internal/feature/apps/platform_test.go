package apps

import (
	"strings"
	"testing"
)

func TestPlatformOf(t *testing.T) {
	st := "Package: pve-manager\nStatus: install ok installed\nVersion: 9.0.11\n\nPackage: proxmox-backup-server\nStatus: install ok installed\nVersion: 4.0.14-1\n"
	if p := PlatformOf(strings.NewReader(st)); p == nil || p.ID != "pve" || p.Version != "9.0.11" {
		t.Errorf("pve: %+v", p)
	}
	st = "Package: proxmox-backup-server\nStatus: install ok installed\nVersion: 4.0.14-1\n\nPackage: pve-manager\nStatus: deinstall ok config-files\nVersion: 8.4.1\n"
	if p := PlatformOf(strings.NewReader(st)); p == nil || p.ID != "pbs" || p.Name != "Proxmox Backup Server" || p.Version != "4.0.14" {
		t.Errorf("pbs: %+v", p)
	}
	if p := PlatformOf(strings.NewReader("Package: apache2\nStatus: install ok installed\nVersion: 2.4\n")); p != nil {
		t.Errorf("none: %+v", p)
	}
}
