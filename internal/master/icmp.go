package master

// ICMP echo (ping) messages, shared by the platform-specific senders.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

var (
	pingSeq     atomic.Uint32
	lastMethod  atomic.Value // string
	pingPayload = []byte("coremon-ping")
)

func pingMethod() string {
	if v, ok := lastMethod.Load().(string); ok {
		return v
	}
	return ""
}

// echoRequest builds an ICMP (v4) or ICMPv6 echo request.
func echoRequest(v6 bool, id, seq uint16) []byte {
	b := make([]byte, 8+len(pingPayload))
	b[0] = 8
	if v6 {
		b[0] = 128
	}
	binary.BigEndian.PutUint16(b[4:], id)
	binary.BigEndian.PutUint16(b[6:], seq)
	copy(b[8:], pingPayload)
	if !v6 { // the kernel fills in the ICMPv6 checksum
		binary.BigEndian.PutUint16(b[2:], icmpChecksum(b))
	}
	return b
}

func icmpChecksum(b []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(b[i])<<8 | uint32(b[i+1])
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

// isEchoReply: an echo reply with this sequence number (and id, unless the
// kernel picks the id itself, as with a ping socket).
func isEchoReply(b []byte, v6 bool, id, seq uint16, checkID bool) bool {
	if len(b) < 8 {
		return false
	}
	want := byte(0)
	if v6 {
		want = 129
	}
	if b[0] != want || binary.BigEndian.Uint16(b[6:]) != seq {
		return false
	}
	return !checkID || binary.BigEndian.Uint16(b[4:]) == id
}

// exchange sends one echo request on c and waits for its reply.
func exchange(c net.PacketConn, to net.Addr, ip net.IP, v6 bool, id uint16, checkID bool, timeout time.Duration) (time.Duration, error) {
	seq := uint16(pingSeq.Add(1))
	msg := echoRequest(v6, id, seq)
	start := time.Now()
	c.SetDeadline(start.Add(timeout))
	if _, err := c.WriteTo(msg, to); err != nil {
		return 0, fmt.Errorf("send: %w", unwrapOp(err))
	}
	buf := make([]byte, 1500)
	for {
		n, from, err := c.ReadFrom(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return 0, fmt.Errorf("no answer within %s", timeout)
			}
			return 0, unwrapOp(err)
		}
		if !sameHost(from, ip) {
			continue
		}
		if isEchoReply(buf[:n], v6, id, seq, checkID) {
			return time.Since(start), nil
		}
	}
}

func sameHost(a net.Addr, ip net.IP) bool {
	switch x := a.(type) {
	case *net.UDPAddr:
		return x.IP.Equal(ip)
	case *net.IPAddr:
		return x.IP.Equal(ip)
	}
	return false
}

func unwrapOp(err error) error {
	var oe *net.OpError
	if errors.As(err, &oe) && oe.Err != nil {
		return oe.Err
	}
	return err
}
