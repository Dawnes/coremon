// Command updates is the coremon module that reports and installs package
// updates. It never upgrades on its own: the master asks it to.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"

	"coremon/internal/modkit"
	"coremon/internal/proto"
)

var version = "dev"

// Package is one pending update.
type Package struct {
	Name      string `json:"name"`
	Arch      string `json:"arch,omitempty"`
	Current   string `json:"current,omitempty"` // empty for a newly pulled-in package
	Available string `json:"available"`
	Origin    string `json:"origin,omitempty"`
	Security  bool   `json:"security"`
	New       bool   `json:"new"` // not installed yet; comes in as a dependency
}

// Snapshot is the module state stored by the master.
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
}

// Backend abstracts a package manager.
type Backend interface {
	Name() string
	Refresh(c *modkit.Ctx) error
	List(c *modkit.Ctx) (*Snapshot, error)
	Install(c *modkit.Ctx, pkgs []string) error
	UpgradeAll(c *modkit.Ctx) error
}

// fakeEnabled is set by fake.go (build tag "fake") for development builds.
var fakeEnabled bool
var newFakeBackend func() Backend

func pickBackend() (Backend, error) {
	if fakeEnabled && newFakeBackend != nil && os.Getenv("COREMON_FAKE") == "1" {
		return newFakeBackend(), nil
	}
	if _, err := exec.LookPath("apt-get"); err == nil {
		return &aptBackend{}, nil
	}
	return nil, errors.New("no supported package manager found (supported: apt)")
}

var pkgNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9+.\-]{0,127}(:[a-z0-9]{1,16})?$`)

func finish(s *Snapshot) map[string]any {
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
	return map[string]any{"state": s}
}

func main() {
	m := proto.Manifest{
		Name:        "updates",
		Version:     version,
		Title:       "Updates",
		Description: "Lists available package updates and installs selected ones on request.",
		Actions: []proto.ActionSpec{
			{Name: "check", Title: "Refresh & check", Timeout: 900},
			{Name: "list", Title: "List (no refresh)", Timeout: 300},
			{Name: "install", Title: "Install selected", Timeout: 7200, Mutating: true},
			{Name: "upgrade_all", Title: "Upgrade all", Timeout: 7200, Mutating: true},
		},
	}
	withBackend := func(fn func(c *modkit.Ctx, b Backend) (any, error)) modkit.Handler {
		return func(c *modkit.Ctx) (any, error) {
			b, err := pickBackend()
			if err != nil {
				return nil, err
			}
			return fn(c, b)
		}
	}
	list := func(c *modkit.Ctx, b Backend) (any, error) {
		s, err := b.List(c)
		if err != nil {
			return nil, err
		}
		c.Log("%d update(s) available, %d security, reboot required: %v", len(s.Packages), countSec(s), s.RebootRequired)
		return finish(s), nil
	}
	modkit.Main(m, map[string]modkit.Handler{
		"check": withBackend(func(c *modkit.Ctx, b Backend) (any, error) {
			if err := b.Refresh(c); err != nil {
				return nil, fmt.Errorf("refresh failed: %w", err)
			}
			return list(c, b)
		}),
		"list": withBackend(list),
		"install": withBackend(func(c *modkit.Ctx, b Backend) (any, error) {
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
				if !pkgNameRe.MatchString(n) {
					return nil, fmt.Errorf("invalid package name %q", n)
				}
			}
			if err := b.Install(c, p.Packages); err != nil {
				return nil, fmt.Errorf("install failed: %w", err)
			}
			return list(c, b)
		}),
		"upgrade_all": withBackend(func(c *modkit.Ctx, b Backend) (any, error) {
			if err := b.UpgradeAll(c); err != nil {
				return nil, fmt.Errorf("upgrade failed: %w", err)
			}
			return list(c, b)
		}),
	})
}

func countSec(s *Snapshot) int {
	n := 0
	for _, p := range s.Packages {
		if p.Security {
			n++
		}
	}
	return n
}
