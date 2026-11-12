//go:build !linux

package main

import "os/exec"

// Development builds on other systems: no root or ownership handling.
func isRoot() bool                               { return true }
func runAsOwner(cmd *exec.Cmd, dir string) error { return nil }
