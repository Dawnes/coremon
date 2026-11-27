package master

// The time zone of the installation (Settings → General): the website shows
// every time in it, backups run at 01:00 in it and notifications use it for
// quiet hours and the daily summary. The zone data comes from the host's
// tzdata package (/usr/share/zoneinfo), which the installer makes sure of.

import (
	"sync"
	"time"
)

const defaultTimeZone = "Europe/Amsterdam"

var zoneCache sync.Map // name → *time.Location

// zoneOf: the location of an IANA name; UTC when it is unknown.
func zoneOf(name string) *time.Location {
	if l, ok := zoneCache.Load(name); ok {
		return l.(*time.Location)
	}
	l, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	zoneCache.Store(name, l)
	return l
}

// zone: the installation's time zone. Caller holds s.st.mu.
func (s *Server) zone() *time.Location { return zoneOf(s.st.st.Settings.TimeZone) }

// zoneNow: the installation's time zone, taking the lock itself.
func (s *Server) zoneNow() *time.Location {
	s.st.mu.Lock()
	defer s.st.mu.Unlock()
	return s.zone()
}
