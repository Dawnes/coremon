// Package system holds the agent's built-in system actions (reboot).
package system

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"time"

	"coremon/internal/feature"
	"coremon/internal/proto"
)

// rebootDelay gives the agent time to report the result before the system
// goes down.
const rebootDelay = 5 * time.Second

// System implements feature.Feature.
type System struct {
	// reboot is replaceable in tests.
	reboot func() error
}

// New returns the system feature.
func New() *System { return &System{reboot: rebootNow} }

// Info describes the feature and its actions.
func (*System) Info() proto.FeatureInfo {
	return proto.FeatureInfo{
		Name:  "system",
		Title: "System",
		Actions: []proto.ActionSpec{
			{Name: "reboot", Title: "Reboot", Timeout: 60, Mutating: true},
		},
	}
}

// Call runs one action.
func (s *System) Call(c *feature.Ctx, action string) (any, error) {
	switch action {
	case "reboot":
		if runtime.GOOS != "linux" {
			return nil, errors.New("reboot is only supported on Linux")
		}
		c.Log("rebooting in %s", rebootDelay)
		go func() {
			time.Sleep(rebootDelay)
			if err := s.reboot(); err != nil {
				log.Printf("reboot failed: %v", err)
			}
		}()
		return map[string]any{"reboot_at": time.Now().Add(rebootDelay).Unix()}, nil
	}
	return nil, fmt.Errorf("unknown action %q", action)
}

func rebootNow() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	log.Printf("reboot requested by the master")
	if out, err := exec.CommandContext(ctx, "systemctl", "reboot").CombinedOutput(); err != nil {
		if out2, err2 := exec.CommandContext(ctx, "/sbin/reboot").CombinedOutput(); err2 != nil {
			return fmt.Errorf("systemctl reboot: %v %s; reboot: %v %s", err, out, err2, out2)
		}
	}
	return nil
}
