package master

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

func TestTOTP(t *testing.T) {
	// RFC 6238 test key (SHA-1), last six digits of the 8-digit vectors.
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for _, c := range []struct {
		t    int64
		want string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}} {
		if got := totpCode(secret, c.t/30); got != c.want {
			t.Errorf("t=%d: %s, want %s", c.t, got, c.want)
		}
	}
	now := time.Unix(1111111109, 0)
	if s := totpCheck(secret, "081804", now, 0); s != 1111111109/30 {
		t.Errorf("valid code refused: %d", s)
	}
	if s := totpCheck(secret, " 081 804 ", now.Add(25*time.Second), 0); s < 0 {
		t.Error("code from the previous step refused")
	}
	if s := totpCheck(secret, "081804", now, 1111111109/30); s >= 0 {
		t.Error("used code accepted again")
	}
	if s := totpCheck(secret, "081804", now.Add(2*time.Minute), 0); s >= 0 {
		t.Error("old code accepted")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if totpCheck(secret, bad, now, 0) >= 0 {
			t.Errorf("%q accepted", bad)
		}
	}
	if u := totpURI("ad min", "ABC"); !strings.HasPrefix(u, "otpauth://totp/Coremon:ad%20min?secret=ABC&issuer=Coremon&algorithm=SHA1&digits=6&period=30") {
		t.Errorf("uri %s", u)
	}
	if len(newTOTPSecret()) != 32 {
		t.Error("secret length")
	}
}

func TestQRParts(t *testing.T) {
	// Reed-Solomon: "HELLO WORLD", version 1-M (known codewords).
	data := []byte{32, 91, 11, 120, 209, 114, 220, 77, 67, 64, 236, 17, 236, 17, 236, 17}
	want := []byte{196, 35, 39, 119, 235, 215, 231, 226, 93, 23}
	got := rsRemainder(data, rsDivisor(10))
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("EC codewords %v, want %v", got, want)
		}
	}
	if b := qrFormatBits(0); b != 0b101010000010010 {
		t.Errorf("format bits M/0: %015b", b)
	}
	if b := qrFormatBits(5); b != 0b100000011001110 {
		t.Errorf("format bits M/5: %015b", b)
	}
	if b := qrVersionBits(7); b != 0b000111110010010100 {
		t.Errorf("version 7 bits: %018b", b)
	}
	for _, v := range qrVersions[1:] {
		if v.dataLen() == 0 {
			t.Fatal("empty version")
		}
	}
	rows, err := qrEncode(totpURI("administrator-with-a-long-name", newTOTPSecret()))
	if err != nil || len(rows) < 21 || len(rows) != len(rows[0]) {
		t.Fatalf("encode: %v %d", err, len(rows))
	}
	// Finder pattern in three corners.
	n := len(rows)
	for _, c := range [][2]int{{0, 0}, {n - 7, 0}, {0, n - 7}} {
		if rows[c[1]][c[0]:c[0]+7] != "1111111" || rows[c[1]+1][c[0]:c[0]+7] != "1000001" || rows[c[1]+3][c[0]:c[0]+7] != "1011101" {
			t.Errorf("finder at %v", c)
		}
	}
	if _, err := qrEncode(strings.Repeat("x", 300)); err == nil {
		t.Error("too long text accepted")
	}
}

// TestQRReadBack decodes generated codes as a scanner would: format bits
// (both copies), unmask, read codewords, check the error correction and
// recover the text.
func TestQRReadBack(t *testing.T) {
	for _, text := range []string{"A", "HELLO WORLD", totpURI("admin", newTOTPSecret()), totpURI(strings.Repeat("u", 32), newTOTPSecret()), strings.Repeat("x", 200)} {
		rows, err := qrEncode(text)
		if err != nil {
			t.Fatal(err)
		}
		n := len(rows)
		ver := (n - 17) / 4
		at := func(x, y int) bool { return rows[y][x] == '1' }
		// Format bits, first copy (around the top-left finder).
		var f1, f2 int
		pos1 := [][2]int{{8, 0}, {8, 1}, {8, 2}, {8, 3}, {8, 4}, {8, 5}, {8, 7}, {8, 8}, {7, 8}, {5, 8}, {4, 8}, {3, 8}, {2, 8}, {1, 8}, {0, 8}}
		for i, p := range pos1 {
			if at(p[0], p[1]) {
				f1 |= 1 << uint(i)
			}
		}
		for i := 0; i < 8; i++ {
			if at(n-1-i, 8) {
				f2 |= 1 << uint(i)
			}
		}
		for i := 8; i < 15; i++ {
			if at(8, n-15+i) {
				f2 |= 1 << uint(i)
			}
		}
		mask := -1
		for m := 0; m < 8; m++ {
			if qrFormatBits(m) == f1 {
				mask = m
			}
		}
		if mask < 0 || f1 != f2 || !at(8, n-8) {
			t.Fatalf("%q: format bits %015b / %015b", text, f1, f2)
		}
		// Unmask and read the codewords in placement order.
		fn := newQRMatrix(ver).fn
		var cw []byte
		var cur byte
		bitsRead := 0
		for right := n - 1; right >= 1; right -= 2 {
			if right == 6 {
				right = 5
			}
			for vert := 0; vert < n; vert++ {
				for j := 0; j < 2; j++ {
					x, y := right-j, vert
					if (right+1)&2 == 0 {
						y = n - 1 - vert
					}
					if fn[y][x] {
						continue
					}
					b := at(x, y) != qrMaskBit(mask, x, y)
					cur <<= 1
					if b {
						cur |= 1
					}
					if bitsRead++; bitsRead%8 == 0 {
						cw = append(cw, cur)
						cur = 0
					}
				}
			}
		}
		v := qrVersions[ver]
		total := v.dataLen() + v.ec*(v.g1+v.g2)
		if len(cw) < total {
			t.Fatalf("%q: %d codewords, want %d", text, len(cw), total)
		}
		// De-interleave and check every block's error correction.
		sizes := []int{}
		for i := 0; i < v.g1; i++ {
			sizes = append(sizes, v.g1n)
		}
		for i := 0; i < v.g2; i++ {
			sizes = append(sizes, v.g2n)
		}
		blocks := make([][]byte, len(sizes))
		k := 0
		for i := 0; i < max(v.g1n, v.g2n); i++ {
			for b := range blocks {
				if i < sizes[b] {
					blocks[b] = append(blocks[b], cw[k])
					k++
				}
			}
		}
		var data []byte
		div := rsDivisor(v.ec)
		ecs := make([][]byte, len(sizes))
		for i := 0; i < v.ec; i++ {
			for b := range blocks {
				ecs[b] = append(ecs[b], cw[k])
				k++
			}
		}
		for b := range blocks {
			if got := rsRemainder(blocks[b], div); string(got) != string(ecs[b]) {
				t.Fatalf("%q: block %d error correction does not match", text, b)
			}
			data = append(data, blocks[b]...)
		}
		// Byte mode: 0100, length, bytes.
		bit := func(i int) int { return int(data[i/8]>>uint(7-i%8)) & 1 }
		read := func(pos, n int) (int, int) {
			v := 0
			for i := 0; i < n; i++ {
				v = v<<1 | bit(pos+i)
			}
			return v, pos + n
		}
		mode, p := read(0, 4)
		lenBits := 8
		if ver > 9 {
			lenBits = 16
		}
		ln, p := read(p, lenBits)
		out := make([]byte, ln)
		for i := range out {
			var c int
			c, p = read(p, 8)
			out[i] = byte(c)
		}
		if mode != 4 || string(out) != text {
			t.Fatalf("read back %q (mode %d), want %q", out, mode, text)
		}
		t.Logf("%d chars: version %d, mask %d, %dx%d", len(text), ver, mask, n, n)
	}
}
