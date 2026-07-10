package score

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/moltnet/moltnet/core"
)

// TestScoreConformance validates MoltScore v1 against the shared vectors in
// spec/conformance/, which the TS and Python clients also validate against.
func TestScoreConformance(t *testing.T) {
	data, err := os.ReadFile("../spec/conformance/score_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Now          string           `json:"now"`
		Attestations []map[string]any `json:"attestations"`
		Expected     struct {
			Score  float64 `json:"score"`
			Inputs Inputs  `json:"inputs"`
		} `json:"expected"`
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) == 0 {
		t.Fatal("no score vectors loaded")
	}
	for i, v := range vectors {
		now, err := time.Parse(time.RFC3339, v.Now)
		if err != nil {
			t.Fatal(err)
		}
		var atts []*core.Attestation
		for _, m := range v.Attestations {
			atts = append(atts, &core.Attestation{
				Type:     m["type"].(string),
				Issuer:   m["issuer"].(string),
				Subject:  m["subject"].(string),
				IssuedAt: m["issued_at"].(string),
			})
		}
		out := Compute(atts, nil, now)
		if out.Score != v.Expected.Score {
			t.Errorf("vector %d: score got %v want %v", i, out.Score, v.Expected.Score)
		}
		if out.Inputs != v.Expected.Inputs {
			t.Errorf("vector %d: inputs got %+v want %+v", i, out.Inputs, v.Expected.Inputs)
		}
	}
}
