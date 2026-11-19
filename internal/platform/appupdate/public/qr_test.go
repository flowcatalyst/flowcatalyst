package public

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The install page's QR code must open the page on a phone. These tests read
// the drawing back with a decoder written here from ISO/IEC 18004, not with
// the encoder that drew it: format and version information checked by their
// BCH codes, every block's Reed-Solomon syndromes zero, the segments parsed.

const qrTestToken = "BKEO5yRKKAW-oT4UmeLTYCKytf4shICj-kqd_Fjx2J4"

func TestQRSVGDecodesToTheText(t *testing.T) {
	t.Parallel()
	token := qrTestToken
	long := "https://platform.example.com/app-updates/acme/driver-app/install/" + token
	cases := []struct {
		name, text string
		version    int
	}{
		{"short", "https://platform.example.com/x", 3},
		{"install address", long, 7},
		{"version 10", long + "?" + strings.Repeat("abcdefghij", 9), 10},
		{"alphanumeric", "HTTPS://PLATFORM.EXAMPLE.COM/ACME", 2},
		{"numeric", "0123456789012345678901234567890", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			svg, err := qrSVG(c.text, "QR code of this page")
			require.NoError(t, err)
			m := matrixOfSVG(t, svg)
			got, version, err := decodeQR(m)
			require.NoError(t, err)
			assert.Equal(t, c.text, got)
			assert.Equal(t, c.version, version)
		})
	}
}

func TestQRSVGMarkup(t *testing.T) {
	t.Parallel()
	token := qrTestToken
	text := "https://platform.example.com/app-updates/acme/driver-app/install/" + token
	svg, err := qrSVG(text, `Scan & open <here>`)
	require.NoError(t, err)
	for _, leak := range []string{"platform.example.com", "example", "acme", "driver-app", "install", token, token[:8]} {
		assert.NotContains(t, svg, leak)
	}
	assert.Contains(t, svg, `aria-label="Scan &amp; open &lt;here&gt;"`)
	assert.Contains(t, svg, `role="img"`)
	assert.Contains(t, svg, `width="240" height="240"`)
	assert.Contains(t, svg, `shape-rendering="crispEdges"`)
	assert.Regexp(t, `<rect width="\d+" height="\d+" fill="#fff"/>`, svg)
	assert.NotContains(t, svg, "class=")
	assert.NotContains(t, svg, "style=")
	assert.Equal(t, 1, strings.Count(svg, "<path "))
}

// The decoder is not vacuous: one flipped data module breaks a syndrome.
func TestQRDecoderRejectsAFlippedModule(t *testing.T) {
	t.Parallel()
	svg, err := qrSVG("https://platform.example.com/app-updates/acme/driver-app/install/"+qrTestToken, "QR")
	require.NoError(t, err)
	m := matrixOfSVG(t, svg)
	_, _, err = decodeQR(m)
	require.NoError(t, err)

	fn := qrFunctionModules(len(m), (len(m)-17)/4)
	// The first data module in reading order: bottom-right corner.
	n := len(m)
	require.False(t, fn[n-1][n-1])
	m[n-1][n-1] = !m[n-1][n-1]
	_, _, err = decodeQR(m)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "syndrome")
}

var (
	qrViewBox = regexp.MustCompile(`viewBox="0 0 (\d+) (\d+)"`)
	qrSquare  = regexp.MustCompile(`M(\d+) (\d+)h1v1h-1z`)
)

// matrixOfSVG is the module matrix of a drawing, quiet zone removed; [y][x],
// true is dark.
func matrixOfSVG(t *testing.T, svg string) [][]bool {
	t.Helper()
	vb := qrViewBox.FindStringSubmatch(svg)
	require.NotNil(t, vb, "no viewBox")
	size, _ := strconv.Atoi(vb[1])
	const quiet = 4
	n := size - 2*quiet
	require.True(t, n >= 21 && (n-17)%4 == 0, "%d modules is not a QR code size", n)
	m := make([][]bool, n)
	for i := range m {
		m[i] = make([]bool, n)
	}
	for _, sq := range qrSquare.FindAllStringSubmatch(svg, -1) {
		x, _ := strconv.Atoi(sq[1])
		y, _ := strconv.Atoi(sq[2])
		x, y = x-quiet, y-quiet
		require.True(t, x >= 0 && y >= 0 && x < n && y < n, "a dark module in the quiet zone at (%d,%d)", x+quiet, y+quiet)
		m[y][x] = true
	}
	return m
}

// ── decoder (ISO/IEC 18004) ────────────────────────────────────────────────

// qrBlocksM is, for level M, versions 1..10: total codewords, error
// correction codewords per block, number of blocks.
var qrBlocksM = map[int][3]int{
	1: {26, 10, 1}, 2: {44, 16, 1}, 3: {70, 26, 1}, 4: {100, 18, 2}, 5: {134, 24, 2},
	6: {172, 16, 4}, 7: {196, 18, 4}, 8: {242, 22, 4}, 9: {292, 22, 5}, 10: {346, 26, 5},
}

// qrAlignment is the alignment pattern centre coordinates, versions 1..10.
var qrAlignment = map[int][]int{
	1: nil, 2: {6, 18}, 3: {6, 22}, 4: {6, 26}, 5: {6, 30},
	6: {6, 34}, 7: {6, 22, 38}, 8: {6, 24, 42}, 9: {6, 26, 46}, 10: {6, 28, 50},
}

func decodeQR(m [][]bool) (string, int, error) {
	n := len(m)
	version := (n - 17) / 4
	blocks, ok := qrBlocksM[version]
	if !ok {
		return "", version, fmt.Errorf("version %d is beyond this decoder's table (1-10)", version)
	}
	at := func(x, y int) bool { return m[y][x] }

	// Format information, both copies.
	var f1, f2 int
	bit := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	for i := range 6 {
		f1 |= bit(at(8, i)) << i
	}
	f1 |= bit(at(8, 7)) << 6
	f1 |= bit(at(8, 8)) << 7
	f1 |= bit(at(7, 8)) << 8
	for i := 9; i < 15; i++ {
		f1 |= bit(at(14-i, 8)) << i
	}
	for i := range 8 {
		f2 |= bit(at(n-1-i, 8)) << i
	}
	for i := 8; i < 15; i++ {
		f2 |= bit(at(8, n-15+i)) << i
	}
	if f1 != f2 {
		return "", version, fmt.Errorf("the two copies of the format information differ: %015b, %015b", f1, f2)
	}
	if !at(8, n-8) {
		return "", version, errors.New("no dark module")
	}
	format := f1 ^ 0x5412
	data := format >> 10
	rem := data
	for range 10 {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	if data<<10|rem != format {
		return "", version, fmt.Errorf("format information %015b fails its BCH check", f1)
	}
	if level := data >> 3; level != 0 {
		return "", version, fmt.Errorf("error correction level bits %02b, want 00 (M)", level)
	}
	mask := data & 7

	// Version information, both copies.
	if version >= 7 {
		var v1, v2 int
		for i := range 18 {
			a, b := n-11+i%3, i/3
			v1 |= bit(at(a, b)) << i
			v2 |= bit(at(b, a)) << i
		}
		if v1 != v2 {
			return "", version, errors.New("the two copies of the version information differ")
		}
		r := v1 >> 12
		vr := r
		for range 12 {
			vr = (vr << 1) ^ ((vr >> 11) * 0x1f25)
		}
		if r != version || r<<12|vr != v1 {
			return "", version, fmt.Errorf("version information %018b does not check for version %d", v1, version)
		}
	}

	// Codewords, zigzag, unmasked.
	fn := qrFunctionModules(n, version)
	total := blocks[0]
	codewords := make([]byte, total)
	i := 0
	for right := n - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5
		}
		for vert := range n {
			for j := range 2 {
				x := right - j
				y := vert
				if (right+1)&2 == 0 {
					y = n - 1 - vert
				}
				if fn[y][x] || i >= total*8 {
					continue
				}
				v := m[y][x] != qrMasked(mask, x, y)
				if v {
					codewords[i>>3] |= 1 << (7 - i&7)
				}
				i++
			}
		}
	}
	if i != total*8 {
		return "", version, fmt.Errorf("read %d bits, want %d", i, total*8)
	}

	// De-interleave and check every block.
	ecc, nb := blocks[1], blocks[2]
	short := nb - total%nb
	shortLen := total / nb
	shortData := shortLen - ecc
	bl := make([][]byte, nb)
	for j := range bl {
		bl[j] = make([]byte, shortLen+1)
	}
	k := 0
	for idx := 0; idx <= shortLen; idx++ {
		for j := range nb {
			if idx == shortData && j < short {
				continue
			}
			bl[j][idx] = codewords[k]
			k++
		}
	}
	var dataBytes []byte
	for j := range bl {
		b := bl[j]
		if j < short {
			b = append(b[:shortData:shortData], b[shortData+1:]...)
		}
		for s := range ecc {
			if syn := qrSyndrome(b, s); syn != 0 {
				return "", version, fmt.Errorf("block %d: Reed-Solomon syndrome %d is %d, not 0", j, s, syn)
			}
		}
		dataBytes = append(dataBytes, b[:len(b)-ecc]...)
	}
	text, err := qrSegments(dataBytes, version)
	return text, version, err
}

// qrFunctionModules marks the modules that carry no data.
func qrFunctionModules(n, version int) [][]bool {
	fn := make([][]bool, n)
	for i := range fn {
		fn[i] = make([]bool, n)
	}
	set := func(x, y int) {
		if x >= 0 && y >= 0 && x < n && y < n {
			fn[y][x] = true
		}
	}
	for i := range n {
		set(6, i)
		set(i, 6)
	}
	for _, c := range [][2]int{{3, 3}, {n - 4, 3}, {3, n - 4}} {
		for dy := -4; dy <= 4; dy++ {
			for dx := -4; dx <= 4; dx++ {
				set(c[0]+dx, c[1]+dy)
			}
		}
	}
	pos := qrAlignment[version]
	last := len(pos) - 1
	for a, cx := range pos {
		for b, cy := range pos {
			if (a == 0 && b == 0) || (a == 0 && b == last) || (a == last && b == 0) {
				continue
			}
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					set(cx+dx, cy+dy)
				}
			}
		}
	}
	for i := range 9 {
		set(8, i)
		set(i, 8)
	}
	for i := range 8 {
		set(n-1-i, 8)
		set(8, n-1-i)
	}
	if version >= 7 {
		for i := range 18 {
			a, b := n-11+i%3, i/3
			set(a, b)
			set(b, a)
		}
	}
	return fn
}

func qrMasked(mask, x, y int) bool {
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

var qrExp, qrLog = func() ([512]byte, [256]byte) {
	var e [512]byte
	var l [256]byte
	x := 1
	for i := range 255 {
		e[i] = byte(x)
		l[x] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11d
		}
	}
	for i := 255; i < 512; i++ {
		e[i] = e[i-255]
	}
	return e, l
}()

func qrMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return qrExp[int(qrLog[a])+int(qrLog[b])]
}

// qrSyndrome evaluates the block, highest-degree coefficient first, at α^s.
func qrSyndrome(block []byte, s int) byte {
	alpha := qrExp[s%255]
	var acc byte
	for _, c := range block {
		acc = qrMul(acc, alpha) ^ c
	}
	return acc
}

const qrAlnum = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

func qrSegments(data []byte, version int) (string, error) {
	pos := 0
	read := func(nbits int) (int, bool) {
		if pos+nbits > len(data)*8 {
			return 0, false
		}
		v := 0
		for range nbits {
			v = v<<1 | int(data[pos>>3]>>(7-pos&7)&1)
			pos++
		}
		return v, true
	}
	wide := version >= 10
	var out strings.Builder
	for {
		mode, ok := read(4)
		if !ok || mode == 0 {
			return out.String(), nil
		}
		switch mode {
		case 4:
			cb := 8
			if wide {
				cb = 16
			}
			count, ok := read(cb)
			if !ok {
				return "", errors.New("byte segment count cut off")
			}
			for range count {
				b, ok := read(8)
				if !ok {
					return "", errors.New("byte segment cut off")
				}
				out.WriteByte(byte(b))
			}
		case 2:
			cb := 9
			if wide {
				cb = 11
			}
			count, ok := read(cb)
			if !ok {
				return "", errors.New("alphanumeric count cut off")
			}
			for ; count >= 2; count -= 2 {
				v, ok := read(11)
				if !ok || v/45 >= 45 {
					return "", errors.New("bad alphanumeric pair")
				}
				out.WriteByte(qrAlnum[v/45])
				out.WriteByte(qrAlnum[v%45])
			}
			if count == 1 {
				v, ok := read(6)
				if !ok || v >= 45 {
					return "", errors.New("bad alphanumeric character")
				}
				out.WriteByte(qrAlnum[v])
			}
		case 1:
			cb := 10
			if wide {
				cb = 12
			}
			count, ok := read(cb)
			if !ok {
				return "", errors.New("numeric count cut off")
			}
			for count > 0 {
				digits, nb := min(count, 3), map[int]int{1: 4, 2: 7, 3: 10}[min(count, 3)]
				v, ok := read(nb)
				if !ok {
					return "", errors.New("numeric group cut off")
				}
				fmt.Fprintf(&out, "%0*d", digits, v)
				count -= digits
			}
		default:
			return "", fmt.Errorf("unexpected segment mode %04b", mode)
		}
	}
}
