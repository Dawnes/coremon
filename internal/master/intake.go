package master

// Limits on agent requests, which are read before the sender is known.

import (
	"net/http"
	"sync"
	"time"
)

const (
	intakeLargeFrom   = 256 << 10
	intakeSmallTotal  = 64
	intakeSmallPerIP  = 8
	intakeLargeTotal  = 2
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
	smallTotal  int
	largeTotal  int
	smallPerMax int
	largePerMax int
	wait        time.Duration
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

func (in *intake) admit(w http.ResponseWriter, r *http.Request) func() {
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
