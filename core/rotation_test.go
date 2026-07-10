package core

import "testing"

func TestRotationSignVerify(t *testing.T) {
	owner, _ := GenerateKeyPair()
	oldAgent, _ := GenerateKeyPair()
	newAgent, _ := GenerateKeyPair()

	r := NewRotation(owner.DID, oldAgent.DID, newAgent.DID)
	if err := r.Sign(owner.Private); err != nil {
		t.Fatal(err)
	}
	if err := r.Verify(); err != nil {
		t.Fatalf("valid rotation rejected: %v", err)
	}

	// Tampering with the target key must break verification.
	r.NewAgent = "did:key:zEvil"
	if err := r.Verify(); err == nil {
		t.Fatal("expected verification failure after tampering new_agent")
	}
}

func TestRotationMustBeSignedByOwner(t *testing.T) {
	owner, _ := GenerateKeyPair()
	oldAgent, _ := GenerateKeyPair()
	newAgent, _ := GenerateKeyPair()

	r := NewRotation(owner.DID, oldAgent.DID, newAgent.DID)
	// Sign with the OLD AGENT key instead of the owner key — must not verify.
	if err := r.Sign(oldAgent.Private); err != nil {
		t.Fatal(err)
	}
	if err := r.Verify(); err == nil {
		t.Fatal("expected rotation signed by non-owner to fail verification")
	}
}

func TestResolveCurrentAgent(t *testing.T) {
	owner, _ := GenerateKeyPair()
	a0, _ := GenerateKeyPair()
	a1, _ := GenerateKeyPair()
	a2, _ := GenerateKeyPair()

	r1 := NewRotation(owner.DID, a0.DID, a1.DID)
	_ = r1.Sign(owner.Private)
	r2 := NewRotation(owner.DID, a1.DID, a2.DID)
	_ = r2.Sign(owner.Private)

	// From a0, following the chain should resolve to a2.
	got, err := ResolveCurrentAgent([]*Rotation{r1, r2}, a0.DID)
	if err != nil {
		t.Fatal(err)
	}
	if got != a2.DID {
		t.Fatalf("resolve: got %s want %s", got, a2.DID)
	}

	// An unrotated DID resolves to itself.
	got, _ = ResolveCurrentAgent([]*Rotation{r1, r2}, "did:key:zUnknown")
	if got != "did:key:zUnknown" {
		t.Fatalf("unrotated DID should resolve to itself, got %s", got)
	}
}

func TestResolveDetectsCycle(t *testing.T) {
	owner, _ := GenerateKeyPair()
	a0, _ := GenerateKeyPair()
	a1, _ := GenerateKeyPair()

	r1 := NewRotation(owner.DID, a0.DID, a1.DID)
	_ = r1.Sign(owner.Private)
	r2 := NewRotation(owner.DID, a1.DID, a0.DID) // cycle back
	_ = r2.Sign(owner.Private)

	if _, err := ResolveCurrentAgent([]*Rotation{r1, r2}, a0.DID); err == nil {
		t.Fatal("expected cycle detection error")
	}
}
