package qr

const maxVersion = 40

func validEC(ec ECLevel) bool {
	switch ec {
	case ECLevelL, ECLevelM, ECLevelQ, ECLevelH:
		return true
	}
	return false
}

// blockSpec describes how the codewords of one version/level are split into
// Reed-Solomon blocks.
type blockSpec struct {
	totalCodewords    int
	ecCodewordsPerBlk int
	numBlocks         int
}

func (s blockSpec) dataCodewords() int {
	return s.totalCodewords - s.ecCodewordsPerBlk*s.numBlocks
}

// Number of error correction codewords per block, indexed [level][version].
// Index 0 is an unused padding entry.
var ecPerBlock = map[ECLevel][]int{
	ECLevelL: {-1, 7, 10, 15, 20, 26, 18, 20, 24, 30, 18, 20, 24, 26, 30, 22, 24, 28, 30, 28, 28, 28, 28, 30, 30, 26, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
	ECLevelM: {-1, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26, 30, 22, 22, 24, 24, 28, 28, 26, 26, 26, 26, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28, 28},
	ECLevelQ: {-1, 13, 22, 18, 26, 18, 24, 18, 22, 20, 24, 28, 26, 24, 20, 30, 24, 28, 28, 26, 30, 28, 30, 30, 30, 30, 28, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
	ECLevelH: {-1, 17, 28, 22, 16, 22, 28, 26, 26, 24, 28, 24, 28, 22, 24, 24, 30, 28, 28, 26, 28, 30, 24, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30, 30},
}

// Number of error correction blocks, indexed [level][version].
var numECCBlocks = map[ECLevel][]int{
	ECLevelL: {-1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 4, 4, 4, 4, 4, 6, 6, 6, 6, 7, 8, 8, 9, 9, 10, 12, 12, 12, 13, 14, 15, 16, 17, 18, 19, 19, 20, 21, 22, 24, 25},
	ECLevelM: {-1, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5, 5, 8, 9, 9, 10, 10, 11, 13, 14, 16, 17, 17, 18, 20, 21, 23, 25, 26, 28, 29, 31, 33, 35, 37, 38, 40, 43, 45, 47, 49},
	ECLevelQ: {-1, 1, 1, 2, 2, 4, 4, 6, 6, 8, 8, 8, 10, 12, 16, 12, 17, 16, 18, 21, 20, 23, 23, 25, 27, 29, 34, 34, 35, 38, 40, 43, 45, 48, 51, 53, 56, 59, 62, 65, 68},
	ECLevelH: {-1, 1, 1, 2, 4, 4, 4, 5, 6, 8, 8, 11, 11, 16, 16, 18, 16, 19, 21, 25, 25, 25, 34, 30, 32, 35, 37, 40, 42, 45, 48, 51, 54, 57, 60, 63, 66, 70, 74, 77, 81},
}

func blockSpecFor(version int, ec ECLevel) blockSpec {
	return blockSpec{
		totalCodewords:    rawDataModules(version) / 8,
		ecCodewordsPerBlk: ecPerBlock[ec][version],
		numBlocks:         numECCBlocks[ec][version],
	}
}

// rawDataModules counts data modules (excluding remainder bits).
func rawDataModules(ver int) int {
	result := (16*ver+128)*ver + 64
	if ver >= 2 {
		numAlign := ver/7 + 2
		result -= (25*numAlign-10)*numAlign - 55
		if ver >= 7 {
			result -= 36
		}
	}
	return result
}

// Alignment pattern center coordinates by version.
var alignPositions = [][]int{
	{},                             // 0 (unused)
	{},                             // 1
	{6, 18},                        // 2
	{6, 22},                        // 3
	{6, 26},                        // 4
	{6, 30},                        // 5
	{6, 34},                        // 6
	{6, 22, 38},                    // 7
	{6, 24, 42},                    // 8
	{6, 26, 46},                    // 9
	{6, 28, 50},                    // 10
	{6, 30, 54},                    // 11
	{6, 32, 58},                    // 12
	{6, 34, 62},                    // 13
	{6, 26, 46, 66},                // 14
	{6, 26, 48, 70},                // 15
	{6, 26, 50, 74},                // 16
	{6, 30, 54, 78},                // 17
	{6, 30, 56, 82},                // 18
	{6, 30, 58, 86},                // 19
	{6, 34, 62, 90},                // 20
	{6, 28, 50, 72, 94},            // 21
	{6, 26, 50, 74, 98},            // 22
	{6, 30, 54, 78, 102},           // 23
	{6, 28, 54, 80, 106},           // 24
	{6, 32, 58, 84, 110},           // 25
	{6, 30, 58, 86, 114},           // 26
	{6, 34, 62, 90, 118},           // 27
	{6, 26, 50, 74, 98, 122},       // 28
	{6, 30, 54, 78, 102, 126},      // 29
	{6, 26, 52, 78, 104, 130},      // 30
	{6, 30, 56, 82, 108, 134},      // 31
	{6, 34, 60, 86, 112, 138},      // 32
	{6, 30, 58, 86, 114, 142},      // 33
	{6, 34, 62, 90, 118, 146},      // 34
	{6, 30, 54, 78, 102, 126, 150}, // 35
	{6, 24, 50, 76, 102, 128, 154}, // 36
	{6, 28, 54, 80, 106, 132, 158}, // 37
	{6, 32, 58, 84, 110, 136, 162}, // 38
	{6, 26, 54, 82, 110, 138, 166}, // 39
	{6, 30, 58, 86, 114, 142, 170}, // 40
}

// Bits used in the format information for each error correction level.
var ecFormatBits = map[ECLevel]int{
	ECLevelL: 1,
	ECLevelM: 0,
	ECLevelQ: 3,
	ECLevelH: 2,
}
