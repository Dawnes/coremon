//go:build !linux

package main

// Development builds on other systems: no root or ownership handling.
func isRoot() bool               { return true }
func chownLike(path, ref string) {}
