package master

// QR code generator (byte mode, error correction M, versions 1-10).

import (
	"errors"
	"strings"
)

type qrVersion struct {
	ec           int // EC codewords per block
	g1, g1n      int // blocks and data codewords per block, group 1
	g2, g2n      int // group 2
	align        []int
	remainderBit int
}

var qrVersions = []qrVersion{
	{},
	{10, 1, 16, 0, 0, nil, 0},
	{16, 1, 28, 0, 0, []int{6, 18}, 7},
	{26, 1, 44, 0, 0, []int{6, 22}, 7},
	{18, 2, 32, 0, 0, []int{6, 26}, 7},
	{24, 2, 43, 0, 0, []int{6, 30}, 7},
	{16, 4, 27, 0, 0, []int{6, 34}, 7},
	{18, 4, 31, 0, 0, []int{6, 22, 38}, 0},
	{22, 2, 38, 2, 39, []int{6, 24, 42}, 0},
	{22, 3, 36, 2, 37, []int{6, 26, 46}, 0},
	{26, 4, 43, 1, 44, []int{6, 28, 50}, 0},
}

func (v qrVersion) dataLen() int { return v.g1*v.g1n + v.g2*v.g2n }

func gfMul(x, y byte) byte {
	z := 0
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x11d)
		z ^= int((y>>uint(i))&1) * int(x)
	}
	return byte(z)
}

func rsDivisor(degree int) []byte {
	r := make([]byte, degree)
	r[degree-1] = 1
	root := byte(1)
	for i := 0; i < degree; i++ {
		for j := 0; j < degree; j++ {
			r[j] = gfMul(r[j], root)
			if j+1 < degree {
				r[j] ^= r[j+1]
			}
		}
		root = gfMul(root, 2)
	}
	return r
}

func rsRemainder(data, div []byte) []byte {
	r := make([]byte, len(div))
	for _, b := range data {
		f := b ^ r[0]
		copy(r, r[1:])
		r[len(r)-1] = 0
		for i := range r {
			r[i] ^= gfMul(div[i], f)
		}
	}
	return r
}

// qrCodewords builds the data and error correction codewords, interleaved.
func qrCodewords(data []byte, ver int) []byte {
	v := qrVersions[ver]
	var bits []bool
	put := func(val, n int) {
		for i := n - 1; i >= 0; i-- {
			bits = append(bits, (val>>uint(i))&1 == 1)
		}
	}
	put(4, 4)
	if ver <= 9 {
		put(len(data), 8)
	} else {
		put(len(data), 16)
	}
	for _, b := range data {
		put(int(b), 8)
	}
	capBits := v.dataLen() * 8
	for i := 0; i < 4 && len(bits) < capBits; i++ {
		bits = append(bits, false)
	}
	for len(bits)%8 != 0 {
		bits = append(bits, false)
	}
	var cw []byte
	for i := 0; i < len(bits); i += 8 {
		var b byte
		for j := 0; j < 8; j++ {
			if bits[i+j] {
				b |= 1 << uint(7-j)
			}
		}
		cw = append(cw, b)
	}
	for pad := byte(0xEC); len(cw) < v.dataLen(); pad ^= 0xEC ^ 0x11 {
		cw = append(cw, pad)
	}
	var blocks, ecs [][]byte
	div := rsDivisor(v.ec)
	pos := 0
	for g, n := range []int{v.g1, v.g2} {
		size := []int{v.g1n, v.g2n}[g]
		for i := 0; i < n; i++ {
			blk := cw[pos : pos+size]
			pos += size
			blocks = append(blocks, blk)
			ecs = append(ecs, rsRemainder(blk, div))
		}
	}
	var out []byte
	for i := 0; i < v.g2n || i < v.g1n; i++ {
		for _, b := range blocks {
			if i < len(b) {
				out = append(out, b[i])
			}
		}
	}
	for i := 0; i < v.ec; i++ {
		for _, e := range ecs {
			out = append(out, e[i])
		}
	}
	return out
}

type qrMatrix struct {
	size int
	dark [][]bool
	fn   [][]bool
}

func (m *qrMatrix) set(x, y int, d bool) { m.dark[y][x], m.fn[y][x] = d, true }

func qrFormatBits(mask int) int {
	data := 0<<3 | mask // level M
	rem := data
	for i := 0; i < 10; i++ {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	return (data<<10 | rem) ^ 0x5412
}

func qrVersionBits(ver int) int {
	rem := ver
	for i := 0; i < 12; i++ {
		rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
	}
	return ver<<12 | rem
}

func (m *qrMatrix) drawFormat(mask int) {
	bits := qrFormatBits(mask)
	bit := func(i int) bool { return (bits>>uint(i))&1 == 1 }
	n := m.size
	for i := 0; i <= 5; i++ {
		m.set(8, i, bit(i))
	}
	m.set(8, 7, bit(6))
	m.set(8, 8, bit(7))
	m.set(7, 8, bit(8))
	for i := 9; i < 15; i++ {
		m.set(14-i, 8, bit(i))
	}
	for i := 0; i < 8; i++ {
		m.set(n-1-i, 8, bit(i))
	}
	for i := 8; i < 15; i++ {
		m.set(8, n-15+i, bit(i))
	}
	m.set(8, n-8, true)
}

func newQRMatrix(ver int) *qrMatrix {
	n := 17 + 4*ver
	m := &qrMatrix{size: n, dark: make([][]bool, n), fn: make([][]bool, n)}
	for i := range m.dark {
		m.dark[i], m.fn[i] = make([]bool, n), make([]bool, n)
	}
	for i := 0; i < n; i++ {
		m.set(6, i, i%2 == 0)
		m.set(i, 6, i%2 == 0)
	}
	for _, c := range [][2]int{{3, 3}, {n - 4, 3}, {3, n - 4}} {
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				x, y := c[0]+dx, c[1]+dy
				if x >= 0 && x < n && y >= 0 && y < n {
					d := max(abs(dx), abs(dy))
					m.set(x, y, d != 2 && d != 4)
				}
			}
		}
	}
	al := qrVersions[ver].align
	for i := range al {
		for j := range al {
			last := len(al) - 1
			if (i == 0 && j == 0) || (i == 0 && j == last) || (i == last && j == 0) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					m.set(al[i]+dx, al[j]+dy, max(abs(dx), abs(dy)) != 1)
				}
			}
		}
	}
	m.drawFormat(0)
	if ver >= 7 {
		bits := qrVersionBits(ver)
		for i := 0; i < 18; i++ {
			d := (bits>>uint(i))&1 == 1
			a, b := n-11+i%3, i/3
			m.set(a, b, d)
			m.set(b, a, d)
		}
	}
	return m
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (m *qrMatrix) drawData(cw []byte) {
	n, i := m.size, 0
	for right := n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := 0; vert < n; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				y := vert
				if (right+1)&2 == 0 {
					y = n - 1 - vert
				}
				if !m.fn[y][x] && i < len(cw)*8 {
					m.dark[y][x] = (cw[i>>3]>>uint(7-i&7))&1 == 1
					i++
				}
			}
		}
	}
}

func qrMaskBit(mask, x, y int) bool {
	switch mask {
	case 0:
		return (x+y)%2 == 0
	case 1:
		return y%2 == 0
	case 2:
		return x%3 == 0
	case 3:
		return (x+y)%3 == 0
	case 4:
		return (x/3+y/2)%2 == 0
	case 5:
		return x*y%2+x*y%3 == 0
	case 6:
		return (x*y%2+x*y%3)%2 == 0
	default:
		return ((x+y)%2+x*y%3)%2 == 0
	}
}

func (m *qrMatrix) applyMask(mask int) {
	for y := 0; y < m.size; y++ {
		for x := 0; x < m.size; x++ {
			if !m.fn[y][x] && qrMaskBit(mask, x, y) {
				m.dark[y][x] = !m.dark[y][x]
			}
		}
	}
}

func (m *qrMatrix) penalty() int {
	n, p := m.size, 0
	line := func(get func(i int) bool) {
		run, prev := 0, false
		var s strings.Builder
		for i := 0; i < n; i++ {
			d := get(i)
			if i > 0 && d == prev {
				run++
			} else {
				if run >= 5 {
					p += run - 2
				}
				run = 1
			}
			prev = d
			s.WriteByte(map[bool]byte{true: '1', false: '0'}[d])
		}
		if run >= 5 {
			p += run - 2
		}
		row := s.String()
		p += 40 * (strings.Count(row, "10111010000") + strings.Count(row, "00001011101"))
	}
	for y := 0; y < n; y++ {
		line(func(i int) bool { return m.dark[y][i] })
	}
	for x := 0; x < n; x++ {
		line(func(i int) bool { return m.dark[i][x] })
	}
	dark := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if m.dark[y][x] {
				dark++
			}
			if x < n-1 && y < n-1 {
				c := m.dark[y][x]
				if m.dark[y][x+1] == c && m.dark[y+1][x] == c && m.dark[y+1][x+1] == c {
					p += 3
				}
			}
		}
	}
	p += 10 * (abs(dark*100/(n*n)-50) / 5)
	return p
}

// qrEncode returns the QR code of text as rows of '1' (dark) and '0'.
func qrEncode(text string) ([]string, error) {
	data := []byte(text)
	ver := 0
	for v := 1; v < len(qrVersions); v++ {
		hdr := 2
		if v > 9 {
			hdr = 3
		}
		if len(data)+hdr <= qrVersions[v].dataLen() {
			ver = v
			break
		}
	}
	if ver == 0 {
		return nil, errors.New("text too long for a QR code")
	}
	cw := qrCodewords(data, ver)
	best, bestP := -1, 0
	var bestM *qrMatrix
	for mask := 0; mask < 8; mask++ {
		m := newQRMatrix(ver)
		m.drawData(cw)
		m.applyMask(mask)
		m.drawFormat(mask)
		if p := m.penalty(); best < 0 || p < bestP {
			best, bestP, bestM = mask, p, m
		}
	}
	rows := make([]string, bestM.size)
	for y := range rows {
		var b strings.Builder
		for x := 0; x < bestM.size; x++ {
			b.WriteByte(map[bool]byte{true: '1', false: '0'}[bestM.dark[y][x]])
		}
		rows[y] = b.String()
	}
	return rows, nil
}
