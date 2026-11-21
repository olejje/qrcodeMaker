package qr

// bitBuffer accumulates bits MSB-first, as required by the QR specification.
type bitBuffer struct {
	bytes  []byte
	bitLen int
}

func (b *bitBuffer) appendBit(bit bool) {
	if b.bitLen%8 == 0 {
		b.bytes = append(b.bytes, 0)
	}
	if bit {
		b.bytes[b.bitLen/8] |= 1 << uint(7-b.bitLen%8)
	}
	b.bitLen++
}

func (b *bitBuffer) appendBits(val, length int) {
	for i := length - 1; i >= 0; i-- {
		b.appendBit(val&(1<<uint(i)) != 0)
	}
}

// toCodewords pads the stream with the terminator, zero bits and alternating
// pad bytes until it is exactly numCodewords long.
func (b *bitBuffer) toCodewords(numCodewords int) []byte {
	capacity := numCodewords * 8

	terminator := 4
	if remain := capacity - b.bitLen; remain < terminator {
		terminator = remain
	}
	if terminator > 0 {
		b.appendBits(0, terminator)
	}
	for b.bitLen%8 != 0 && b.bitLen < capacity {
		b.appendBit(false)
	}
	for i := 0; len(b.bytes) < numCodewords; i++ {
		if i%2 == 0 {
			b.bytes = append(b.bytes, 0xEC)
		} else {
			b.bytes = append(b.bytes, 0x11)
		}
	}
	return b.bytes
}

// interleave splits the data codewords into blocks, appends Reed-Solomon
// error correction to each block, then interleaves data and ECC bytes.
func interleave(data []byte, spec blockSpec) []byte {
	numBlocks := spec.numBlocks
	numData := spec.dataCodewords()
	numECC := spec.ecCodewordsPerBlk

	shortLen := numData / numBlocks
	numShort := numBlocks - numData%numBlocks

	dataBlocks := make([][]byte, numBlocks)
	divisor := reedSolomonGenerator(numECC)
	pos := 0
	for i := 0; i < numBlocks; i++ {
		length := shortLen + 1
		if i < numShort {
			length = shortLen
		}
		dataBlocks[i] = make([]byte, length)
		copy(dataBlocks[i], data[pos:pos+length])
		pos += length
	}

	eccBlocks := make([][]byte, numBlocks)
	for i := 0; i < numBlocks; i++ {
		eccBlocks[i] = reedSolomonRemainder(dataBlocks[i], divisor)
	}

	result := make([]byte, 0, spec.totalCodewords)
	for i := 0; i <= shortLen; i++ {
		for j := 0; j < numBlocks; j++ {
			if i == shortLen && j < numShort {
				continue
			}
			if i == shortLen && i >= len(dataBlocks[j]) {
				continue
			}
			result = append(result, dataBlocks[j][i])
		}
	}
	for i := 0; i < numECC; i++ {
		for j := 0; j < numBlocks; j++ {
			result = append(result, eccBlocks[j][i])
		}
	}
	return result
}
