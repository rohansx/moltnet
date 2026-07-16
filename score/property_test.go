package score

import (
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/moltnet/moltnet/core"
)

// TestScoreProperties asserts MoltScore v1's structural invariants over many
// randomly generated attestation sets (seeded PRNG, so it is reproducible):
//   - the score is always within [0, 100],
//   - a self.claim never changes the score,
//   - an incident never raises the score,
//   - a completion from a fresh issuer never lowers the score.
func TestScoreProperties(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	now := time.Unix(1_760_000_000, 0).UTC()
	iso := now.Format(time.RFC3339)
	types := []string{
		core.TypeTaskCompleted, core.TypeEndorsement, core.TypeIncident,
		core.TypeTaskDisputed, core.TypePaymentReceipt, core.TypeSelfClaim,
	}
	mk := func(typ, issuer string) *core.Attestation {
		a := core.NewAttestation(typ, issuer, "did:key:zSubject")
		a.IssuedAt = iso
		return a
	}
	with := func(base []*core.Attestation, extra *core.Attestation) []*core.Attestation {
		out := make([]*core.Attestation, len(base), len(base)+1)
		copy(out, base)
		return append(out, extra)
	}

	for i := 0; i < 300; i++ {
		n := rng.Intn(12)
		var atts []*core.Attestation
		for j := 0; j < n; j++ {
			atts = append(atts, mk(types[rng.Intn(len(types))], "did:key:z"+strconv.Itoa(rng.Intn(6))))
		}
		base := Compute(atts, nil, nil, now).Score

		if base < 0 || base > 100 {
			t.Fatalf("score out of range: %v", base)
		}
		if s := Compute(with(atts, mk(core.TypeSelfClaim, "did:key:zClaim")), nil, nil, now).Score; s != base {
			t.Fatalf("self.claim changed score: %v -> %v", base, s)
		}
		if s := Compute(with(atts, mk(core.TypeIncident, "did:key:zInc")), nil, nil, now).Score; s > base {
			t.Fatalf("incident raised score: %v -> %v", base, s)
		}
		if s := Compute(with(atts, mk(core.TypeTaskCompleted, "did:key:zFresh")), nil, nil, now).Score; s < base {
			t.Fatalf("fresh completion lowered score: %v -> %v", base, s)
		}
	}
}
