package master

// OS support: a service on every agent device that is down once the OS
// release no longer gets security support (optionally a set number of days
// before). The master decides it from the support dates it already has
// (osinfo.go); nothing is measured on the device. It is added to every
// approved agent device and cannot be removed, only paused (muted). A
// release without known dates leaves it undecided (unknown, not down).

import (
	"errors"
	"fmt"
	"time"
)

const (
	methodOSSupport = "os_support"
	osSupportName   = "OS support"
	osSupportEvery  = 10 * time.Minute
	maxSupportDays  = 365
)

func (sv *Service) isOSSupport() bool { return sv.Method == methodOSSupport }

// osSupportSvc: the device's OS support service, or nil.
func (c *Checks) osSupportSvc() *Service {
	for _, sv := range c.Services {
		if sv.isOSSupport() {
			return sv
		}
	}
	return nil
}

// ensureOSSupport gives an approved agent device its OS support service
// (and a checks block when it has none). Caller holds s.st.mu.
func (s *Server) ensureOSSupport(a *Agent) *Service {
	if a.Status == StatusPending {
		return nil
	}
	if a.Checks == nil {
		a.Checks = &Checks{PingMode: pingRemoved}
	}
	if sv := a.Checks.osSupportSvc(); sv != nil {
		return sv
	}
	sv := &Service{ID: "s_" + randHex(8), Name: osSupportName, Method: methodOSSupport, Interval: int(osSupportEvery / time.Second)}
	a.Checks.Services = append(a.Checks.Services, sv)
	s.st.markDirty()
	return sv
}

// osSupportCheck adds the service when missing and decides it now. Not in
// maintenance, paused, or for an agent below the lowest allowed version (no
// data is used from it). Caller holds s.st.mu.
func (s *Server) osSupportCheck(a *Agent, now time.Time) {
	sv := s.ensureOSSupport(a)
	if sv == nil || sv.Paused || a.Maint != nil || s.belowMin(a) {
		return
	}
	text, fail, known := supportVerdict(s.supportFor(a.Facts.OSID, a.Facts.OSVersion, now), sv.Days, now)
	if !known {
		if sv.Checked || sv.Banner != text {
			sv.Checked, sv.Up, sv.Fails, sv.LastError, sv.Banner, sv.LastCheck = false, false, 0, "", text, now.Unix()
			s.markChecks()
		}
		return
	}
	var err error
	if fail != "" {
		err = errors.New(fail)
	}
	s.storeService(sv, 0, text, err == nil, err, "", now)
}

// supportVerdict: what the support dates mean for the service: a text, why
// it is down ("" when up), and whether anything is known at all.
func supportVerdict(o *osSupport, days int, now time.Time) (text, fail string, known bool) {
	if o == nil {
		return "No support dates are known for this OS release", "", false
	}
	if o.End == "" {
		return "Supported; no end date announced yet", "", true
	}
	end, err := time.Parse("2006-01-02", o.End)
	if err != nil {
		return "No support dates are known for this OS release", "", false
	}
	end = end.Add(24 * time.Hour) // supported through the whole last day
	text = "Supported until " + o.End
	if o.Phase == "security" && o.LTSUntil != "" {
		text = fmt.Sprintf("Security support until %s, LTS until %s", o.Until, o.LTSUntil)
	}
	left := int(end.Sub(now).Hours() / 24)
	switch {
	case !now.Before(end):
		return "Support ended on " + o.End, "support ended on " + o.End + ": no security updates any more", true
	case days > 0 && left < days:
		return text, fmt.Sprintf("support ends on %s, in %d day%s (down %d days before)", o.End, left, map[bool]string{true: "", false: "s"}[left == 1], days), true
	}
	return text, "", true
}

// osSupportLoop decides every agent device's OS support service now and
// then every 10 minutes.
func (s *Server) osSupportLoop(stop <-chan struct{}) {
	t := time.NewTicker(osSupportEvery)
	defer t.Stop()
	for {
		now := time.Now()
		s.st.mu.Lock()
		for _, a := range s.st.st.Agents {
			s.osSupportCheck(a, now)
		}
		s.st.mu.Unlock()
		select {
		case <-stop:
			return
		case <-t.C:
		}
	}
}

// applyOSSupport: only the name and the days before the end can change.
func (req *serviceReq) applyOSSupport(sv *Service) error {
	if (req.Preset != nil && *req.Preset != "") || (req.Method != nil && *req.Method != methodOSSupport) {
		return errors.New("OS support cannot be changed into another check")
	}
	if req.Name != nil {
		n, err := checkName("service name", *req.Name, maxServiceName)
		if err != nil {
			return err
		}
		if n != "" {
			sv.Name = n
		}
	}
	if req.Days != nil {
		if *req.Days < 0 || *req.Days > maxSupportDays {
			return fmt.Errorf("the days before the end must be 0-%d", maxSupportDays)
		}
		sv.Days = *req.Days
	}
	sv.Interval, sv.Retries, sv.Timeout = int(osSupportEvery/time.Second), 0, 0
	return nil
}

func (sv *Service) describeOSSupport() string {
	if sv.Days > 0 {
		return fmt.Sprintf("%s, down %d days before support ends", sv.Name, sv.Days)
	}
	return sv.Name + ", down when support has ended"
}
