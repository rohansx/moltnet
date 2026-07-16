// Package score implements MoltScore v1: a deterministic, open reputation
// function computed locally from an agent's attestation set. Given the same
// inputs, every client computes the same score. The registry serves a
// precomputed value for convenience, but nothing needs to trust it.
package score

import (
	"math"
	"sort"
	"time"

	"github.com/moltnet/moltnet/core"
)

// Algorithm is the version tag emitted with every score object.
const Algorithm = "moltscore/v1"

// Model weights and constants. Design principles (issuer weighting, diversity
// beats volume, recency decay, self-claims count zero) matter more than the
// exact figures; see spec/moltscore-v1.md.
const (
	wCompletions = 1.0
	wDisputes    = 1.2
	wIncidents   = 2.0
	wDiversity   = 0.6
	baseline     = 2.0 // shifts a no-history agent toward a low score

	endorsementWeight = 0.25 // endorsements count for less than real work
	receiptWeight     = 0.50 // x402 receipts attach economic cost to a record

	halfLifePositiveDays = 180.0
	halfLifeDisputeDays  = 180.0
	halfLifeIncidentDays = 365.0 // incidents decay slower than positive signal
)

// Inputs is the observable breakdown that fed a score.
type Inputs struct {
	Completions     int `json:"completions"`
	Disputes        int `json:"disputes"`
	Incidents       int `json:"incidents"`
	Endorsements    int `json:"endorsements"`
	Receipts        int `json:"receipts"`
	DistinctIssuers int `json:"distinct_issuers"`
}

// Output is the full score object, including the breakdown and the attestation
// head it was computed over, so a client can reproduce it.
type Output struct {
	Algorithm       string  `json:"algorithm"`
	Score           float64 `json:"score"`
	Inputs          Inputs  `json:"inputs"`
	ComputedAt      string  `json:"computed_at"`
	AttestationHead string  `json:"attestation_head"`
}

// Compute runs MoltScore v1 over an agent's attestations.
//
// issuerWeights optionally maps an issuer DID to a trust weight in [0,1]
// (typically the issuer's own normalized score). Missing issuers default to
// defaultIssuerWeight, so a farm of fresh keypairs contributes little. Passing
// nil weights everyone at 1.0, which is the correct default for a standalone,
// trustless recomputation.
//
// ownerOf optionally maps a DID (the subject and any issuer) to its controlling
// owner DID. When supplied, any attestation whose issuer shares an owner with
// the subject is DROPPED as self-dealing — wash trading or self-endorsement.
// This independence rule lives in the function, not a server gate, so anyone who
// can resolve the owners recomputes the same number; passing nil disables it
// (the trustless uniform basis, as `molt verify` uses).
func Compute(atts []*core.Attestation, issuerWeights map[string]float64, ownerOf map[string]string, now time.Time) Output {
	const defaultIssuerWeight = 1.0

	var weightedCompletions, weightedDisputes, weightedIncidents float64
	var in Inputs
	positiveIssuers := map[string]struct{}{}

	// The subject is constant across a Compute call (all attestations are ABOUT
	// the same agent); resolve its owner once for the self-dealing check.
	var subjectOwner string
	if ownerOf != nil && len(atts) > 0 {
		subjectOwner = ownerOf[atts[0].Subject]
	}

	weightOf := func(issuer string) float64 {
		if issuerWeights == nil {
			return defaultIssuerWeight
		}
		if w, ok := issuerWeights[issuer]; ok {
			return w
		}
		return 0.25 // unknown / fresh issuer: near-nothing (primary sybil defense)
	}

	for _, a := range atts {
		// Self-dealing: the issuer is controlled by the subject's own owner. Drop
		// it entirely — it contributes to no weighted sum and no diversity count.
		if subjectOwner != "" && ownerOf[a.Issuer] == subjectOwner {
			continue
		}
		iw := weightOf(a.Issuer)
		switch a.Type {
		case core.TypeTaskCompleted:
			in.Completions++
			weightedCompletions += iw * decay(a.IssuedAt, now, halfLifePositiveDays)
			positiveIssuers[a.Issuer] = struct{}{}
		case core.TypeEndorsement:
			in.Endorsements++
			weightedCompletions += endorsementWeight * iw * decay(a.IssuedAt, now, halfLifePositiveDays)
			positiveIssuers[a.Issuer] = struct{}{}
		case core.TypePaymentReceipt:
			in.Receipts++
			weightedCompletions += receiptWeight * iw * decay(a.IssuedAt, now, halfLifePositiveDays)
			positiveIssuers[a.Issuer] = struct{}{}
		case core.TypeTaskDisputed:
			in.Disputes++
			weightedDisputes += iw * decay(a.IssuedAt, now, halfLifeDisputeDays)
		case core.TypeIncident:
			in.Incidents++
			weightedIncidents += iw * decay(a.IssuedAt, now, halfLifeIncidentDays)
		case core.TypeSelfClaim:
			// Weight zero. Always. (Displayed elsewhere, never scored.)
		case core.TypeKeyRotation:
			// Continuity event, not a reputation signal.
		}
	}
	in.DistinctIssuers = len(positiveIssuers)

	x := wCompletions*math.Log(1+weightedCompletions) +
		wDiversity*math.Log(1+float64(in.DistinctIssuers)) -
		wDisputes*weightedDisputes -
		wIncidents*weightedIncidents -
		baseline

	return Output{
		Algorithm:       Algorithm,
		Score:           round1(100 * sigmoid(x)),
		Inputs:          in,
		ComputedAt:      now.UTC().Format(time.RFC3339),
		AttestationHead: head(atts),
	}
}

func sigmoid(x float64) float64 { return 1.0 / (1.0 + math.Exp(-x)) }

// decay returns 0.5^(ageDays/halfLife), clamped to (0,1]. A timestamp that
// fails to parse or is in the future contributes at full weight.
func decay(issuedAt string, now time.Time, halfLifeDays float64) float64 {
	t, err := time.Parse(time.RFC3339, issuedAt)
	if err != nil {
		return 1.0
	}
	ageDays := now.Sub(t).Hours() / 24.0
	if ageDays <= 0 {
		return 1.0
	}
	return math.Pow(0.5, ageDays/halfLifeDays)
}

func head(atts []*core.Attestation) string {
	if len(atts) == 0 {
		return ""
	}
	sorted := make([]*core.Attestation, len(atts))
	copy(sorted, atts)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].IssuedAt < sorted[j].IssuedAt
	})
	h, err := sorted[len(sorted)-1].Hash()
	if err != nil {
		return ""
	}
	return h
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
