package core

import (
	"fmt"
	"sort"
)

// VerifyIssuerChain verifies a single issuer's per-issuer hash chain. The input
// is that issuer's attestations in chain order (oldest first). It checks that:
//   - every attestation is signed by the stated issuer,
//   - every attestation is issued by the same issuer,
//   - the first attestation has an empty Prev,
//   - each subsequent Prev equals the hash of the preceding attestation.
//
// This makes it impossible for an issuer to silently retract or reorder its own
// history: any tampering breaks the chain.
func VerifyIssuerChain(chain []*Attestation) error {
	var prevHash string
	for i, att := range chain {
		if att.Issuer != chain[0].Issuer {
			return fmt.Errorf("chain: attestation %d has issuer %s, expected %s", i, att.Issuer, chain[0].Issuer)
		}
		if err := att.Verify(); err != nil {
			return fmt.Errorf("chain: attestation %d: %w", i, err)
		}
		if att.Prev != prevHash {
			return fmt.Errorf("chain: attestation %d prev=%q, expected %q", i, att.Prev, prevHash)
		}
		h, err := att.Hash()
		if err != nil {
			return err
		}
		prevHash = h
	}
	return nil
}

// GroupByIssuer partitions attestations by issuer DID, preserving input order
// within each group.
func GroupByIssuer(atts []*Attestation) map[string][]*Attestation {
	out := make(map[string][]*Attestation)
	for _, a := range atts {
		out[a.Issuer] = append(out[a.Issuer], a)
	}
	return out
}

// VerifyAll verifies the chains of every issuer present in atts. Attestations
// are grouped by issuer and, within each group, sorted by issued_at before the
// chain link check so callers can pass an unordered set.
func VerifyAll(atts []*Attestation) error {
	for issuer, group := range GroupByIssuer(atts) {
		sorted := make([]*Attestation, len(group))
		copy(sorted, group)
		sort.SliceStable(sorted, func(i, j int) bool {
			return sorted[i].IssuedAt < sorted[j].IssuedAt
		})
		if err := VerifyIssuerChain(sorted); err != nil {
			return fmt.Errorf("issuer %s: %w", issuer, err)
		}
	}
	return nil
}
