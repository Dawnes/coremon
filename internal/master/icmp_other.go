//go:build !linux

package master

// Development builds (not Linux): use the system's ping command.

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func icmpPing(ctx context.Context, ip net.IP, timeout time.Duration) (time.Duration, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "ping", "-n", "1", "-w", strconv.Itoa(int(timeout/time.Millisecond)), ip.String())
	} else {
		cmd = exec.CommandContext(ctx, "ping", "-c", "1", "-W", strconv.Itoa(int(timeout/time.Second)), ip.String())
	}
	start := time.Now()
	out, err := cmd.CombinedOutput()
	rtt := time.Since(start)
	lastMethod.Store("ping command")
	if err != nil || !strings.Contains(strings.ToLower(string(out)), "ttl=") {
		return 0, fmt.Errorf("no answer within %s", timeout)
	}
	return rtt, nil
}
