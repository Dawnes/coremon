package agent

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"coremon/internal/proto"
)

func TestProberSchedule(t *testing.T) {
	p := newProber()
	now := time.Unix(1_000_000, 0)
	ping := proto.ProbeCheck{ID: "l_1", Kind: "ping", Address: "10.0.0.5", Interval: 60, Timeout: 2, DownAfter: 2}
	ssh := proto.ProbeCheck{ID: "s_1", Kind: "tcp", Address: "10.0.0.5", Port: 22, Greeting: true, Interval: 300, Timeout: 5, Retries: 1}
	bad := []proto.ProbeList{
		{Version: "x", Checks: []proto.ProbeCheck{{ID: "a", Kind: "ping", Address: "host.lan", Interval: 60, Timeout: 2}}},
		{Version: "x", Checks: []proto.ProbeCheck{{ID: "a", Kind: "tcp", Address: "10.0.0.1", Port: 0, Interval: 60, Timeout: 2}}},
		{Version: "x", Checks: []proto.ProbeCheck{{ID: "a", Kind: "ping", Address: "10.0.0.1", Interval: 1, Timeout: 2}}},
		{Version: "x", Checks: []proto.ProbeCheck{ping, ping}},
	}
	for i, l := range bad {
		if p.setList(l, now) == nil {
			t.Errorf("bad list %d accepted", i)
		}
	}
	if err := p.setList(proto.ProbeList{Version: "v1", Checks: []proto.ProbeCheck{ping, ssh}}, now); err != nil {
		t.Fatal(err)
	}
	if v, n := p.version(); v != "v1" || n != 2 {
		t.Fatalf("version %q, %d checks", v, n)
	}
	// Both start within 20 s; a busy check is not handed out twice.
	if due := p.due(now.Add(20 * time.Second)); len(due) != 2 {
		t.Fatalf("due: %v", due)
	}
	if due := p.due(now.Add(21 * time.Second)); len(due) != 0 {
		t.Fatalf("busy checks handed out again: %v", due)
	}

	// The first result decides "up": a status change is sent right away.
	t0 := now.Add(20 * time.Second)
	p.record(ssh, proto.ProbeResult{ID: "s_1", OK: true, RTTUs: 900, Banner: "SSH-2.0"}, t0)
	select {
	case <-p.urgent:
	default:
		t.Error("status change not signalled")
	}
	// A failed TCP check is retried 30 s later; down after the retries.
	p.due(t0.Add(300 * time.Second))
	p.record(ssh, proto.ProbeResult{ID: "s_1", Error: "refused"}, t0.Add(300*time.Second))
	select {
	case <-p.urgent:
		t.Error("one failure with a retry left is no status change")
	default:
	}
	if p.checks["s_1"].next != t0.Add(330*time.Second) {
		t.Errorf("retry not 30 s later: %v", p.checks["s_1"].next.Sub(t0))
	}
	p.due(t0.Add(330 * time.Second))
	p.record(ssh, proto.ProbeResult{ID: "s_1", Error: "refused"}, t0.Add(330*time.Second))
	select {
	case <-p.urgent:
	default:
		t.Error("down not signalled")
	}

	// A new list keeps unchanged checks with their state and drops results of changed ones.
	ssh2 := ssh
	ssh2.Port = 2222
	p.setList(proto.ProbeList{Version: "v2", Checks: []proto.ProbeCheck{ping, ssh2}}, t0)
	p.record(ssh, proto.ProbeResult{ID: "s_1", OK: true}, t0.Add(400*time.Second)) // the old definition: ignored
	if p.checks["s_1"].state != 0 || p.checks["l_1"].c != ping {
		t.Errorf("list change: %+v", p.checks)
	}

	// Results go out oldest first; a failed send puts them back in front.
	rep, taken := p.take(2, t0.Add(400*time.Second))
	if rep.Version != "v2" || len(rep.Results) != 2 || rep.Results[0].AgoMs != 400_000 {
		t.Fatalf("take: %+v", rep)
	}
	p.putBack(taken)
	if p.pending() != 3 {
		t.Errorf("put back: %d pending", p.pending())
	}

	// Clear: nothing left, no version.
	p.clear()
	if v, n := p.version(); v != "" || n != 0 || p.pending() != 0 {
		t.Errorf("after clear: %q %d %d", v, n, p.pending())
	}
	p.record(ping, proto.ProbeResult{ID: "l_1", OK: true}, t0)
	if p.pending() != 0 {
		t.Error("a result of a cleared check was kept")
	}
}

func TestProberRun(t *testing.T) {
	p := newProber()
	p.fn.ping = func(ctx context.Context, ip net.IP, timeout time.Duration) (time.Duration, error) {
		if ip.String() == "10.0.0.5" {
			return 3 * time.Millisecond, nil
		}
		return 0, errors.New("no answer within 2s")
	}
	p.fn.tcp = func(ctx context.Context, addr string, port int, greeting bool, timeout time.Duration) (time.Duration, string, error) {
		return time.Millisecond, "220 ready", nil
	}
	r := p.run(context.Background(), proto.ProbeCheck{ID: "l_1", Kind: "ping", Address: "10.0.0.5", Interval: 60, Timeout: 2})
	if !r.OK || r.RTTUs != 3000 {
		t.Errorf("ping: %+v", r)
	}
	r = p.run(context.Background(), proto.ProbeCheck{ID: "l_2", Kind: "ping", Address: "10.0.0.6", Interval: 60, Timeout: 2})
	if r.OK || r.Error == "" {
		t.Errorf("unanswered ping: %+v", r)
	}
	r = p.run(context.Background(), proto.ProbeCheck{ID: "s_1", Kind: "tcp", Address: "10.0.0.5", Port: 25, Greeting: true, Interval: 60, Timeout: 2})
	if !r.OK || r.Banner != "220 ready" {
		t.Errorf("tcp: %+v", r)
	}
}
