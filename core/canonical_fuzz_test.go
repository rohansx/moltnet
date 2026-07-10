package core

import (
	"encoding/json"
	"strings"
	"testing"
)

// FuzzCanonicalize checks two invariants over arbitrary JSON inputs:
//   - determinism: canonicalizing the same value twice yields identical bytes,
//   - idempotence: the canonical output is valid JSON that re-canonicalizes to
//     itself.
//
// The seed corpus runs during a normal `go test`; deep fuzzing is available via
// `go test ./core/ -run x -fuzz FuzzCanonicalize`.
func FuzzCanonicalize(f *testing.F) {
	seeds := []string{
		`{"b":1,"a":[3,2,1],"c":{"z":true,"a":null}}`,
		`{"x":{"y":{"z":true}},"n":null,"s":"héllo\t\"q\""}`,
		`[1,2,3,"a",false]`,
		`"just a string"`,
		`123`,
		`{}`,
		`[]`,
		`{"dup":1,"dup":2}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		var v any
		dec := json.NewDecoder(strings.NewReader(s))
		dec.UseNumber()
		if dec.Decode(&v) != nil {
			return // not JSON; nothing to check
		}
		c1, err := Canonicalize(v)
		if err != nil {
			return
		}
		c2, _ := Canonicalize(v)
		if string(c1) != string(c2) {
			t.Fatalf("non-deterministic: %q vs %q", c1, c2)
		}
		// The canonical output must itself be valid JSON.
		var v2 any
		d2 := json.NewDecoder(strings.NewReader(string(c1)))
		d2.UseNumber()
		if err := d2.Decode(&v2); err != nil {
			t.Fatalf("canonical output is not valid JSON: %q: %v", c1, err)
		}
		c3, err := Canonicalize(v2)
		if err != nil {
			t.Fatalf("re-canonicalize failed: %v", err)
		}
		if string(c3) != string(c1) {
			t.Fatalf("not idempotent:\n  %q\n  %q", c1, c3)
		}
	})
}
