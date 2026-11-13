//go:build !linux

package agent

import (
	"time"

	"coremon/internal/proto"
)

func collectHealth() *proto.Health {
	return &proto.Health{
		FailedCount: 2, Failed: []string{"nginx.service", "backup.timer"},
		AutoUpdates: "on",
		LastUpgrade: &proto.Upgrade{Time: time.Now().Add(-6 * 24 * time.Hour).Unix(), Packages: 12, By: "coremon"},
	}
}
