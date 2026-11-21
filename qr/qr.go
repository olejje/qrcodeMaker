// Package qr implements a small, dependency-free QR Code encoder.
//
// Supported modes: numeric, alphanumeric, byte (with optional ECI 26 for
// UTF-8) and Kanji (JIS X 0208), versions 1-40, all four error correction
// levels. The implementation follows ISO/IEC 18004: data encoding,
// Reed-Solomon error correction, block interleaving, function patterns, mask
// selection and format/version information.
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
	Version int     // 1-40; zero means "pick the smallest version that fits"
	Mode    Mode    // ModeAuto by default
	ECI     bool    // force ECI 26 (UTF-8) in byte mode; auto for non-ASCII
}

// Matrix is the finished QR symbol. Modules[y][x] is true for a dark module.
// Quiet zone is not included; callers add it when rendering.
type Matrix struct {
	Version int
	EC      ECLevel
	Mode    Mode
	ECI     bool
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

// Encode encodes text as a QR Code.
func Encode(text string, opts Options) (*Matrix, error) {
	ec := opts.EC
	if ec == 0 {
		ec = ECLevelM
	}
	if !validEC(ec) {
		return nil, fmt.Errorf("invalid error correction level %d", uint8(ec))
	}
	mode := opts.Mode
	if mode == ModeAuto {
		mode = ModeByte
	}
	if mode != ModeAuto && mode != ModeNumeric && mode != ModeAlphanumeric && mode != ModeByte && mode != ModeKanji {
		return nil, fmt.Errorf("invalid mode %d", uint8(mode))
	}
	if opts.Mode != ModeAuto && !canEncode(text, opts.Mode) {
		return nil, fmt.Errorf("text cannot be encoded in %s mode", opts.Mode)
	}

	version := opts.Version
	var chosenMode Mode
	var useECI bool
	if version == 0 {
		found := false
		for v := 1; v <= maxVersion; v++ {
			m, eci := chooseMode(text, v, opts.Mode, opts.ECI)
			if payloadBitLen(text, m, v, eci) <= blockSpecFor(v, ec).dataCodewords()*8 {
				version, chosenMode, useECI, found = v, m, eci, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("data too long: %d characters do not fit version %d at level %s",
				len(text), maxVersion, ec)
		}
	} else {
		if version < 1 || version > maxVersion {
			return nil, fmt.Errorf("version must be between 1 and %d, got %d", maxVersion, version)
		}
		chosenMode, useECI = chooseMode(text, version, opts.Mode, opts.ECI)
		if bits := payloadBitLen(text, chosenMode, version, useECI); bits > blockSpecFor(version, ec).dataCodewords()*8 {
			return nil, fmt.Errorf("data too long: payload needs %d bits, version %d at level %s has %d",
				bits, version, ec, blockSpecFor(version, ec).dataCodewords()*8)
		}
	}

	// 1. Build the data codeword stream.
	bits := &bitBuffer{}
	if err := appendPayload(bits, text, chosenMode, version, useECI); err != nil {
		return nil, err
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
		Mode:    chosenMode,
		ECI:     useECI,
		Mask:    bestMask,
		Size:    result.size,
		Modules: result.modules,
	}, nil
}

// byteCapacity is the largest byte-mode payload (in bytes) for a version/level.
func byteCapacity(version int, ec ECLevel) int {
	countBits := 8
	if version >= 10 {
		countBits = 16
	}
	return (blockSpecFor(version, ec).dataCodewords()*8 - 4 - countBits) / 8
}
