package master

// Intake limits for the agent endpoint. A request is read and decrypted
// before the master knows who sent it, so without limits anyone could make
// the master hold many large bodies in memory at once. Requests are
// admitted per address and in total, small and large separately; the slot
// is held only while the body is read and processed (not while a poll
// waits for commands).

import (
	"net/http"
	"sync"
	"time"
)

const (
	intakeLargeFrom   = 256 << 10 // bodies above this are "large"
	intakeSmallTotal  = 64        // small requests being processed at once
	intakeSmallPerIP  = 8
	intakeLargeTotal  = 2 // large requests being processed at once
	intakeLargePerIP  = 1
	intakeSmallWait   = 5 * time.Second
	intakeLargeWait   = 20 * time.Second
	intakeRetryAfterS = "5"
)

type intake struct {
	mu          sync.Mutex
	small       int
	large       int
	smallPer    map[string]int
	largePer    map[string]int
	smallTotal  int // limits (0 = defaults); set in tests
	largeTotal  int
	smallPerMax int
	largePerMax int
	wait        time.Duration // how long to wait for a slot (0 = defaults); set in tests
}

func (in *intake) limits() (st, sp, lt, lp int) {
	st, sp, lt, lp = in.smallTotal, in.smallPerMax, in.largeTotal, in.largePerMax
	if st == 0 {
		st = intakeSmallTotal
	}
	if sp == 0 {
		sp = intakeSmallPerIP
	}
	if lt == 0 {
		lt = intakeLargeTotal
	}
	if lp == 0 {
		lp = intakeLargePerIP
	}
	return
}

// try takes a slot if one is free.
func (in *intake) try(ip string, large bool) bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.smallPer == nil {
		in.smallPer, in.largePer = map[string]int{}, map[string]int{}
	}
	st, sp, lt, lp := in.limits()
	if large {
		if in.large >= lt || in.largePer[ip] >= lp {
			return false
		}
		in.large++
		in.largePer[ip]++
		return true
	}
	if in.small >= st || in.smallPer[ip] >= sp {
		return false
	}
	in.small++
	in.smallPer[ip]++
	return true
}

func (in *intake) release(ip string, large bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if large {
		in.large--
		if in.largePer[ip]--; in.largePer[ip] <= 0 {
			delete(in.largePer, ip)
		}
		return
	}
	in.small--
	if in.smallPer[ip]--; in.smallPer[ip] <= 0 {
		delete(in.smallPer, ip)
	}
}

// admit waits briefly for a slot. It returns a release function (safe to
// call more than once), or nil after answering 503.
func (in *intake) admit(w http.ResponseWriter, r *http.Request) func() {
	// Unknown length (chunked) counts as large.
	large := r.ContentLength < 0 || r.ContentLength > intakeLargeFrom
	ip := clientIP(r)
	wait := intakeSmallWait
	if large {
		wait = intakeLargeWait
	}
	if in.wait > 0 {
		wait = in.wait
	}
	deadline := time.Now().Add(wait)
	for !in.try(ip, large) {
		if time.Now().After(deadline) || r.Context().Err() != nil {
			w.Header().Set("Retry-After", intakeRetryAfterS)
			http.Error(w, "busy, try again", http.StatusServiceUnavailable)
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	var once sync.Once
	return func() { once.Do(func() { in.release(ip, large) }) }
}
