package probe

import "testing"

func TestEchoMessages(t *testing.T) {
	m := echoRequest(false, 0x1234, 7)
	if m[0] != 8 || icmpChecksum(m) != 0 {
		t.Errorf("echo request: % x", m)
	}
	reply := append([]byte{}, m...)
	reply[0] = 0
	if !isEchoReply(reply, false, 0x1234, 7, true) || isEchoReply(reply, false, 0x1234, 8, true) || isEchoReply(m, false, 0x1234, 7, true) {
		t.Error("reply matching")
	}
	if !isEchoReply(reply, false, 0, 7, false) {
		t.Error("ping socket: the kernel picks the id")
	}
}
