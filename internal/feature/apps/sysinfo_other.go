//go:build !linux

package apps

// Non-Linux builds exist only for development: no services.
func collectSystem() *System { return nil }
