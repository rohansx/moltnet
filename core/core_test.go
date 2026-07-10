package core

import "testing"

func TestDIDRoundTrip(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pub, err := PublicKeyFromDID(kp.DID)
	if err != nil {
		t.Fatalf("decode did: %v", err)
	}
	if string(pub) != string(kp.Public) {
		t.Fatal("recovered public key does not match")
	}
}

func TestCardSignVerify(t *testing.T) {
	owner, _ := GenerateKeyPair()
	agent, _ := GenerateKeyPair()
	c := NewCard(agent.DID, owner.DID, "test-agent")
	c.Capabilities = []Capability{{Tag: "privacy.redaction", Desc: "x"}}
	if err := c.Sign(agent.Private, owner.Private); err != nil {
		t.Fatal(err)
	}
	if err := c.Verify(); err != nil {
		t.Fatalf("verify: %v", err)
	}
	// Tampering must break verification.
	c.Name = "evil"
	if err := c.Verify(); err == nil {
		t.Fatal("expected verification to fail after tampering")
	}
}

func TestAttestationChain(t *testing.T) {
	issuer, _ := GenerateKeyPair()
	subject, _ := GenerateKeyPair()

	a1 := NewAttestation(TypeTaskCompleted, issuer.DID, subject.DID)
	a1.IssuedAt = "2026-07-01T00:00:00Z"
	if err := a1.Sign(issuer.Private); err != nil {
		t.Fatal(err)
	}
	h1, _ := a1.Hash()

	a2 := NewAttestation(TypeTaskCompleted, issuer.DID, subject.DID)
	a2.IssuedAt = "2026-07-02T00:00:00Z"
	a2.Prev = h1
	if err := a2.Sign(issuer.Private); err != nil {
		t.Fatal(err)
	}

	if err := VerifyIssuerChain([]*Attestation{a1, a2}); err != nil {
		t.Fatalf("valid chain rejected: %v", err)
	}

	// Break the link.
	a2.Prev = "blake3:deadbeef"
	if err := a2.Sign(issuer.Private); err != nil {
		t.Fatal(err)
	}
	if err := VerifyIssuerChain([]*Attestation{a1, a2}); err == nil {
		t.Fatal("expected broken chain to be rejected")
	}
}
