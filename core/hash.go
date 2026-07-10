package core

import (
	"encoding/hex"

	"lukechampine.com/blake3"
)

// HashBytes returns a "blake3:<hex>" tag for the given bytes.
func HashBytes(b []byte) string {
	sum := blake3.Sum256(b)
	return "blake3:" + hex.EncodeToString(sum[:])
}

// HashCanonical canonicalizes v and returns its "blake3:<hex>" content hash.
func HashCanonical(v any) (string, error) {
	c, err := Canonicalize(v)
	if err != nil {
		return "", err
	}
	return HashBytes(c), nil
}
