//go:build !linux

package main

import "os/exec"

func isRoot() bool                               { return true }
func runAsOwner(cmd *exec.Cmd, dir string) error { return nil }
