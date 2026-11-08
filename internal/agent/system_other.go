//go:build !linux

package agent

import (
	"context"
	"errors"

	"coremon/internal/proto"
)

func selfUpdate(ctx context.Context, cl *client, pub string, a *proto.Artifact, logf func(string, ...any)) error {
	return errors.New("self-update is only supported on Linux")
}

func uninstall(opt Options) error {
	return errors.New("uninstall is only supported on Linux")
}
