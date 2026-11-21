package qr

type builder struct {
	version int
	ec      ECLevel
	size    int
	modules [][]bool // [y][x], true = dark
	isFunc  [][]bool // function modules are excluded from data and masking
}

func newBuilder(version int, ec ECLevel) *builder {
	size := version*4 + 17
	b := &builder{
		version: version,
		ec:      ec,
		size:    size,
		modules: make([][]bool, size),
		isFunc:  make([][]bool, size),
	}
	for i := range b.modules {
		b.modules[i] = make([]bool, size)
		b.isFunc[i] = make([]bool, size)
	}
	b.drawFunctionPatterns()
	return b
}

func (b *builder) clone() *builder {
	c := &builder{
		version: b.version,
		ec:      b.ec,
		size:    b.size,
		modules: make([][]bool, b.size),
		isFunc:  make([][]bool, b.size),
	}
	for i := range b.modules {
		c.modules[i] = append([]bool(nil), b.modules[i]...)
		c.isFunc[i] = append([]bool(nil), b.isFunc[i]...)
	}
	return c
}

func (b *builder) setFunc(x, y int, dark bool) {
	if x < 0 || y < 0 || x >= b.size || y >= b.size {
		return
	}
	b.modules[y][x] = dark
	b.isFunc[y][x] = true
}

func (b *builder) drawFunctionPatterns() {
	// Timing patterns.
	for i := 0; i < b.size; i++ {
		b.setFunc(6, i, i%2 == 0)
		b.setFunc(i, 6, i%2 == 0)
	}

	// Three finder patterns with their separators.
	b.drawFinder(3, 3)
	b.drawFinder(b.size-4, 3)
	b.drawFinder(3, b.size-4)

	// Alignment patterns, skipping those overlapping the finders.
	positions := alignPositions[b.version]
	last := len(positions) - 1
	for i, x := range positions {
		for j, y := range positions {
			if (i == 0 && j == 0) || (i == 0 && j == last) || (i == last && j == 0) {
				continue
			}
			b.drawAlignment(x, y)
		}
	}

	// Reserve and initially fill the format area with mask 0.
	b.drawFormatBits(0)
	if b.version >= 7 {
		b.drawVersionBits()
	}
}

func (b *builder) drawFinder(cx, cy int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			dist := abs(dx)
			if abs(dy) > dist {
				dist = abs(dy)
			}
			b.setFunc(cx+dx, cy+dy, dist != 2 && dist != 4)
		}
	}
}

func (b *builder) drawAlignment(cx, cy int) {
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			dist := abs(dx)
			if abs(dy) > dist {
				dist = abs(dy)
			}
			b.setFunc(cx+dx, cy+dy, dist != 1)
		}
	}
}

// drawFormatBits writes the 15-bit BCH-protected format information.
func (b *builder) drawFormatBits(mask int) {
	data := ecFormatBits[b.ec]<<3 | mask
	rem := data
	for i := 0; i < 10; i++ {
		rem = (rem << 1) ^ ((rem >> 9) * 0x537)
	}
	bits := ((data << 10) | rem) ^ 0x5412

	for i := 0; i <= 5; i++ {
		b.setFunc(8, i, getBit(bits, i))
	}
	b.setFunc(8, 7, getBit(bits, 6))
	b.setFunc(8, 8, getBit(bits, 7))
	b.setFunc(7, 8, getBit(bits, 8))
	for i := 9; i < 15; i++ {
		b.setFunc(14-i, 8, getBit(bits, i))
	}
	for i := 0; i < 8; i++ {
		b.setFunc(b.size-1-i, 8, getBit(bits, i))
	}
	for i := 8; i < 15; i++ {
		b.setFunc(8, b.size-15+i, getBit(bits, i))
	}

	// The fixed dark module.
	b.setFunc(8, b.size-8, true)
}

// drawVersionBits writes the 18-bit BCH-protected version information (v7+).
func (b *builder) drawVersionBits() {
	rem := b.version
	for i := 0; i < 12; i++ {
		rem = (rem << 1) ^ ((rem >> 11) * 0x1F25)
	}
	bits := b.version<<12 | rem

	for i := 0; i < 18; i++ {
		bit := getBit(bits, i)
		a := b.size - 11 + i%3
		c := i / 3
		b.setFunc(a, c, bit)
		b.setFunc(c, a, bit)
	}
}

func (b *builder) drawCodewords(data []byte) {
	bitIndex := 0
	for right := b.size - 1; right >= 1; right -= 2 {
		if right == 6 {
			right = 5 // skip the vertical timing column
		}
		for vert := 0; vert < b.size; vert++ {
			for j := 0; j < 2; j++ {
				x := right - j
				y := vert
				if (right+1)&2 == 0 {
					y = b.size - 1 - vert
				}
				if b.isFunc[y][x] || bitIndex >= len(data)*8 {
					continue
				}
				b.modules[y][x] = getBit(int(data[bitIndex>>3]), 7-bitIndex%8)
				bitIndex++
			}
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func getBit(x, i int) bool {
	return x&(1<<uint(i)) != 0
}
