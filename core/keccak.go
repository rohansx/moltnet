package core

// Minimal, dependency-free Keccak-256 (the original Keccak padding used by
// Ethereum, not the FIPS-202 SHA3-256 variant). It exists solely so MoltNet can
// validate EIP-55 address checksums for ERC-8004 anchors without pulling in a
// crypto dependency. It is not performance-critical and processes only short
// inputs (addresses), so clarity is favoured over speed.

// keccakRC are the 24 Keccak-f[1600] round constants.
var keccakRC = [24]uint64{
	0x0000000000000001, 0x0000000000008082, 0x800000000000808a, 0x8000000080008000,
	0x000000000000808b, 0x0000000080000001, 0x8000000080008081, 0x8000000000008009,
	0x000000000000008a, 0x0000000000000088, 0x0000000080008009, 0x000000008000000a,
	0x000000008000808b, 0x800000000000008b, 0x8000000000008089, 0x8000000000008003,
	0x8000000000008002, 0x8000000000000080, 0x000000000000800a, 0x800000008000000a,
	0x8000000080008081, 0x8000000000008080, 0x0000000080000001, 0x8000000080008008,
}

// keccakRotc are the rho-step rotation offsets, in pi-permutation order.
var keccakRotc = [24]int{
	1, 3, 6, 10, 15, 21, 28, 36, 45, 55, 2, 14,
	27, 41, 56, 8, 25, 43, 62, 18, 39, 61, 20, 44,
}

// keccakPiln are the pi-step lane destinations.
var keccakPiln = [24]int{
	10, 7, 11, 17, 18, 3, 5, 16, 8, 21, 24, 4,
	15, 23, 19, 13, 12, 2, 20, 14, 22, 9, 6, 1,
}

func rotl64(x uint64, n int) uint64 {
	return (x << uint(n)) | (x >> uint(64-n))
}

// keccakF applies the Keccak-f[1600] permutation to the state in place. The
// structure follows the compact tiny_sha3 reference.
func keccakF(a *[25]uint64) {
	for round := 0; round < 24; round++ {
		// theta
		var c [5]uint64
		for i := 0; i < 5; i++ {
			c[i] = a[i] ^ a[i+5] ^ a[i+10] ^ a[i+15] ^ a[i+20]
		}
		for i := 0; i < 5; i++ {
			t := c[(i+4)%5] ^ rotl64(c[(i+1)%5], 1)
			for j := 0; j < 25; j += 5 {
				a[j+i] ^= t
			}
		}
		// rho + pi
		t := a[1]
		for i := 0; i < 24; i++ {
			j := keccakPiln[i]
			bc0 := a[j]
			a[j] = rotl64(t, keccakRotc[i])
			t = bc0
		}
		// chi
		for j := 0; j < 25; j += 5 {
			var bc [5]uint64
			for i := 0; i < 5; i++ {
				bc[i] = a[j+i]
			}
			for i := 0; i < 5; i++ {
				a[j+i] ^= (^bc[(i+1)%5]) & bc[(i+2)%5]
			}
		}
		// iota
		a[0] ^= keccakRC[round]
	}
}

// Keccak256 returns the 32-byte Keccak-256 digest of data.
func Keccak256(data []byte) [32]byte {
	const rate = 136 // bytes (1088 bits) for Keccak-256
	var a [25]uint64

	// absorb full rate-sized blocks
	in := data
	for len(in) >= rate {
		absorbBlock(&a, in[:rate])
		keccakF(&a)
		in = in[rate:]
	}

	// pad: last partial block with Keccak (Ethereum) domain byte 0x01 and 0x80.
	var block [rate]byte
	copy(block[:], in)
	block[len(in)] ^= 0x01
	block[rate-1] ^= 0x80
	absorbBlock(&a, block[:])
	keccakF(&a)

	// squeeze 32 bytes (fits in the first four lanes)
	var out [32]byte
	for i := 0; i < 4; i++ {
		lane := a[i]
		for j := 0; j < 8; j++ {
			out[i*8+j] = byte(lane >> uint(8*j))
		}
	}
	return out
}

// absorbBlock XORs a rate-sized block into the state as little-endian lanes.
func absorbBlock(a *[25]uint64, block []byte) {
	for i := 0; i < len(block)/8; i++ {
		var lane uint64
		for j := 0; j < 8; j++ {
			lane |= uint64(block[i*8+j]) << uint(8*j)
		}
		a[i] ^= lane
	}
}
