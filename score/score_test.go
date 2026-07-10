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
	single := Compute(oneIssuer, nil, now).Score
	diverse := Compute(manyIssuers, nil, now).Score
	if diverse <= single {
		t.Fatalf("diversity should beat volume: diverse=%.2f single=%.2f", diverse, single)
	}
}

func TestSelfClaimIsZero(t *testing.T) {
	now := time.Now()
	base := Compute(nil, nil, now).Score
	withClaims := Compute([]*core.Attestation{
		att(core.TypeSelfClaim, "did:key:zSubject", now),
		att(core.TypeSelfClaim, "did:key:zSubject", now),
	}, nil, now).Score
	if withClaims != base {
		t.Fatalf("self-claims must not change the score: base=%.2f with=%.2f", base, withClaims)
	}
}

func TestIncidentsLowerScore(t *testing.T) {
	now := time.Now()
	good := Compute([]*core.Attestation{
		att(core.TypeTaskCompleted, "did:key:zA", now),
		att(core.TypeTaskCompleted, "did:key:zB", now),
	}, nil, now).Score
	withIncident := Compute([]*core.Attestation{
		att(core.TypeTaskCompleted, "did:key:zA", now),
		att(core.TypeTaskCompleted, "did:key:zB", now),
		att(core.TypeIncident, "did:key:zC", now),
	}, nil, now).Score
	if withIncident >= good {
		t.Fatalf("an incident should lower the score: good=%.2f with=%.2f", good, withIncident)
	}
}

func TestFreshIssuersWeightedLow(t *testing.T) {
	now := time.Now()
	atts := []*core.Attestation{att(core.TypeTaskCompleted, "did:key:zFresh", now)}
	trustless := Compute(atts, nil, now).Score
	// Same attestation, but the issuer is unknown to the registry (weight 0.25).
	weighted := Compute(atts, map[string]float64{}, now).Score
	if weighted >= trustless {
		t.Fatalf("unknown issuer should count for less: weighted=%.2f trustless=%.2f", weighted, trustless)
	}
}
