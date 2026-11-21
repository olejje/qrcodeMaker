package qr

// applyMask flips every non-function module for which the mask condition holds.
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

// penalty computes the four ISO/IEC 18004 penalty rules. Lower is better.
func (b *builder) penalty() int {
	n := b.size
	score := 0

	// N1: runs of five or more same-colored modules in a row or column.
	// N3: finder-like 1:1:3:1:1 patterns with four light modules on one side.
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
				runLen = n // implicit light border
			}
			addRunHistory(&history, runLen, n)
			score += 40 * finderPatternCount(history)
		}
	}

	// N2: 2×2 blocks of a single color.
	for y := 0; y < n-1; y++ {
		for x := 0; x < n-1; x++ {
			c := b.modules[y][x]
			if c == b.modules[y][x+1] && c == b.modules[y+1][x] && c == b.modules[y+1][x+1] {
				score += 3
			}
		}
	}

	// N4: deviation from 50% dark modules, in steps of 5%.
	dark := 0
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if b.modules[y][x] {
				dark++
			}
		}
	}
	total := n * n
	steps := (abs(dark*20-total*10) + total - 1) / total
	score += steps * 10

	return score
}

// addRunHistory records a finished run; the first run gets an implicit light
// border outside the symbol.
func addRunHistory(history *[7]int, runLen, size int) {
	if history[0] == 0 {
		runLen += size
	}
	for i := 6; i > 0; i-- {
		history[i] = history[i-1]
	}
	history[0] = runLen
}

// finderPatternCount detects a 1:1:3:1:1 dark/light ratio with four light
// modules before or after it.
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

// at returns a module from a row (direction 0) or a column (direction 1).
func (b *builder) at(line, pos, direction int) bool {
	if direction == 0 {
		return b.modules[line][pos]
	}
	return b.modules[pos][line]
}
