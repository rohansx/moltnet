package core

import (
	"crypto/ed25519"
	"fmt"
	"time"
)

// CardSpec is the spec tag stamped into every v0.1 Agent Card.
const CardSpec = "moltnet/card/v0.1"

// Capability is a namespaced capability tag plus a free-text description.
type Capability struct {
	Tag  string `json:"tag"`
	Desc string `json:"desc,omitempty"`
}

// Liveness is an agent's opt-in health-probe configuration. When enabled, a
// registry periodically probes URL and records reachability + latency. This is
// an observable signal, displayed on profiles but deliberately kept OUT of
// MoltScore so the score stays purely attestation-derived and recomputable.
type Liveness struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}

// Card is the canonical Agent Card: one JSON document describing an agent's
// identity, capabilities, endpoints and protocol bindings. It is content
// addressed (BLAKE3 of the canonical signing payload) and doubly signed by the
// agent key and the owner key.
type Card struct {
	Spec         string                 `json:"spec"`
	ID           string                 `json:"id"`    // did:key of the agent
	Name         string                 `json:"name"`
	Owner        string                 `json:"owner"` // did:key of the owner
	Description  string                 `json:"description,omitempty"`
	Version      string                 `json:"version,omitempty"`
	Prev         string                 `json:"prev,omitempty"` // hash of the previous card version ("" for genesis)
	Capabilities []Capability           `json:"capabilities,omitempty"`
	Protocols    map[string]any         `json:"protocols,omitempty"`
	Anchors      map[string]any         `json:"anchors,omitempty"`
	Links        map[string]string      `json:"links,omitempty"`
	PricingHint  map[string]any         `json:"pricing_hint,omitempty"`
	Liveness     *Liveness              `json:"liveness,omitempty"`
	CreatedAt    string                 `json:"created_at"`
	Sig          string                 `json:"sig,omitempty"`       // agent key signature
	OwnerSig     string                 `json:"owner_sig,omitempty"` // owner key signature
}

// NewCard builds an unsigned card with the spec tag and creation timestamp set.
func NewCard(agentDID, ownerDID, name string) *Card {
	return &Card{
		Spec:      CardSpec,
		ID:        agentDID,
		Owner:     ownerDID,
		Name:      name,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// SigningPayload is the canonical card with signature fields removed. Both the
// agent and owner signatures are computed over this payload.
func (c *Card) SigningPayload() ([]byte, error) {
	return CanonicalizeWithout(c, "sig", "owner_sig")
}

// Hash returns the content address of the card: BLAKE3 of the signing payload.
// This is stable regardless of signature bytes so a card has one identity.
func (c *Card) Hash() (string, error) {
	payload, err := c.SigningPayload()
	if err != nil {
		return "", err
	}
	return HashBytes(payload), nil
}

// Sign fills in the agent and owner signatures.
func (c *Card) Sign(agentKey, ownerKey ed25519.PrivateKey) error {
	payload, err := c.SigningPayload()
	if err != nil {
		return err
	}
	c.Sig = Sign(agentKey, payload)
	c.OwnerSig = Sign(ownerKey, payload)
	return nil
}

// Verify checks structural invariants and both signatures.
func (c *Card) Verify() error {
	if c.Spec != CardSpec {
		return fmt.Errorf("card: unexpected spec %q", c.Spec)
	}
	if c.ID == "" || c.Owner == "" {
		return fmt.Errorf("card: id and owner are required")
	}
	if c.Sig == "" {
		return fmt.Errorf("card: missing agent signature")
	}
	if c.OwnerSig == "" {
		return fmt.Errorf("card: missing owner signature")
	}
	payload, err := c.SigningPayload()
	if err != nil {
		return err
	}
	if err := Verify(c.ID, payload, c.Sig); err != nil {
		return fmt.Errorf("card: agent signature invalid: %w", err)
	}
	if err := Verify(c.Owner, payload, c.OwnerSig); err != nil {
		return fmt.Errorf("card: owner signature invalid: %w", err)
	}
	return nil
}
