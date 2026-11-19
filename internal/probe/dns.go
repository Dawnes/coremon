package probe

// The DNS check: one query to the DNS server on the device itself, in
// Coremon's own code (the resolver of the checking system is never used).
// UDP first, asking for answers up to 1232 bytes (EDNS0); the same query
// over TCP when the server marks the UDP answer as truncated. Recursion
// desired is always set: a caching resolver also looks up names it does
// not hold, an authoritative server ignores it.

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	dnsUDPSize  = 1232
	maxDNSName  = 253
	maxDNSJumps = 32
)

var dnsTypeCode = map[string]uint16{"A": 1, "NS": 2, "CNAME": 5, "MX": 15, "TXT": 16, "AAAA": 28}

var dnsRcodes = map[int]string{
	1: "format error (FORMERR)", 2: "server failure (SERVFAIL)", 3: "name does not exist (NXDOMAIN)",
	4: "not implemented (NOTIMP)", 5: "query refused (REFUSED)",
}

var errDNSBroken = errors.New("the answer is not a valid DNS message")

func validDNS(s Service) error {
	if s.Greeting {
		return errors.New("a DNS check does not wait for a greeting")
	}
	if err := ValidDNSName(s.Host); err != nil {
		return err
	}
	if _, ok := dnsTypeCode[s.RType]; !ok {
		return fmt.Errorf("the record type must be one of %s", strings.Join(DNSTypes, ", "))
	}
	if len(s.Expect) > maxExpect {
		return fmt.Errorf("the expected value may have at most %d characters", maxExpect)
	}
	for _, c := range s.Expect {
		if c < ' ' || c == 0x7f {
			return errors.New("the expected value may not contain control characters")
		}
	}
	if s.MaxMs < 0 || s.MaxMs > MaxMaxMs {
		return fmt.Errorf("the maximum response time must be 1-%d ms", MaxMaxMs)
	}
	return nil
}

// ValidDNSName: a domain name such as example.com or _dmarc.example.com
// (underscores allowed, a trailing dot allowed).
func ValidDNSName(n string) error {
	if strings.TrimSuffix(n, ".") == "" {
		return errors.New("a name to resolve is required for a DNS check")
	}
	n = strings.TrimSuffix(n, ".")
	bad := errors.New("the name to resolve must look like example.com or _dmarc.example.com")
	if len(n) > maxDNSName {
		return bad
	}
	for _, l := range strings.Split(n, ".") {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return bad
		}
		for _, c := range l {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return bad
			}
		}
	}
	return nil
}

func checkDNS(ctx context.Context, addr string, s Service, timeout time.Duration) Outcome {
	qtype := dnsTypeCode[s.RType]
	name := strings.TrimSuffix(s.Host, ".")
	id := uint16(rand.Uint32())
	q := dnsQuery(id, name, qtype)
	start := time.Now()
	rep, err := dnsUDP(ctx, addr, s.Port, q, id, timeout)
	via := ""
	if err == nil && rep.tc {
		rep, err = dnsTCP(ctx, addr, s.Port, q, id, timeout)
		via = " (TCP)"
	}
	rtt := time.Since(start)
	if err != nil {
		return Outcome{Failed: StepQuery, Err: err}
	}
	if rep.rcode != 0 {
		why := dnsRcodes[rep.rcode]
		if why == "" {
			why = "error code " + strconv.Itoa(rep.rcode)
		}
		return Outcome{RTT: rtt, Failed: StepRcode, Err: errors.New("the server answered: " + why)}
	}
	var vals []string
	for _, a := range rep.answers {
		if a.typ == qtype {
			vals = append(vals, a.val)
		}
	}
	if len(vals) == 0 {
		return Outcome{RTT: rtt, Failed: StepRecords, Err: fmt.Errorf("no %s record for %s", s.RType, name)}
	}
	detail := s.RType + " " + strings.Join(vals, ", ") + via
	if s.Expect != "" {
		want, found := strings.ToLower(s.Expect), false
		for _, v := range vals {
			if strings.Contains(strings.ToLower(v), want) {
				found = true
				break
			}
		}
		if !found {
			return Outcome{RTT: rtt, Detail: detail, Failed: StepExpect, Err: fmt.Errorf("no %s record contains %q", s.RType, s.Expect)}
		}
	}
	if s.MaxMs > 0 && rtt > time.Duration(s.MaxMs)*time.Millisecond {
		return Outcome{RTT: rtt, Detail: detail, Failed: StepTime,
			Err: fmt.Errorf("answered in %d ms (at most %d ms)", rtt.Milliseconds(), s.MaxMs)}
	}
	return Outcome{RTT: rtt, Detail: detail}
}

// dnsQuery builds a query: recursion desired, one question, an EDNS0 OPT record.
func dnsQuery(id uint16, name string, qtype uint16) []byte {
	b := make([]byte, 12, 64+len(name))
	binary.BigEndian.PutUint16(b[0:], id)
	b[2] = 0x01                           // RD
	binary.BigEndian.PutUint16(b[4:], 1)  // one question
	binary.BigEndian.PutUint16(b[10:], 1) // one additional record: OPT
	for _, l := range strings.Split(name, ".") {
		b = append(b, byte(len(l)))
		b = append(b, l...)
	}
	b = append(b, 0)
	b = binary.BigEndian.AppendUint16(b, qtype)
	b = binary.BigEndian.AppendUint16(b, 1) // class IN
	b = append(b, 0)                        // OPT: root name
	b = binary.BigEndian.AppendUint16(b, 41)
	b = binary.BigEndian.AppendUint16(b, dnsUDPSize)
	return append(b, 0, 0, 0, 0, 0, 0) // extended rcode and flags, no options
}

func dnsDeadline(ctx context.Context, timeout time.Duration) time.Time {
	d := time.Now().Add(timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(d) {
		d = dl
	}
	return d
}

func dnsNetErr(err error, timeout time.Duration) error {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return fmt.Errorf("no answer within %s", timeout)
	}
	return dialError(err)
}

func dnsUDP(ctx context.Context, addr string, port int, q []byte, id uint16, timeout time.Duration) (*dnsReply, error) {
	d := net.Dialer{Timeout: timeout}
	c, err := d.DialContext(ctx, "udp", net.JoinHostPort(addr, strconv.Itoa(port)))
	if err != nil {
		return nil, dialError(err)
	}
	defer c.Close()
	c.SetDeadline(dnsDeadline(ctx, timeout))
	if _, err := c.Write(q); err != nil {
		return nil, dnsNetErr(err, timeout)
	}
	buf := make([]byte, 65535)
	for {
		n, err := c.Read(buf)
		if err != nil {
			return nil, dnsNetErr(err, timeout)
		}
		if n < 2 || binary.BigEndian.Uint16(buf) != id {
			continue // not the answer to this query
		}
		return parseDNS(buf[:n])
	}
}

func dnsTCP(ctx context.Context, addr string, port int, q []byte, id uint16, timeout time.Duration) (*dnsReply, error) {
	c, err := dial(ctx, addr, port, timeout)
	if err != nil {
		return nil, fmt.Errorf("truncated over UDP, and over TCP: %w", err)
	}
	defer c.Close()
	c.SetDeadline(dnsDeadline(ctx, timeout))
	msg := binary.BigEndian.AppendUint16(make([]byte, 0, 2+len(q)), uint16(len(q)))
	if _, err := c.Write(append(msg, q...)); err != nil {
		return nil, dnsNetErr(err, timeout)
	}
	var hdr [2]byte
	if _, err := io.ReadFull(c, hdr[:]); err != nil {
		return nil, dnsNetErr(err, timeout)
	}
	buf := make([]byte, binary.BigEndian.Uint16(hdr[:]))
	if _, err := io.ReadFull(c, buf); err != nil {
		return nil, dnsNetErr(err, timeout)
	}
	rep, err := parseDNS(buf)
	if err == nil && rep.id != id {
		err = errors.New("the answer over TCP does not belong to the query")
	}
	return rep, err
}

type dnsReply struct {
	id      uint16
	tc      bool
	rcode   int
	answers []dnsRR
}

type dnsRR struct {
	typ uint16
	val string
}

// parseDNS reads an answer: the header and the answer records of the
// types a check can ask for (others are skipped).
func parseDNS(m []byte) (*dnsReply, error) {
	if len(m) < 12 || m[2]&0x80 == 0 {
		return nil, errDNSBroken
	}
	r := &dnsReply{id: binary.BigEndian.Uint16(m), tc: m[2]&0x02 != 0, rcode: int(m[3] & 0x0f)}
	if r.tc {
		return r, nil // repeated over TCP; the rest may be cut off
	}
	qd, an := int(binary.BigEndian.Uint16(m[4:])), int(binary.BigEndian.Uint16(m[6:]))
	off := 12
	var err error
	for i := 0; i < qd; i++ {
		if _, off, err = readName(m, off); err != nil {
			return nil, err
		}
		if off += 4; off > len(m) {
			return nil, errDNSBroken
		}
	}
	for i := 0; i < an; i++ {
		if _, off, err = readName(m, off); err != nil {
			return nil, err
		}
		if off+10 > len(m) {
			return nil, errDNSBroken
		}
		typ, rdlen := binary.BigEndian.Uint16(m[off:]), int(binary.BigEndian.Uint16(m[off+8:]))
		off += 10
		if off+rdlen > len(m) {
			return nil, errDNSBroken
		}
		v, err := rdataText(m, typ, off, rdlen)
		if err != nil {
			return nil, err
		}
		if v != "" {
			r.answers = append(r.answers, dnsRR{typ, v})
		}
		off += rdlen
	}
	return r, nil
}

// rdataText: a record's value as the website shows it and the expected
// value is matched against ("" for types a check does not ask for).
func rdataText(m []byte, typ uint16, off, n int) (string, error) {
	rd := m[off : off+n]
	switch typ {
	case 1, 28: // A, AAAA
		if (typ == 1 && n != 4) || (typ == 28 && n != 16) {
			return "", errDNSBroken
		}
		return net.IP(rd).String(), nil
	case 2, 5: // NS, CNAME
		name, _, err := readName(m, off)
		return name, err
	case 15: // MX
		if n < 3 {
			return "", errDNSBroken
		}
		name, _, err := readName(m, off+2)
		return strconv.Itoa(int(binary.BigEndian.Uint16(rd))) + " " + name, err
	case 16: // TXT: its pieces joined
		var sb strings.Builder
		for i := 0; i < n; {
			l := int(rd[i])
			if i+1+l > n {
				return "", errDNSBroken
			}
			sb.Write(rd[i+1 : i+1+l])
			i += 1 + l
		}
		return sb.String(), nil
	}
	return "", nil
}

// readName reads a (possibly compressed) name at off; it returns the name
// without a trailing dot and the offset after it.
func readName(m []byte, off int) (string, int, error) {
	var labels []string
	end, jumps := -1, 0
	for {
		if off >= len(m) {
			return "", 0, errDNSBroken
		}
		l := int(m[off])
		switch {
		case l == 0:
			if end < 0 {
				end = off + 1
			}
			return strings.Join(labels, "."), end, nil
		case l&0xc0 == 0xc0:
			if off+1 >= len(m) || jumps >= maxDNSJumps {
				return "", 0, errDNSBroken
			}
			jumps++
			if end < 0 {
				end = off + 2
			}
			off = int(binary.BigEndian.Uint16(m[off:]) & 0x3fff)
		case l&0xc0 != 0:
			return "", 0, errDNSBroken
		default:
			if off+1+l > len(m) || len(labels) >= 128 {
				return "", 0, errDNSBroken
			}
			labels = append(labels, string(m[off+1:off+1+l]))
			off += 1 + l
		}
	}
}
