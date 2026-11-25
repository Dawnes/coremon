// Package updates reports pending updates and installs them on request; it never upgrades on its own.
package updates

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"coremon/internal/feature"
	"coremon/internal/feature/apps"
	"coremon/internal/proto"
)

type Package struct {
	Name      string `json:"name"`
	Arch      string `json:"arch,omitempty"`
	Current   string `json:"current,omitempty"`
	Available string `json:"available"`
	Origin    string `json:"origin,omitempty"`
	// Source: the source package (apk, Tumbleweed): an update of another
	// package of the same source fixes the same CVEs.
	Source   string `json:"source,omitempty"`
	Security bool   `json:"security"`
	New      bool   `json:"new"`
}

type Snapshot struct {
	Backend        string    `json:"backend"`
	CheckedAt      int64     `json:"checked_at"`
	RefreshedAt    int64     `json:"refreshed_at"`
	Packages       []Package `json:"packages"`
	Removals       []string  `json:"removals"`
	RebootRequired bool      `json:"reboot_required"`
	RebootPkgs     []string  `json:"reboot_pkgs"`
	Total          int       `json:"total"`
	Security       int       `json:"security"`
	// SecurityFromCVE: the package manager does not mark security updates
	// (apk, zypper on Tumbleweed): the master counts an update as security
	// when it fixes a CVE it knows for the device.
	SecurityFromCVE bool `json:"security_from_cve,omitempty"`
	// NoSources: no enabled package sources (e.g. SLES not registered):
	// "up to date" would mean nothing.
	NoSources bool `json:"no_sources,omitempty"`
}

type Backend interface {
	Refresh(c *feature.Ctx) error
	List(c *feature.Ctx) (*Snapshot, error)
	Install(c *feature.Ctx, pkgs []string) error
	UpgradeAll(c *feature.Ctx) error
}

var fakeEnabled bool
var newFakeBackend func() Backend

func pickBackend() (Backend, error) {
	if fakeEnabled && newFakeBackend != nil && os.Getenv("COREMON_FAKE") == "1" {
		return newFakeBackend(), nil
	}
	if _, err := exec.LookPath("apt-get"); err == nil {
		return &aptBackend{}, nil
	}
	// zypper before dnf: openSUSE can have dnf installed too, but zypper is
	// its package manager.
	if _, err := exec.LookPath("zypper"); err == nil {
		return &zypperBackend{rolling: osReleaseID() == "opensuse-tumbleweed"}, nil
	}
	if _, err := exec.LookPath("dnf"); err == nil {
		return &dnfBackend{}, nil
	}
	if _, err := exec.LookPath("apk"); err == nil {
		return &apkBackend{}, nil
	}
	return nil, errors.New("no supported package manager found (supported: apt, dnf, zypper, apk)")
}

// osReleaseID: ID from /etc/os-release.
func osReleaseID() string {
	b, _ := os.ReadFile("/etc/os-release")
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "ID="); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// PackageNameRe: a Debian or RPM package name (RPM names may have capitals
// and underscores, like NetworkManager or perl_Foo).
var PackageNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+._\-]{0,127}(:[a-z0-9]{1,16})?$`)

type Updates struct{}

func New() *Updates { return &Updates{} }

func (*Updates) Info() proto.FeatureInfo {
	return proto.FeatureInfo{
		Name:  "updates",
		Title: "Updates",
		Actions: []proto.ActionSpec{
			{Name: "check", Title: "Refresh & check", Timeout: 900},
			{Name: "list", Title: "List (no refresh)", Timeout: 300},
			{Name: "install", Title: "Install selected", Timeout: 7200, Mutating: true},
			{Name: "upgrade_all", Title: "Upgrade all", Timeout: 7200, Mutating: true},
		},
	}
}

func (*Updates) Call(c *feature.Ctx, action string) (any, error) {
	b, err := pickBackend()
	if err != nil {
		return nil, err
	}
	switch action {
	case "check":
		if err := b.Refresh(c); err != nil {
			return nil, fmt.Errorf("refresh failed: %w", err)
		}
		return list(c, b)
	case "list":
		return list(c, b)
	case "install":
		var p struct {
			Packages []string `json:"packages"`
		}
		if err := json.Unmarshal(c.Params, &p); err != nil {
			return nil, fmt.Errorf("bad params: %w", err)
		}
		if len(p.Packages) == 0 {
			return nil, errors.New("no packages given")
		}
		for _, n := range p.Packages {
			if !PackageNameRe.MatchString(n) {
				return nil, fmt.Errorf("invalid package name %q", n)
			}
		}
		if err := b.Install(c, p.Packages); err != nil {
			return nil, fmt.Errorf("install failed: %w", err)
		}
		return list(c, b)
	case "upgrade_all":
		if err := b.UpgradeAll(c); err != nil {
			return nil, fmt.Errorf("upgrade failed: %w", err)
		}
		return list(c, b)
	}
	return nil, fmt.Errorf("unknown action %q", action)
}

func list(c *feature.Ctx, b Backend) (any, error) {
	s, err := b.List(c)
	if err != nil {
		return nil, err
	}
	finish(s)
	c.Log("%d update(s) available, %d security, reboot required: %v", s.Total, s.Security, s.RebootRequired)

	return map[string]any{"state": s, "states": map[string]any{"apps": apps.Scan()}}, nil
}

func finish(s *Snapshot) {
	s.CheckedAt = time.Now().Unix()
	s.Total, s.Security = 0, 0
	for _, p := range s.Packages {
		if p.New {
			continue
		}
		s.Total++
		if p.Security {
			s.Security++
		}
	}
	if s.Packages == nil {
		s.Packages = []Package{}
	}
	if s.Removals == nil {
		s.Removals = []string{}
	}
	if s.RebootPkgs == nil {
		s.RebootPkgs = []string{}
	}
}
