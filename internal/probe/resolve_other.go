//go:build !linux

package probe

// v6DefaultRoute: outside Linux the routes are not read; a global IPv6
// address counts as connectivity.
func v6DefaultRoute() bool { return true }
