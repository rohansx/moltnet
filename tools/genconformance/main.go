// Command genconformance regenerates the cross-implementation conformance
// vectors in spec/conformance/ from the Go reference implementation. The Go, TS
// and Python clients all validate against these frozen vectors, so any drift
// between implementations fails a test.
//
//	go run ./tools/genconformance
package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/moltnet/moltnet/core"
	"github.com/moltnet/moltnet/score"
)

type canonVector struct {
	Input    any    `json:"input"`
	Expected string `json:"expected"`
}

type scoreVector struct {
	Now          string           `json:"now"`
	Attestations []map[string]any `json:"attestations"`
	Expected     scoreExpected    `json:"expected"`
}

type scoreExpected struct {
	Score  float64      `json:"score"`
	Inputs score.Inputs `json:"inputs"`
}

func main() {
	// --- canonicalization vectors ---
	inputs := []any{
		map[string]any{"b": 1, "a": []any{3, 2, 1}, "c": map[string]any{"z": true, "a": nil}},
		map[string]any{"unicode": "héllo", "tab": "a\tb", "quote": "a\"b", "back": "a\\b"},
		map[string]any{"nested": map[string]any{"x": map[string]any{"y": 1}}, "arr": []any{map[string]any{"k": 2}, map[string]any{"k": 1}}},
		map[string]any{"spec": "moltnet/card/v0.1", "id": "did:key:z6Mk", "n": 10, "flag": false, "empty": ""},
	}
	var cvs []canonVector
	for _, in := range inputs {
		c, err := core.Canonicalize(in)
		if err != nil {
			log.Fatal(err)
		}
		cvs = append(cvs, canonVector{Input: in, Expected: string(c)})
	}

	// --- score vectors (fixed clock so decay is deterministic) ---
	now, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
	iso := now.UTC().Format(time.RFC3339)
	att := func(typ, issuer string) *core.Attestation {
		a := core.NewAttestation(typ, issuer, "did:key:zSubject")
		a.IssuedAt = iso
		return a
	}
	scenarios := [][]*core.Attestation{
		{},
		{att("task.completed", "did:key:zA")},
		{att("task.completed", "did:key:zA"), att("task.completed", "did:key:zB"), att("endorsement", "did:key:zC")},
		{att("task.completed", "did:key:zA"), att("incident", "did:key:zB")},
		{att("self.claim", "did:key:zA"), att("self.claim", "did:key:zA")},
		{att("task.completed", "did:key:zA"), att("task.completed", "did:key:zA"), att("payment.receipt", "did:key:zB")},
	}
	var svs []scoreVector
	for _, sc := range scenarios {
		out := score.Compute(sc, nil, now)
		var atts []map[string]any
		for _, a := range sc {
			atts = append(atts, map[string]any{
				"type": a.Type, "issuer": a.Issuer, "subject": a.Subject, "issued_at": a.IssuedAt,
			})
		}
		svs = append(svs, scoreVector{Now: iso, Attestations: atts, Expected: scoreExpected{Score: out.Score, Inputs: out.Inputs}})
	}

	dir := filepath.Join("spec", "conformance")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatal(err)
	}
	write(filepath.Join(dir, "canonical_vectors.json"), cvs)
	write(filepath.Join(dir, "score_vectors.json"), svs)
	log.Printf("wrote %d canonical + %d score vectors to %s", len(cvs), len(svs), dir)
}

func write(path string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
}
