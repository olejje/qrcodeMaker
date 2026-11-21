// Package qr implements a dependency-free QR Code encoder (ISO/IEC 18004).
package qr

import "fmt"

// ECLevel is the error correction level.
type ECLevel uint8

const (
	ECLevelL ECLevel = 1
	ECLevelM ECLevel = 0
	ECLevelQ ECLevel = 3
	ECLevelH ECLevel = 2
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

// ParseECLevel converts L, M, Q or H into an ECLevel.
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

// Options controls QR encoding; zero values enable defaults.
type Options struct {
	EC      ECLevel
	Version int
	Mode    Mode
	ECI     bool
}

// Matrix is a finished QR symbol; Modules[y][x] is true for a dark module.
type Matrix struct {
	Version int
	EC      ECLevel
	Mode    Mode
	ECI     bool
	Mask    int
	Size    int
	Modules [][]bool
}

// Get reports whether (x, y) is dark; points outside the symbol are light.
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
	if opts.Mode != ModeAuto {
		if modeIndex(opts.Mode) < 0 {
			return nil, fmt.Errorf("invalid mode %d", uint8(opts.Mode))
		}
		if !canEncode(text, opts.Mode) {
			return nil, fmt.Errorf("text cannot be encoded in %s mode", opts.Mode)
		}
	}

	version, mode, eci, err := selectEncoding(text, opts, ec)
	if err != nil {
		return nil, err
	}
	spec := blockSpecFor(version, ec)
	bits := &bitBuffer{}
	if err := appendPayload(bits, text, mode, version, eci); err != nil {
		return nil, err
	}
	base := newBuilder(version, ec)
	base.drawCodewords(interleave(bits.toCodewords(spec.dataCodewords()), spec))
	result, bestMask := applyBestMask(base)

	return &Matrix{
		Version: version,
		EC:      ec,
		Mode:    mode,
		ECI:     eci,
		Mask:    bestMask,
		Size:    result.size,
		Modules: result.modules,
	}, nil
}

func selectEncoding(text string, opts Options, ec ECLevel) (int, Mode, bool, error) {
	if opts.Version != 0 {
		if opts.Version < 1 || opts.Version > maxVersion {
			return 0, 0, false, fmt.Errorf("version must be between 1 and %d, got %d", maxVersion, opts.Version)
		}
		mode, eci := chooseMode(text, opts.Version, opts.Mode, opts.ECI)
		capacity := blockSpecFor(opts.Version, ec).dataCodewords() * 8
		if bits := payloadBitLen(text, mode, opts.Version, eci); bits > capacity {
			return 0, 0, false, fmt.Errorf("data too long: payload needs %d bits, version %d at level %s has %d",
				bits, opts.Version, ec, capacity)
		}
		return opts.Version, mode, eci, nil
	}

	for v := 1; v <= maxVersion; v++ {
		mode, eci := chooseMode(text, v, opts.Mode, opts.ECI)
		if payloadBitLen(text, mode, v, eci) <= blockSpecFor(v, ec).dataCodewords()*8 {
			return v, mode, eci, nil
		}
	}
	return 0, 0, false, fmt.Errorf("data too long: %d characters do not fit version %d at level %s",
		len(text), maxVersion, ec)
}

func applyBestMask(base *builder) (*builder, int) {
	bestMask, bestScore := 0, int(^uint(0)>>1)
	for mask := 0; mask < 8; mask++ {
		candidate := base.clone()
		candidate.applyMask(mask)
		candidate.drawFormatBits(mask)
		if score := candidate.penalty(); score < bestScore {
			bestScore, bestMask = score, mask
		}
	}
	result := base.clone()
	result.applyMask(bestMask)
	result.drawFormatBits(bestMask)
	return result, bestMask
}

func byteCapacity(version int, ec ECLevel) int {
	countBits := 8
	if version >= 10 {
		countBits = 16
	}
	return (blockSpecFor(version, ec).dataCodewords()*8 - 4 - countBits) / 8
}
