package qr

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type Mode uint8

const (
	ModeAuto         Mode = 0
	ModeNumeric      Mode = 1
	ModeAlphanumeric Mode = 2
	ModeByte         Mode = 4
	ModeKanji        Mode = 8
)

var modeNames = [9]string{
	ModeAuto:         "auto",
	ModeNumeric:      "numeric",
	ModeAlphanumeric: "alphanumeric",
	ModeByte:         "byte",
	ModeKanji:        "kanji",
}

var modesByName = map[string]Mode{
	"auto":         ModeAuto,
	"numeric":      ModeNumeric,
	"num":          ModeNumeric,
	"alphanumeric": ModeAlphanumeric,
	"alnum":        ModeAlphanumeric,
	"byte":         ModeByte,
	"kanji":        ModeKanji,
}

func (m Mode) String() string {
	if int(m) < len(modeNames) && modeNames[m] != "" {
		return modeNames[m]
	}
	return fmt.Sprintf("Mode(%d)", uint8(m))
}

func ParseMode(s string) (Mode, error) {
	if m, ok := modesByName[strings.ToLower(s)]; ok {
		return m, nil
	}
	return ModeAuto, fmt.Errorf("invalid mode %q (want auto, numeric, alnum, byte or kanji)", s)
}

func modeIndex(m Mode) int {
	switch m {
	case ModeNumeric:
		return 0
	case ModeAlphanumeric:
		return 1
	case ModeByte:
		return 2
	case ModeKanji:
		return 3
	}
	return -1
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

var characterCountWidths = [3][4]int{
	{10, 9, 8, 8},
	{12, 11, 16, 10},
	{14, 13, 16, 12},
}

func characterCountBits(mode Mode, version int) int {
	i := modeIndex(mode)
	if i < 0 {
		return 0
	}
	group := 0
	if version >= 27 {
		group = 2
	} else if version >= 10 {
		group = 1
	}
	return characterCountWidths[group][i]
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

func payloadBitLen(text string, mode Mode, version int, eci bool) int {
	bits := 4 + characterCountBits(mode, version)
	if eci {
		bits += 12
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

var autoModes = [4]Mode{ModeNumeric, ModeAlphanumeric, ModeKanji, ModeByte}

func chooseMode(text string, version int, requested Mode, forceECI bool) (Mode, bool) {
	if requested != ModeAuto {
		return requested, forceECI || needsECI(text, requested)
	}
	best, bestBits := ModeByte, int(^uint(0)>>1)
	bestECI := false
	for _, mode := range autoModes {
		if !canEncode(text, mode) {
			continue
		}
		eci := forceECI || needsECI(text, mode)
		if length := payloadBitLen(text, mode, version, eci); length < bestBits {
			best, bestBits, bestECI = mode, length, eci
		}
	}
	return best, bestECI
}

func appendPayload(bits *bitBuffer, text string, mode Mode, version int, eci bool) error {
	if eci {
		bits.appendBits(0b0111, 4)
		bits.appendBits(26, 8)
	}
	bits.appendBits(int(mode), 4)

	switch mode {
	case ModeNumeric:
		bits.appendBits(len(text), characterCountBits(mode, version))
		for i := 0; i < len(text); i += 3 {
			switch len(text) - i {
			case 1:
				bits.appendBits(int(text[i]-'0'), 4)
			case 2:
				bits.appendBits(int(text[i]-'0')*10+int(text[i+1]-'0'), 7)
			default:
				bits.appendBits(int(text[i]-'0')*100+int(text[i+1]-'0')*10+int(text[i+2]-'0'), 10)
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
