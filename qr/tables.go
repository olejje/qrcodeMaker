package qr

const maxVersion = 10

func validEC(ec ECLevel) bool {
	switch ec {
	case ECLevelL, ECLevelM, ECLevelQ, ECLevelH:
		return true
	}
	return false
}

// blockSpec describes how the codewords of one version/level are split into
// Reed-Solomon blocks. The tables below cover versions 1-10.
type blockSpec struct {
	totalCodewords    int
	ecCodewordsPerBlk int
	numBlocks         int
}

func (s blockSpec) dataCodewords() int {
	return s.totalCodewords - s.ecCodewordsPerBlk*s.numBlocks
}

// Number of error correction codewords per block, indexed [level][version].
var ecPerBlock = map[ECLevel][]int{
	ECLevelL: {-1, 7, 10, 15, 20, 26, 18, 20, 24, 30, 18},
	ECLevelM: {-1, 10, 16, 26, 18, 24, 16, 18, 22, 22, 26},
	ECLevelQ: {-1, 13, 22, 18, 26, 18, 24, 18, 22, 20, 24},
	ECLevelH: {-1, 17, 28, 22, 16, 22, 28, 26, 26, 24, 28},
}

// Number of error correction blocks, indexed [level][version].
var numECCBlocks = map[ECLevel][]int{
	ECLevelL: {-1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 4},
	ECLevelM: {-1, 1, 1, 1, 2, 2, 4, 4, 4, 5, 5},
	ECLevelQ: {-1, 1, 1, 2, 2, 4, 4, 6, 6, 8, 8},
	ECLevelH: {-1, 1, 1, 2, 4, 4, 4, 5, 6, 8, 8},
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
	{},          // 0 (unused)
	{},          // 1
	{6, 18},     // 2
	{6, 22},     // 3
	{6, 26},     // 4
	{6, 30},     // 5
	{6, 34},     // 6
	{6, 22, 38}, // 7
	{6, 24, 42}, // 8
	{6, 26, 46}, // 9
	{6, 28, 50}, // 10
}

// Bits used in the format information for each error correction level.
var ecFormatBits = map[ECLevel]int{
	ECLevelL: 1,
	ECLevelM: 0,
	ECLevelQ: 3,
	ECLevelH: 2,
}
