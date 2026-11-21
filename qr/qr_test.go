package qr

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestGFMultiply(t *testing.T) {
	if gfMultiply(0x53, 0xCA) != 0x8F { // QR polynomial x^8+x^4+x^3+x^2+1
		t.Fatal("GF(256) multiplication is wrong")
	}
}

func TestReedSolomonCodewordIsValid(t *testing.T) {
	data := []byte{0x40, 0xD2, 0xC4, 0x69, 0x5A, 0x10, 0x20}
	numECC := 10
	divisor := reedSolomonGenerator(numECC)
	ecc := reedSolomonRemainder(data, divisor)
	codeword := append(append([]byte(nil), data...), ecc...)

	// A valid RS codeword evaluates to zero at every root of the generator.
	for root := 0; root < numECC; root++ {
		x := byte(1)
		for i := 0; i < root; i++ {
			x = gfMultiply(x, 2)
		}
		value := byte(0)
		for _, b := range codeword {
			value = gfMultiply(value, x) ^ b
		}
		if value != 0 {
			t.Fatalf("syndrome at root %d = %02X, want 00", root, value)
		}
	}
}

func TestBlockInterleavingLength(t *testing.T) {
	for version := 1; version <= maxVersion; version++ {
		for _, ec := range []ECLevel{ECLevelL, ECLevelM, ECLevelQ, ECLevelH} {
			spec := blockSpecFor(version, ec)
			data := make([]byte, spec.dataCodewords())
			for i := range data {
				data[i] = byte(i)
			}
			got := interleave(data, spec)
			if len(got) != spec.totalCodewords {
				t.Fatalf("v%d-%s: %d interleaved codewords, want %d", version, ec, len(got), spec.totalCodewords)
			}
		}
	}
}

func TestKnownCapacities(t *testing.T) {
	cases := []struct {
		version int
		ec      ECLevel
		want    int
	}{
		{1, ECLevelL, 17}, {1, ECLevelM, 14}, {1, ECLevelQ, 11}, {1, ECLevelH, 7},
		{10, ECLevelL, 271}, {10, ECLevelM, 213}, {10, ECLevelQ, 151}, {10, ECLevelH, 119},
	}
	for _, c := range cases {
		if got := byteCapacity(c.version, c.ec); got != c.want {
			t.Errorf("v%d-%s capacity = %d, want %d", c.version, c.ec, got, c.want)
		}
	}
}

func TestEncodeStructure(t *testing.T) {
	m, err := Encode("hello world", Options{EC: ECLevelM})
	if err != nil {
		t.Fatal(err)
	}
	if m.Size != 21 || m.Version != 1 || m.Mask < 0 || m.Mask > 7 {
		t.Fatalf("unexpected header: %+v", m)
	}
	if !m.Get(0, 0) || !m.Get(6, 0) || m.Get(7, 0) || !m.Get(m.Size-4, m.Size-4) || m.Get(m.Size-1, m.Size-1) {
		t.Fatal("finder/timing modules are wrong")
	}
	if !m.Get(8, m.Size-8) {
		t.Fatal("dark module is not dark")
	}
	if m.Get(-1, 0) || m.Get(0, -1) || m.Get(m.Size, 0) {
		t.Fatal("out-of-range Get must return light")
	}
}

func TestAutoVersion(t *testing.T) {
	cases := []struct {
		text    string
		ec      ECLevel
		version int
	}{
		{"hi", ECLevelM, 1},
		{strings.Repeat("a", 14), ECLevelM, 1},
		{strings.Repeat("a", 15), ECLevelM, 2},
		{strings.Repeat("a", 120), ECLevelM, 7},
	}
	for _, c := range cases {
		m, err := Encode(c.text, Options{EC: c.ec})
		if err != nil {
			t.Fatal(err)
		}
		if m.Version != c.version {
			t.Errorf("len %d at %s: version %d, want %d", len(c.text), c.ec, m.Version, c.version)
		}
		if m.Size != 4*c.version+17 {
			t.Errorf("len %d: size %d, want %d", len(c.text), m.Size, 4*c.version+17)
		}
	}
}

func TestEncodeErrors(t *testing.T) {
	if _, err := Encode("x", Options{Version: 11}); err == nil {
		t.Error("version 11 must be rejected")
	}
	if _, err := Encode(strings.Repeat("x", 300), Options{EC: ECLevelH}); err == nil {
		t.Error("oversized payload must be rejected")
	}
	if _, err := Encode("x", Options{EC: ECLevel(99)}); err == nil {
		t.Error("invalid EC level must be rejected")
	}
}

func TestEncodeIsDeterministic(t *testing.T) {
	a, err := Encode("deterministic", Options{EC: ECLevelQ, Version: 4})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encode("deterministic", Options{EC: ECLevelQ, Version: 4})
	if err != nil {
		t.Fatal(err)
	}
	if !matrixEqual(a, b) {
		t.Fatal("same input produced different symbols")
	}
}

func TestDecodeWithZBar(t *testing.T) {
	path, err := exec.LookPath("zbarimg")
	if err != nil {
		t.Skip("zbarimg is not installed")
	}
	cases := []struct {
		text   string
		ec     ECLevel
		ver    int
		invert bool
	}{
		{"hello world", ECLevelL, 0, false},
		{"hello world", ECLevelM, 0, false},
		{"hello world", ECLevelQ, 0, false},
		{"hello world", ECLevelH, 0, false},
		{"https://github.com/olejje/ascii-raytrace", ECLevelH, 10, false},
		{"version seven with a longer message", ECLevelQ, 7, false},
	}
	for i, c := range cases {
		m, err := Encode(c.text, Options{EC: c.ec, Version: c.ver})
		if err != nil {
			t.Fatal(err)
		}
		file := t.TempDir() + "/qr.png"
		writeTestPNG(t, file, m, c.invert)

		args := []string{"-q", "--raw", file}
		out, err := exec.Command(path, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("case %d: zbarimg failed: %v (%s)", i, err, out)
		}
		if got := strings.TrimSpace(string(out)); got != c.text {
			t.Errorf("case %d: decoded %q, want %q", i, got, c.text)
		}
	}
}

func TestMatchesQrencodeReference(t *testing.T) {
	path, err := exec.LookPath("qrencode")
	if err != nil {
		t.Skip("qrencode is not installed")
	}
	cases := []struct {
		text string
		ec   ECLevel
		flag string
	}{
		{"hello world", ECLevelM, "M"},
		{"the quick brown fox jumps over the lazy dog", ECLevelL, "L"},
		{"a somewhat longer byte-mode message for version four", ECLevelQ, "Q"},
	}
	for _, c := range cases {
		ours, err := Encode(c.text, Options{EC: c.ec})
		if err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(path, "-t", "ASCII", "-m", "0", "-l", c.flag, "-o", "-", c.text).Output()
		if err != nil {
			t.Fatal(err)
		}
		ref := parseASCIIMatrix(t, string(out))
		if ref.Size != ours.Size || ref.Version != ours.Version || ref.EC != ours.EC {
			t.Fatalf("%q: reference is v%d-%s, ours is v%d-%s", c.text, ref.Version, ref.EC, ours.Version, ours.EC)
		}

		// Mask selection is a heuristic and can differ between encoders, so
		// compare the unmasked data stream: mode, payload, ECC and interleaving.
		if !bytes.Equal(unmaskedData(ours), unmaskedData(ref)) {
			t.Fatalf("%q: data codewords differ from qrencode", c.text)
		}
	}
}

func parseASCIIMatrix(t *testing.T, s string) *Matrix {
	t.Helper()
	rows := strings.Split(strings.TrimRight(s, "\n"), "\n")
	n := len(rows)
	m := &Matrix{Size: n, Version: (n - 17) / 4, Modules: make([][]bool, n)}
	for y, row := range rows {
		if len(row) != 2*n {
			t.Fatalf("row %d has %d chars, want %d", y, len(row), 2*n)
		}
		m.Modules[y] = make([]bool, n)
		for x := 0; x < n; x++ {
			m.Modules[y][x] = row[2*x] == '#'
		}
	}
	m.EC, m.Mask = decodeFormat(m)
	return m
}

func formatBits(m *Matrix) int {
	set := func(bits *int, i int, v bool) {
		if v {
			*bits |= 1 << uint(i)
		}
	}
	bits := 0
	n := m.Size
	for i := 0; i <= 5; i++ {
		set(&bits, i, m.Get(8, i))
	}
	set(&bits, 6, m.Get(8, 7))
	set(&bits, 7, m.Get(8, 8))
	set(&bits, 8, m.Get(7, 8))
	for i := 9; i < 15; i++ {
		set(&bits, i, m.Get(14-i, 8))
	}
	for i := 0; i < 8; i++ {
		set(&bits, i, m.Get(n-1-i, 8))
	}
	for i := 8; i < 15; i++ {
		set(&bits, i, m.Get(8, n-15+i))
	}
	return bits
}

func decodeFormat(m *Matrix) (ECLevel, int) {
	want := formatBits(m)
	for _, ec := range []ECLevel{ECLevelL, ECLevelM, ECLevelQ, ECLevelH} {
		for mask := 0; mask < 8; mask++ {
			data := ecFormatBits[ec]<<3 | mask
			rem := data
			for i := 0; i < 10; i++ {
				rem = rem<<1 ^ (rem>>9)*0x537
			}
			if (data<<10|rem)^0x5412 == want {
				return ec, mask
			}
		}
	}
	return 0, -1
}

func unmaskedData(m *Matrix) []byte {
	funcs := newBuilder(m.Version, m.EC).isFunc
	out := make([]byte, 0, m.Size*m.Size)
	for y := 0; y < m.Size; y++ {
		for x := 0; x < m.Size; x++ {
			if funcs[y][x] {
				continue
			}
			dark := m.Get(x, y)
			if maskBit(m.Mask, x, y) {
				dark = !dark
			}
			if dark {
				out = append(out, 1)
			} else {
				out = append(out, 0)
			}
		}
	}
	return out
}

func matrixEqual(a, b *Matrix) bool {
	if a.Size != b.Size || a.Version != b.Version || a.EC != b.EC || a.Mask != b.Mask {
		return false
	}
	for y := 0; y < a.Size; y++ {
		if !bytes.Equal(boolsToBytes(a.Modules[y]), boolsToBytes(b.Modules[y])) {
			return false
		}
	}
	return true
}

func boolsToBytes(values []bool) []byte {
	out := make([]byte, len(values))
	for i, v := range values {
		if v {
			out[i] = 1
		}
	}
	return out
}

func writeTestPNG(t *testing.T, path string, m *Matrix, invert bool) {
	t.Helper()
	const scale = 8
	side := (m.Size + 8) * scale
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	ink, paper := color.Black, color.White
	if invert {
		ink, paper = color.White, color.Black
	}
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.Set(x, y, paper)
		}
	}
	for y := 0; y < m.Size; y++ {
		for x := 0; x < m.Size; x++ {
			if !m.Get(x, y) {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.Set((x+4)*scale+dx, (y+4)*scale+dy, ink)
				}
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}
