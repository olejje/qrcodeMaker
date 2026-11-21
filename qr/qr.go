// Package qr implements a small, dependency-free QR Code encoder.
//
// Supported modes: byte mode, versions 1-10, all four error correction levels.
// The encoder follows ISO/IEC 18004: data encoding, Reed-Solomon error
// correction, block interleaving, function patterns, mask selection and
// format/version information.
package qr

import "fmt"

// ECLevel is the error correction level of a QR Code.
type ECLevel uint8

const (
	ECLevelL ECLevel = 1 // ~7% of codewords can be restored
	ECLevelM ECLevel = 0 // ~15%
	ECLevelQ ECLevel = 3 // ~25%
	ECLevelH ECLevel = 2 // ~30%
)

func (e ECLevel) String() string {
	switch e {
	case ECLevelL:
		return "L"
	case ECLevelM:
		return "M"
	case ECLevelQ:
		return "Q"
	case ECLevelH:
		return "H"
	default:
		return fmt.Sprintf("ECLevel(%d)", uint8(e))
	}
}

// ParseECLevel converts "L", "M", "Q" or "H" into an ECLevel.
func ParseECLevel(s string) (ECLevel, error) {
	switch s {
	case "L":
		return ECLevelL, nil
	case "M":
		return ECLevelM, nil
	case "Q":
		return ECLevelQ, nil
	case "H":
		return ECLevelH, nil
	default:
		return 0, fmt.Errorf("invalid error correction level %q (want L, M, Q or H)", s)
	}
}

// Options controls the QR Code encoding.
type Options struct {
	EC      ECLevel // error correction level; zero means ECLevelM
	Version int     // 1-10; zero means "pick the smallest version that fits"
}

// Matrix is the finished QR symbol. Modules[y][x] is true for a dark module.
// Quiet zone is not included; callers add it when rendering.
type Matrix struct {
	Version int
	EC      ECLevel
	Mask    int
	Size    int
	Modules [][]bool
}

// Get reports whether the module at (x, y) is dark. Coordinates outside the
// symbol are treated as light, which is handy when adding a quiet zone.
func (m *Matrix) Get(x, y int) bool {
	if x < 0 || y < 0 || x >= m.Size || y >= m.Size {
		return false
	}
	return m.Modules[y][x]
}

// Encode encodes text as a QR Code using byte mode.
func Encode(text string, opts Options) (*Matrix, error) {
	ec := opts.EC
	if ec == 0 {
		ec = ECLevelM
	}
	if !validEC(ec) {
		return nil, fmt.Errorf("invalid error correction level %d", uint8(ec))
	}

	data := []byte(text)
	version := opts.Version
	if version == 0 {
		found := false
		for v := 1; v <= maxVersion; v++ {
			if byteCapacity(v, ec) >= len(data) {
				version = v
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("data too long: %d bytes exceed the capacity of version %d at level %s (%d bytes)",
				len(data), maxVersion, ec, byteCapacity(maxVersion, ec))
		}
	}
	if version < 1 || version > maxVersion {
		return nil, fmt.Errorf("version must be between 1 and %d, got %d", maxVersion, version)
	}
	if len(data) > byteCapacity(version, ec) {
		return nil, fmt.Errorf("data too long: %d bytes do not fit version %d at level %s (%d bytes)",
			len(data), version, ec, byteCapacity(version, ec))
	}

	// 1. Build the data codeword stream.
	bits := &bitBuffer{}
	bits.appendBits(0b0100, 4) // byte mode
	if version <= 9 {
		bits.appendBits(len(data), 8)
	} else {
		bits.appendBits(len(data), 16)
	}
	for _, b := range data {
		bits.appendBits(int(b), 8)
	}

	spec := blockSpecFor(version, ec)
	codewords := bits.toCodewords(spec.dataCodewords())

	// 2. Add error correction and interleave the blocks.
	all := interleave(codewords, spec)

	// 3. Place codewords, then try all eight masks and keep the best score.
	base := newBuilder(version, ec)
	base.drawCodewords(all)

	bestMask, bestScore := 0, int(^uint(0)>>1)
	for mask := 0; mask < 8; mask++ {
		candidate := base.clone()
		candidate.applyMask(mask)
		candidate.drawFormatBits(mask)
		score := candidate.penalty()
		if score < bestScore {
			bestScore, bestMask = score, mask
		}
	}

	result := base.clone()
	result.applyMask(bestMask)
	result.drawFormatBits(bestMask)

	return &Matrix{
		Version: version,
		EC:      ec,
		Mask:    bestMask,
		Size:    result.size,
		Modules: result.modules,
	}, nil
}

// byteCapacity is the largest byte-mode payload (in bytes) for a version/level.
func byteCapacity(version int, ec ECLevel) int {
	spec := blockSpecFor(version, ec)
	countBits := 8
	if version >= 10 {
		countBits = 16
	}
	return (spec.dataCodewords()*8 - 4 - countBits) / 8
}
