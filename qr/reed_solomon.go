package qr

// Multiplication in GF(2^8) modulo the QR primitive polynomial x^8+x^4+x^3+x^2+1.
func gfMultiply(x, y byte) byte {
	z := byte(0)
	for i := 7; i >= 0; i-- {
		z = (z << 1) ^ ((z >> 7) * 0x1D)
		z ^= ((y >> uint(i)) & 1) * x
	}
	return z
}

// reedSolomonGenerator returns the coefficients of the generator polynomial
// of the given degree, excluding the leading 1.
func reedSolomonGenerator(degree int) []byte {
	result := make([]byte, degree)
	result[degree-1] = 1
	root := byte(1)
	for i := 0; i < degree; i++ {
		for j := 0; j < degree; j++ {
			result[j] = gfMultiply(result[j], root)
			if j+1 < degree {
				result[j] ^= result[j+1]
			}
		}
		root = gfMultiply(root, 2)
	}
	return result
}

// reedSolomonRemainder computes the ECC codewords for one data block.
func reedSolomonRemainder(data, divisor []byte) []byte {
	result := make([]byte, len(divisor))
	for _, b := range data {
		factor := b ^ result[0]
		copy(result, result[1:])
		result[len(result)-1] = 0
		for i := range divisor {
			result[i] ^= gfMultiply(divisor[i], factor)
		}
	}
	return result
}
