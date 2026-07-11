package core

import "testing"

func TestKeccak256Vectors(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Empty input — the canonical Ethereum keccak256("") value.
		{"", "c5d2460186f7233c927e7db2dcc703c0e500b653ca82273b7bfad8045d85a470"},
		{"abc", "4e03657aea45a94fc7d47ba826c8d667c0d1e6e33a64a036ec44f58fa12d6c45"},
		{"The quick brown fox jumps over the lazy dog",
			"4d741b6f1eb29cb2a9b9911c82f56fa8d73b04959d3d9d222895df6c0b28aa15"},
	}
	for _, c := range cases {
		got := hexEncode(Keccak256([]byte(c.in)))
		if got != c.want {
			t.Errorf("keccak256(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func hexEncode(b [32]byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 64)
	for i, v := range b {
		out[i*2] = hexdigits[v>>4]
		out[i*2+1] = hexdigits[v&0x0f]
	}
	return string(out)
}
