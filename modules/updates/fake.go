//go:build fake

package main

// fakeBackend simulates a Debian system with pending updates. Only compiled
// with `-tags fake` and enabled with COREMON_FAKE=1; used for UI
// development and end-to-end tests on machines without apt. Release builds
// never contain it.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"coremon/internal/modkit"
)

func init() {
	fakeEnabled = true
	newFakeBackend = func() Backend { return &fakeBackend{} }
}

type fakeBackend struct{}

func (fakeBackend) Name() string { return "fake" }

func fakeFile() string { return filepath.Join(os.TempDir(), "coremon-fake-updates.json") }

func fakeDefault() []Package {
	return []Package{
		{Name: "libssl3", Arch: "amd64", Current: "3.0.11-1~deb12u2", Available: "3.0.15-1~deb12u1", Origin: "Debian-Security:12/stable-security", Security: true},
		{Name: "openssl", Arch: "amd64", Current: "3.0.11-1~deb12u2", Available: "3.0.15-1~deb12u1", Origin: "Debian-Security:12/stable-security", Security: true},
		{Name: "libc6", Arch: "amd64", Current: "2.36-9+deb12u7", Available: "2.36-9+deb12u9", Origin: "Debian-Security:12/stable-security", Security: true},
		{Name: "base-files", Arch: "amd64", Current: "12.4+deb12u7", Available: "12.4+deb12u8", Origin: "Debian:12.8/stable"},
		{Name: "tzdata", Arch: "all", Current: "2024a-0+deb12u1", Available: "2024b-0+deb12u1", Origin: "Debian:12.8/stable"},
		{Name: "curl", Arch: "amd64", Current: "7.88.1-10+deb12u7", Available: "7.88.1-10+deb12u8", Origin: "Debian:12.8/stable"},
		{Name: "linux-image-6.1.0-28-amd64", Arch: "amd64", Available: "6.1.119-1", Origin: "Debian:12.8/stable", New: true},
	}
}

func (fakeBackend) load() []Package {
	b, err := os.ReadFile(fakeFile())
	if err != nil {
		return fakeDefault()
	}
	var p []Package
	if json.Unmarshal(b, &p) != nil {
		return fakeDefault()
	}
	return p
}

func (fakeBackend) save(p []Package) {
	b, _ := json.Marshal(p)
	_ = os.WriteFile(fakeFile(), b, 0o600)
}

func (f fakeBackend) Refresh(c *modkit.Ctx) error {
	for _, l := range []string{"Hit:1 http://deb.debian.org/debian bookworm InRelease", "Get:2 http://security.debian.org/debian-security bookworm-security InRelease [48.0 kB]", "Fetched 48.0 kB in 1s (52.1 kB/s)", "Reading package lists..."} {
		c.Log("%s", l)
		time.Sleep(300 * time.Millisecond)
	}
	return nil
}

func (f fakeBackend) List(c *modkit.Ctx) (*Snapshot, error) {
	p := f.load()
	return &Snapshot{Backend: "fake", Packages: p, RefreshedAt: time.Now().Unix(), RebootRequired: len(p) < len(fakeDefault()), RebootPkgs: []string{"libc6"}}, nil
}

func (f fakeBackend) Install(c *modkit.Ctx, pkgs []string) error {
	want := map[string]bool{}
	for _, n := range pkgs {
		want[n] = true
	}
	var keep []Package
	for _, p := range f.load() {
		if want[p.Name] {
			c.Log("Unpacking %s (%s) over (%s) ...", p.Name, p.Available, p.Current)
			time.Sleep(400 * time.Millisecond)
			c.Log("Setting up %s (%s) ...", p.Name, p.Available)
			continue
		}
		keep = append(keep, p)
	}
	if keep == nil {
		keep = []Package{}
	}
	f.save(keep)
	return nil
}

func (f fakeBackend) UpgradeAll(c *modkit.Ctx) error {
	var names []string
	for _, p := range f.load() {
		names = append(names, p.Name)
	}
	return f.Install(c, names)
}
