package core

import (
	"crypto/ed25519"
	"fmt"
	"time"
)

// RotationSpec is the spec tag for a v0.1 key rotation record.
const RotationSpec = "moltnet/rotation/v0.1"

// Rotation is an owner-signed record that rotates an agent's key: it links an
// old agent DID to a new one. Because it is signed by the *owner* key (which is
// separate from the agent key), an agent-key compromise does not destroy the
// identity — the owner rotates to a fresh agent key and the attestation history
// continues across the rotation.
type Rotation struct {
	Spec     string `json:"spec"`
	Owner    string `json:"owner"`     // owner DID that authorizes the rotation
	OldAgent string `json:"old_agent"` // agent DID being retired
	NewAgent string `json:"new_agent"` // replacement agent DID
	IssuedAt string `json:"issued_at"`
	Sig      string `json:"sig,omitempty"` // owner signature
}

// NewRotation builds an unsigned rotation with the spec tag and timestamp set.
func NewRotation(ownerDID, oldAgentDID, newAgentDID string) *Rotation {
	return &Rotation{
		Spec:     RotationSpec,
		Owner:    ownerDID,
		OldAgent: oldAgentDID,
		NewAgent: newAgentDID,
		IssuedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// SigningPayload is the canonical rotation without its signature.
func (r *Rotation) SigningPayload() ([]byte, error) {
	return CanonicalizeWithout(r, "sig")
}

// Hash returns the content address of the rotation record.
func (r *Rotation) Hash() (string, error) {
	payload, err := r.SigningPayload()
	if err != nil {
		return "", err
	}
	return HashBytes(payload), nil
}

// Sign fills in the owner signature.
func (r *Rotation) Sign(ownerKey ed25519.PrivateKey) error {
	payload, err := r.SigningPayload()
	if err != nil {
		return err
	}
	r.Sig = Sign(ownerKey, payload)
	return nil
}

// Verify checks structural invariants and the owner signature.
func (r *Rotation) Verify() error {
	if r.Spec != RotationSpec {
		return fmt.Errorf("rotation: unexpected spec %q", r.Spec)
	}
	if r.Owner == "" || r.OldAgent == "" || r.NewAgent == "" {
		return fmt.Errorf("rotation: owner, old_agent and new_agent are required")
	}
	if r.OldAgent == r.NewAgent {
		return fmt.Errorf("rotation: old_agent and new_agent must differ")
	}
	if r.Sig == "" {
		return fmt.Errorf("rotation: missing owner signature")
	}
	payload, err := r.SigningPayload()
	if err != nil {
		return err
	}
	if err := Verify(r.Owner, payload, r.Sig); err != nil {
		return fmt.Errorf("rotation: owner signature invalid: %w", err)
	}
	return nil
}

// ResolveCurrentAgent follows a set of rotations from a starting DID to the
// current (most recently rotated-to) agent DID. A DID with no rotation resolves
// to itself. Returns an error if the rotations form a cycle.
func ResolveCurrentAgent(rotations []*Rotation, start string) (string, error) {
	next := map[string]string{}
	for _, r := range rotations {
		next[r.OldAgent] = r.NewAgent
	}
	seen := map[string]bool{start: true}
	current := start
	for {
		n, ok := next[current]
		if !ok {
			return current, nil
		}
		if seen[n] {
			return "", fmt.Errorf("rotation cycle detected at %s", n)
		}
		seen[n] = true
		current = n
	}
}
