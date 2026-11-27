//go:build !notzdata

package master

// The time zone data, built in: every zone works, also on a system without
// tzdata. (notzdata: for a Go toolchain without it; zones are then read
// from the system.)
import _ "time/tzdata"
