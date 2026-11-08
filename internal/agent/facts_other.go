//go:build !linux

package agent

import (
	"runtime"

	"coremon/internal/proto"
)

// Non-Linux builds exist only for development and testing.
func collectFacts() proto.Facts {
	f := baseFacts()
	f.OSName = runtime.GOOS + " (development build)"
	f.OSID = runtime.GOOS
	f.Virt = "none"
	return f
}
