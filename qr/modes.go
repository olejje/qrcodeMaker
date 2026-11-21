package qr

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Mode is a QR data encoding mode. ModeAuto selects the shortest applicable
// representation automatically.
type Mode uint8

const (
	ModeAuto         Mode = 0
	ModeNumeric      Mode = 1 // mode indicator 0001
	ModeAlphanumeric Mode = 2 // mode indicator 0010
	ModeByte         Mode = 4 // mode indicator 0100
	ModeKanji        Mode = 8 // mode indicator 1000
)

func (m Mode) String() string {
	switch m {
	case ModeAuto:
		return "auto"
	case ModeNumeric:
		return "numeric"
	case ModeAlphanumeric:
		return "alphanumeric"
	case ModeByte:
		return "byte"
	case ModeKanji:
		return "kanji"
	default:
		return fmt.Sprintf("Mode(%d)", uint8(m))
	}
}

// ParseMode converts a mode name into a Mode.
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(s) {
	case "auto":
		return ModeAuto, nil
	case "numeric", "num":
		return ModeNumeric, nil
	case "alphanumeric", "alnum":
		return ModeAlphanumeric, nil
	case "byte":
		return ModeByte, nil
	case "kanji":
		return ModeKanji, nil
	default:
		return ModeAuto, fmt.Errorf("invalid mode %q (want auto, numeric, alnum, byte or kanji)", s)
	}
}

const alphanumericChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:"

var alphanumericValue = func() [256]int {
	var table [256]int
	for i := range table {
		table[i] = -1
	}
	for i, c := range alphanumericChars {
		table[c] = i
	}
	return table
}()

// characterCountBits returns the width of the mode's character count field.
func characterCountBits(mode Mode, version int) int {
	if version <= 9 {
		switch mode {
		case ModeNumeric:
			return 10
		case ModeAlphanumeric:
			return 9
		case ModeByte, ModeKanji:
			return 8
		}
	} else if version <= 26 {
		switch mode {
		case ModeNumeric:
			return 12
		case ModeAlphanumeric:
			return 11
		case ModeByte:
			return 16
		case ModeKanji:
			return 10
		}
	} else {
		switch mode {
		case ModeNumeric:
			return 14
		case ModeAlphanumeric:
			return 13
		case ModeByte:
			return 16
		case ModeKanji:
			return 12
		}
	}
	return 0
}

func canEncode(text string, mode Mode) bool {
	switch mode {
	case ModeNumeric:
		for i := 0; i < len(text); i++ {
			if text[i] < '0' || text[i] > '9' {
				return false
			}
		}
		return true
	case ModeAlphanumeric:
		for i := 0; i < len(text); i++ {
			if alphanumericValue[text[i]] < 0 {
				return false
			}
		}
		return true
	case ModeByte:
		return true
	case ModeKanji:
		for _, r := range text {
			if _, ok := kanjiCode(r); !ok {
				return false
			}
		}
		return true
	}
	return false
}

func needsECI(text string, mode Mode) bool {
	if mode != ModeByte {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] >= 0x80 {
			return true
		}
	}
	return false
}

// payloadBitLen is the exact number of data bits for one segment, including
// its mode indicator, character count and optional ECI header.
func payloadBitLen(text string, mode Mode, version int, eci bool) int {
	bits := 4 + characterCountBits(mode, version)
	if eci {
		bits += 4 + 8 // ECI mode indicator + designator 26 (UTF-8)
	}
	switch mode {
	case ModeNumeric:
		n := len(text)
		bits += 10 * (n / 3)
		switch n % 3 {
		case 1:
			bits += 4
		case 2:
			bits += 7
		}
	case ModeAlphanumeric:
		bits += 11 * (len(text) / 2)
		if len(text)%2 != 0 {
			bits += 6
		}
	case ModeByte:
		bits += 8 * len(text)
	case ModeKanji:
		bits += 13 * utf8.RuneCountInString(text)
	}
	return bits
}

// chooseMode returns the best mode and ECI flag for a given version.
func chooseMode(text string, version int, requested Mode, forceECI bool) (Mode, bool) {
	if requested != ModeAuto {
		return requested, forceECI || needsECI(text, requested)
	}
	candidates := []Mode{ModeNumeric, ModeAlphanumeric, ModeKanji, ModeByte}
	best, bestBits := ModeByte, int(^uint(0)>>1)
	bestECI := false
	for _, mode := range candidates {
		if !canEncode(text, mode) {
			continue
		}
		eci := forceECI || needsECI(text, mode)
		length := payloadBitLen(text, mode, version, eci)
		if length < bestBits {
			best, bestBits, bestECI = mode, length, eci
		}
	}
	return best, bestECI
}

// appendPayload writes the segment(s) to the bit buffer.
func appendPayload(bits *bitBuffer, text string, mode Mode, version int, eci bool) error {
	if eci {
		bits.appendBits(0b0111, 4) // ECI
		bits.appendBits(26, 8)     // UTF-8
	}
	bits.appendBits(int(mode), 4)

	switch mode {
	case ModeNumeric:
		digits := make([]int, 0, len(text))
		for i := 0; i < len(text); i++ {
			digits = append(digits, int(text[i]-'0'))
		}
		bits.appendBits(len(digits), characterCountBits(mode, version))
		for i := 0; i < len(digits); i += 3 {
			switch len(digits) - i {
			case 1:
				bits.appendBits(digits[i], 4)
			case 2:
				bits.appendBits(digits[i]*10+digits[i+1], 7)
			default:
				bits.appendBits(digits[i]*100+digits[i+1]*10+digits[i+2], 10)
			}
		}
	case ModeAlphanumeric:
		bits.appendBits(len(text), characterCountBits(mode, version))
		for i := 0; i < len(text); i += 2 {
			if i+1 < len(text) {
				bits.appendBits(alphanumericValue[text[i]]*45+alphanumericValue[text[i+1]], 11)
			} else {
				bits.appendBits(alphanumericValue[text[i]], 6)
			}
		}
	case ModeByte:
		bits.appendBits(len(text), characterCountBits(mode, version))
		for i := 0; i < len(text); i++ {
			bits.appendBits(int(text[i]), 8)
		}
	case ModeKanji:
		runes := []rune(text)
		bits.appendBits(len(runes), characterCountBits(mode, version))
		for _, r := range runes {
			code, ok := kanjiCode(r)
			if !ok {
				return fmt.Errorf("character %q cannot be encoded in Kanji mode", r)
			}
			bits.appendBits(code, 13)
		}
	default:
		return fmt.Errorf("unsupported mode %s", mode)
	}
	return nil
}
