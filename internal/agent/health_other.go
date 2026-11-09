//go:build !linux

package agent

import (
	"time"

	"coremon/internal/proto"
)

// Non-Linux builds exist only for development: simulated health
// (COREMON_FAKE=1), so the UI can be tested.
func collectHealth() *proto.Health {
	return &proto.Health{
		FailedCount: 2, Failed: []string{"nginx.service", "backup.timer"},
		AutoUpdates: "on",
		LastUpgrade: &proto.Upgrade{Time: time.Now().Add(-6 * 24 * time.Hour).Unix(), Packages: 12, By: "coremon"},
	}
}
