//go:build linux

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func isRoot() bool { return os.Geteuid() == 0 }

// runAsOwner makes cmd run as the owner of dir, with no extra groups. dir
// itself must be a real directory (not a link).
func runAsOwner(cmd *exec.Cmd, dir string) error {
	st, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return errors.New(dir + " is not a directory")
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("cannot determine the owner of " + dir)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: s.Uid, Gid: s.Gid, Groups: []uint32{}}}
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
	return nil
}
