package main

import (
	"strings"
	"testing"

	"github.com/moltnet/moltnet/core"
)

// checkSubjectBinding is what stands between "verified ✓" and a registry that
// simply answered a different question than the one asked. Every signature in a
// substitution attack is genuine — only the binding to the requested DID is
// wrong — so signature checks alone can never catch it.
func TestCheckSubjectBinding(t *testing.T) {
	const did = "did:key:zAlice"
	const other = "did:key:zBob"

	att := func(subject string) *core.Attestation {
		a := core.NewAttestation(core.TypeTaskCompleted, "did:key:zIssuer", subject)
		return a
	}
	card := func(id string) *core.Card { return &core.Card{ID: id} }

	tests := []struct {
		name    string
		card    *core.Card
		atts    []*core.Attestation
		wantErr string
	}{
		{
			name: "honest registry: card and attestations are all about the requested did",
			card: card(did),
			atts: []*core.Attestation{att(did), att(did)},
		},
		{
			name:    "identity substitution: registry returns someone else's card",
			card:    card(other),
			atts:    []*core.Attestation{att(other)},
			wantErr: "returned a card for",
		},
		{
			name:    "history substitution: right card, but another agent's track record",
			card:    card(did),
			atts:    []*core.Attestation{att(did), att(other)},
			wantErr: "is about",
		},
		{
			name:    "missing card cannot be verified",
			card:    nil,
			atts:    nil,
			wantErr: "not found",
		},
		{
			name: "no attestations is honest — an agent may simply have no history",
			card: card(did),
			atts: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := checkSubjectBinding(did, tc.card, tc.atts)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected binding to hold, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error containing %q, got nil — substitution would pass as verified", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q should mention %q", err, tc.wantErr)
			}
		})
	}
}
