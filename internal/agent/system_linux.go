//go:build linux

package agent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"coremon/internal/proto"
)

const unitPath = "/etc/systemd/system/coremon-agent.service"

func selfUpdate(ctx context.Context, cl *client, pub string, a *proto.Artifact, logf func(string, ...any)) error {
	if a == nil || a.Kind != "core" {
		return errors.New("invalid core artifact")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.EvalSymlinks(exe)
	tmp := exe + ".new"
	logf("downloading agent-core %s", a.Version)
	if err := fetchVerified(ctx, cl, pub, a, tmp); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		os.Remove(tmp)
		return err
	}

	if out, err := exec.CommandContext(ctx, tmp, "version").Output(); err != nil {
		os.Remove(tmp)
		return errors.New("new binary failed to run: " + err.Error())
	} else {
		logf("new binary reports: %s", string(out))
	}
	return os.Rename(tmp, exe)
}

func uninstall(opt Options) error {
	_ = exec.Command("systemctl", "disable", "coremon-agent.service").Run()
	os.Remove(unitPath)
	_ = exec.Command("systemctl", "daemon-reload").Run()
	os.RemoveAll(opt.StateDir)
	os.RemoveAll(filepath.Dir(opt.ConfigPath))
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe)
	}
	return nil
}
