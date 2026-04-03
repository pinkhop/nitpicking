package domain

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
)

// resetKeyLength is the number of Crockford Base32 characters in a reset key.
// Each character encodes 5 bits; 26 characters encode 130 bits, of which
// 128 are random (the leading character uses only 3 of its 5 bits).
const resetKeyLength = 26

// ResetKeyGenerate produces a cryptographically random reset key encoded as
// lowercase Crockford Base32. Uses math/rand/v2, which is backed by
// crypto/rand by default in Go 1.22+. The key encodes 128 bits of entropy —
// identical to the claim ID generation algorithm.
func ResetKeyGenerate() string {
	hi := rand.Uint64() // #nosec G404 -- math/rand/v2 is backed by crypto/rand in Go 1.22+
	lo := rand.Uint64() // #nosec G404 -- see comment above

	buf := make([]byte, resetKeyLength)

	// Extract 5-bit groups right-to-left from lo (12 full groups = 60 bits),
	// then the boundary group spanning lo and hi, then from hi.
	for i := resetKeyLength - 1; i >= 14; i-- {
		buf[i] = crockfordAlphabet[lo&0x1f]
		lo >>= 5
	}
	// lo has 4 remaining bits. Combine with 1 bit from hi for character 13.
	buf[13] = crockfordAlphabet[(lo&0x0f)|((hi&0x01)<<4)]
	hi >>= 1

	// hi now has 63 bits. Extract 12 full 5-bit groups (60 bits).
	for i := 12; i >= 1; i-- {
		buf[i] = crockfordAlphabet[hi&0x1f]
		hi >>= 5
	}
	// hi has 3 remaining bits for the leading character.
	buf[0] = crockfordAlphabet[hi&0x07]

	return string(buf)
}

// ResetKeyHash computes the SHA-512 hash of the binary value encoded by a
// Crockford Base32 reset key. The key is first normalized (lowercased, with
// I/L→1 and O→0 substitutions) and then decoded back to 128-bit binary before
// hashing. Returns the hex-encoded hash string. Returns an error if the key is
// not a valid 26-character Crockford Base32 string.
func ResetKeyHash(key string) (string, error) {
	normalized := NormalizeCrockford(key)

	if len(normalized) != resetKeyLength {
		return "", fmt.Errorf("reset key must be %d Crockford Base32 characters, got %d", resetKeyLength, len(normalized))
	}

	// Validate all characters are in the Crockford alphabet. The normalized
	// string is ASCII-only, so the byte→rune widening is lossless.
	for i := range len(normalized) {
		if !isCrockfordChar(rune(normalized[i])) {
			return "", fmt.Errorf("reset key character %d (%c) is not valid Crockford Base32", i, normalized[i])
		}
	}

	// Decode the Crockford Base32 string back to 128 bits (16 bytes).
	binary := decodeCrockford(normalized)

	hash := sha512.Sum512(binary)
	return hex.EncodeToString(hash[:]), nil
}

// decodeCrockford decodes a 26-character lowercase Crockford Base32 string
// into 16 bytes (128 bits). The caller must ensure the input is valid and
// normalized.
func decodeCrockford(s string) []byte {
	// Build a lookup table for decoding. The alphabet is ASCII-only so
	// iterating bytes avoids int→byte overflow warnings.
	var charVal [256]byte
	for i := range len(crockfordAlphabet) {
		charVal[crockfordAlphabet[i]] = byte(i) // #nosec G115 -- i is at most 31, fits in byte
	}

	// Reconstruct the two uint64s from 5-bit groups.
	// Leading character uses 3 bits for hi.
	var hi, lo uint64

	hi = uint64(charVal[s[0]] & 0x07)
	for i := 1; i <= 12; i++ {
		hi = (hi << 5) | uint64(charVal[s[i]])
	}
	// hi now has 3 + 12*5 = 63 bits. Shift left 1 for the boundary bit.
	hi = (hi << 1) | uint64(charVal[s[13]]>>4)

	lo = uint64(charVal[s[13]] & 0x0f)
	for i := 14; i < resetKeyLength; i++ {
		lo = (lo << 5) | uint64(charVal[s[i]])
	}

	// Encode the two uint64s as big-endian into 16 bytes.
	var out [16]byte
	for i := 7; i >= 0; i-- {
		out[i] = byte(hi & 0xff)
		hi >>= 8
	}
	for i := 15; i >= 8; i-- {
		out[i] = byte(lo & 0xff)
		lo >>= 8
	}

	return out[:]
}
