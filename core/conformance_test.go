package core

import (
	"encoding/json"
	"os"
	"testing"
)

// TestCanonicalConformance validates the Go canonicalizer against the shared
// cross-implementation vectors in spec/conformance/. The TS and Python clients
// validate against the same file, so any drift fails a test somewhere.
func TestCanonicalConformance(t *testing.T) {
	data, err := os.ReadFile("../spec/conformance/canonical_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Input    any    `json:"input"`
		Expected string `json:"expected"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) == 0 {
		t.Fatal("no canonical vectors loaded")
	}
	for i, v := range vectors {
		got, err := Canonicalize(v.Input)
		if err != nil {
			t.Fatalf("vector %d: %v", i, err)
		}
		if string(got) != v.Expected {
			t.Errorf("vector %d:\n got  %s\n want %s", i, got, v.Expected)
		}
	}
}
