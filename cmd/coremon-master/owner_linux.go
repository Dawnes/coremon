//go:build linux

package main

import (
	"os"
	"syscall"
)

func isRoot() bool { return os.Geteuid() == 0 }

// chownLike gives path the owner and group of ref.
func chownLike(path, ref string) {
	st, err := os.Stat(ref)
	if err != nil {
		return
	}
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		os.Chown(path, int(s.Uid), int(s.Gid))
	}
}
