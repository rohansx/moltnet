package score

import (
	"testing"
	"time"

	"github.com/moltnet/moltnet/core"
)

func att(typ, issuer string, at time.Time) *core.Attestation {
	a := core.NewAttestation(typ, issuer, "did:key:zSubject")
	a.IssuedAt = at.UTC().Format(time.RFC3339)
	return a
}

func TestDiversityBeatsVolume(t *testing.T) {
	now := time.Now()
	// 8 completions from one issuer.
	var oneIssuer []*core.Attestation
	for i := 0; i < 8; i++ {
		oneIssuer = append(oneIssuer, att(core.TypeTaskCompleted, "did:key:zA", now))
	}
	// 8 completions from 8 distinct issuers.
	var manyIssuers []*core.Attestation
	for i := 0; i < 8; i++ {
		manyIssuers = append(manyIssuers, att(core.TypeTaskCompleted, "did:key:z"+string(rune('A'+i)), now))
	}
	single := Compute(oneIssuer, nil, nil, now).Score
	diverse := Compute(manyIssuers, nil, nil, now).Score
	if diverse <= single {
		t.Fatalf("diversity should beat volume: diverse=%.2f single=%.2f", diverse, single)
	}
}

// The independence rule (v0.2): an attestation whose issuer shares an owner
// with the subject is self-dealing — wash trading or self-endorsement — and must
// count for nothing. This lives INSIDE the score function (not a server gate),
// so anyone with the issuer/subject owners can recompute the same number.
func TestOwnerDiscountBlocksWashTrading(t *testing.T) {
	now := time.Now()
	const subject = "did:key:zSubject" // att() fixes this as the subject
	indep := []*core.Attestation{
		att(core.TypeTaskCompleted, "did:key:zI1", now),
		att(core.TypeTaskCompleted, "did:key:zI2", now),
		att(core.TypeTaskCompleted, "did:key:zI3", now),
	}
	// The same three, plus three more from SIBLING agents the subject's owner controls.
	washed := append(append([]*core.Attestation{}, indep...),
		att(core.TypeTaskCompleted, "did:key:zSib1", now),
		att(core.TypeTaskCompleted, "did:key:zSib2", now),
		att(core.TypeTaskCompleted, "did:key:zSib3", now),
	)
	ownerOf := map[string]string{
		subject:         "did:key:zOwner", // siblings share this owner…
		"did:key:zSib1": "did:key:zOwner",
		"did:key:zSib2": "did:key:zOwner",
		"did:key:zSib3": "did:key:zOwner",
		"did:key:zI1":   "did:key:zOwnerA", // …independents do not.
		"did:key:zI2":   "did:key:zOwnerB",
		"did:key:zI3":   "did:key:zOwnerC",
	}

	honest := Compute(indep, nil, ownerOf, now).Score
	discounted := Compute(washed, nil, ownerOf, now)
	if discounted.Score != honest {
		t.Fatalf("self-owned attestations must not inflate the score: honest=%.1f washed=%.1f", honest, discounted.Score)
	}
	if discounted.Inputs.Completions != 3 || discounted.Inputs.DistinctIssuers != 3 {
		t.Fatalf("self-dealing records must be dropped from inputs: got completions=%d issuers=%d",
			discounted.Inputs.Completions, discounted.Inputs.DistinctIssuers)
	}
	// Sanity: with no owner map (the trustless uniform basis) the siblings DO inflate,
	// which is exactly why the registry supplies owners and molt verify documents the gap.
	if naive := Compute(washed, nil, nil, now).Score; naive <= honest {
		t.Fatalf("without the owner map the extra siblings should inflate: naive=%.1f honest=%.1f", naive, honest)
	}
}

func TestSelfClaimIsZero(t *testing.T) {
	now := time.Now()
	base := Compute(nil, nil, nil, now).Score
	withClaims := Compute([]*core.Attestation{
		att(core.TypeSelfClaim, "did:key:zSubject", now),
		att(core.TypeSelfClaim, "did:key:zSubject", now),
	}, nil, nil, now).Score
	if withClaims != base {
		t.Fatalf("self-claims must not change the score: base=%.2f with=%.2f", base, withClaims)
	}
}

func TestIncidentsLowerScore(t *testing.T) {
	now := time.Now()
	good := Compute([]*core.Attestation{
		att(core.TypeTaskCompleted, "did:key:zA", now),
		att(core.TypeTaskCompleted, "did:key:zB", now),
	}, nil, nil, now).Score
	withIncident := Compute([]*core.Attestation{
		att(core.TypeTaskCompleted, "did:key:zA", now),
		att(core.TypeTaskCompleted, "did:key:zB", now),
		att(core.TypeIncident, "did:key:zC", now),
	}, nil, nil, now).Score
	if withIncident >= good {
		t.Fatalf("an incident should lower the score: good=%.2f with=%.2f", good, withIncident)
	}
}

func TestFreshIssuersWeightedLow(t *testing.T) {
	now := time.Now()
	atts := []*core.Attestation{att(core.TypeTaskCompleted, "did:key:zFresh", now)}
	trustless := Compute(atts, nil, nil, now).Score
	// Same attestation, but the issuer is unknown to the registry (weight 0.25).
	weighted := Compute(atts, map[string]float64{}, nil, now).Score
	if weighted >= trustless {
		t.Fatalf("unknown issuer should count for less: weighted=%.2f trustless=%.2f", weighted, trustless)
	}
}
