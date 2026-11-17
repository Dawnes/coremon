package probe

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// TCP connects to addr:port; with greeting it waits for the first line the
// service sends. It returns the time to connect and that line (trimmed, not
// otherwise cleaned). Nothing is sent and nothing is logged in to.
func TCP(ctx context.Context, addr string, port int, greeting bool, timeout time.Duration) (time.Duration, string, error) {
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(addr, strconv.Itoa(port)))
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return 0, "", fmt.Errorf("no connection within %s", timeout)
		}
		return 0, "", dialError(err)
	}
	defer c.Close()
	rtt := time.Since(start)
	if !greeting {
		return rtt, "", nil
	}
	c.SetReadDeadline(time.Now().Add(timeout))
	line, err := bufio.NewReaderSize(c, 512).ReadSlice('\n')
	if len(line) == 0 && err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return 0, "", fmt.Errorf("connected, but no greeting within %s", timeout)
		}
		return 0, "", fmt.Errorf("connected, but no greeting: %v", UnwrapOp(err))
	}
	return rtt, strings.TrimSpace(string(line)), nil
}

// dialError: a plain reason for a failed connection.
func dialError(err error) error {
	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		return errors.New("connection refused: nothing listens on this port")
	case errors.Is(err, syscall.EHOSTUNREACH):
		return errors.New("host unreachable")
	case errors.Is(err, syscall.ENETUNREACH):
		return errors.New("network unreachable")
	}
	var se *os.SyscallError
	if errors.As(err, &se) {
		return se.Err
	}
	return UnwrapOp(err)
}
