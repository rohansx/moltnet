package core

import (
	"crypto/ed25519"
	"fmt"
	"time"
)

// AttestationSpec is the spec tag stamped into every v0.1 attestation.
const AttestationSpec = "moltnet/attestation/v0.1"

// Attestation types recognised in v0.1.
const (
	TypeTaskCompleted   = "task.completed"
	TypeTaskDisputed    = "task.disputed"
	TypeEndorsement     = "endorsement"
	TypeIncident        = "incident"
	TypePaymentReceipt  = "payment.receipt"
	TypeKeyRotation     = "key.rotation"
	TypeSelfClaim       = "self.claim"
)

// ValidType reports whether t is a known v0.1 attestation type.
func ValidType(t string) bool {
	switch t {
	case TypeTaskCompleted, TypeTaskDisputed, TypeEndorsement, TypeIncident,
		TypePaymentReceipt, TypeKeyRotation, TypeSelfClaim:
		return true
	default:
		return false
	}
}

// Anchor is an optional external timestamp anchor (Rekor entry or RFC 3161).
type Anchor struct {
	Kind     string `json:"kind"`
	LogIndex int64  `json:"log_index,omitempty"`
	Ref      string `json:"ref,omitempty"`
}

// Attestation is a signed statement by one identity (issuer) about another
// (subject). Attestations are the raw material of reputation. Each one chains
// to the issuer's previous attestation (any subject) via a per-issuer hash
// chain, so history cannot be silently reordered or retracted.
type Attestation struct {
	Spec        string         `json:"spec"`
	Type        string         `json:"type"`
	Subject     string         `json:"subject"`      // did:key of the subject agent
	SubjectCard string         `json:"subject_card"` // card hash at time of attestation
	Issuer      string         `json:"issuer"`       // did:key of the issuer
	Prev        string         `json:"prev,omitempty"` // hash of issuer's previous attestation
	Body        map[string]any `json:"body,omitempty"`
	IssuedAt    string         `json:"issued_at"`
	Anchor      *Anchor        `json:"anchor,omitempty"`
	Sig         string         `json:"sig,omitempty"`
}

// NewAttestation builds an unsigned attestation with spec tag and timestamp set.
func NewAttestation(typ, issuerDID, subjectDID string) *Attestation {
	return &Attestation{
		Spec:     AttestationSpec,
		Type:     typ,
		Issuer:   issuerDID,
		Subject:  subjectDID,
		IssuedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// SigningPayload is the canonical attestation without its signature.
func (a *Attestation) SigningPayload() ([]byte, error) {
	return CanonicalizeWithout(a, "sig")
}

// Hash returns the content address (BLAKE3 of the signing payload). This is the
// value a subsequent attestation references in its Prev field.
func (a *Attestation) Hash() (string, error) {
	payload, err := a.SigningPayload()
	if err != nil {
		return "", err
	}
	return HashBytes(payload), nil
}

// Sign fills in the issuer signature.
func (a *Attestation) Sign(issuerKey ed25519.PrivateKey) error {
	payload, err := a.SigningPayload()
	if err != nil {
		return err
	}
	a.Sig = Sign(issuerKey, payload)
	return nil
}

// Verify checks structural invariants and the issuer signature.
func (a *Attestation) Verify() error {
	if a.Spec != AttestationSpec {
		return fmt.Errorf("attestation: unexpected spec %q", a.Spec)
	}
	if !ValidType(a.Type) {
		return fmt.Errorf("attestation: unknown type %q", a.Type)
	}
	if a.Issuer == "" || a.Subject == "" {
		return fmt.Errorf("attestation: issuer and subject are required")
	}
	if a.Sig == "" {
		return fmt.Errorf("attestation: missing issuer signature")
	}
	payload, err := a.SigningPayload()
	if err != nil {
		return err
	}
	if err := Verify(a.Issuer, payload, a.Sig); err != nil {
		return fmt.Errorf("attestation: issuer signature invalid: %w", err)
	}
	return nil
}
