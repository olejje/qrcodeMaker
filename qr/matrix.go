package qr

type builder struct {
	version int
	ec      ECLevel
	size    int
	modules [][]bool
	isFunc  [][]bool
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
		isFunc:  b.isFunc,
	}
	for i := range b.modules {
		c.modules[i] = append([]bool(nil), b.modules[i]...)
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
	for i := 0; i < b.size; i++ {
		b.setFunc(6, i, i%2 == 0)
		b.setFunc(i, 6, i%2 == 0)
	}
	b.drawFinder(3, 3)
	b.drawFinder(b.size-4, 3)
	b.drawFinder(3, b.size-4)

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
	b.drawFormatBits(0)
	if b.version >= 7 {
		b.drawVersionBits()
	}
}

func (b *builder) drawFinder(cx, cy int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			dist := maxInt(abs(dx), abs(dy))
			b.setFunc(cx+dx, cy+dy, dist != 2 && dist != 4)
		}
	}
}

func (b *builder) drawAlignment(cx, cy int) {
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			b.setFunc(cx+dx, cy+dy, maxInt(abs(dx), abs(dy)) != 1)
		}
	}
}

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
	b.setFunc(8, b.size-8, true)
}

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
			right = 5
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

func (b *builder) applyMask(mask int) {
	for y := 0; y < b.size; y++ {
		for x := 0; x < b.size; x++ {
			if !b.isFunc[y][x] && maskBit(mask, x, y) {
				b.modules[y][x] = !b.modules[y][x]
			}
		}
	}
}

func maskBit(mask, x, y int) bool {
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
	case 7:
		return ((x+y)%2+x*y%3)%2 == 0
	}
	return false
}

func (b *builder) penalty() int {
	n := b.size
	score := 0
	for direction := 0; direction < 2; direction++ {
		for line := 0; line < n; line++ {
			runColor := false
			runLen := 0
			var history [7]int
			for pos := 0; pos < n; pos++ {
				c := b.at(line, pos, direction)
				if c == runColor {
					runLen++
					if runLen == 5 {
						score += 3
					} else if runLen > 5 {
						score++
					}
					continue
				}
				addRunHistory(&history, runLen, n)
				if !runColor {
					score += 40 * finderPatternCount(history)
				}
				runColor, runLen = c, 1
			}
			if runColor {
				addRunHistory(&history, runLen, n)
				runLen = n
			}
			addRunHistory(&history, runLen, n)
			score += 40 * finderPatternCount(history)
		}
	}

	for y := 0; y < n-1; y++ {
		for x := 0; x < n-1; x++ {
			c := b.modules[y][x]
			if c == b.modules[y][x+1] && c == b.modules[y+1][x] && c == b.modules[y+1][x+1] {
				score += 3
			}
		}
	}

	dark := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if b.modules[y][x] {
				dark++
			}
		}
	}
	total := n * n
	score += ((abs(dark*20-total*10) + total - 1) / total) * 10
	return score
}

func addRunHistory(history *[7]int, runLen, size int) {
	if history[0] == 0 {
		runLen += size
	}
	for i := 6; i > 0; i-- {
		history[i] = history[i-1]
	}
	history[0] = runLen
}

func finderPatternCount(history [7]int) int {
	n := history[1]
	core := n > 0 && history[2] == n && history[3] == 3*n && history[4] == n && history[5] == n
	count := 0
	if core && history[0] >= 4*n && history[6] >= n {
		count++
	}
	if core && history[6] >= 4*n && history[0] >= n {
		count++
	}
	return count
}

func (b *builder) at(line, pos, direction int) bool {
	if direction == 0 {
		return b.modules[line][pos]
	}
	return b.modules[pos][line]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func getBit(x, i int) bool {
	return x&(1<<uint(i)) != 0
}
