package store

import (
	"testing"

	"github.com/moltnet/moltnet/core"
)

func signedCard(t *testing.T, owner, agent *core.KeyPair, name, prev string) *core.Card {
	t.Helper()
	c := core.NewCard(agent.DID, owner.DID, name)
	c.Prev = prev
	if err := c.Sign(agent.Private, owner.Private); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCardForkDetection(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	owner, _ := core.GenerateKeyPair()
	agent, _ := core.GenerateKeyPair()

	// Genesis (prev="").
	genesis := signedCard(t, owner, agent, "v1", "")
	if changed, err := st.PutCard(genesis); err != nil || !changed {
		t.Fatalf("genesis: changed=%v err=%v", changed, err)
	}
	gh, _ := genesis.Hash()

	// Linear update off genesis -> becomes the head, no fork.
	update := signedCard(t, owner, agent, "v2", gh)
	if changed, err := st.PutCard(update); err != nil || !changed {
		t.Fatalf("update: changed=%v err=%v", changed, err)
	}
	if f, _ := st.GetFork(agent.DID); f != nil {
		t.Fatalf("did not expect a fork after a linear update, got %+v", f)
	}
	if cur, _ := st.GetCard(agent.DID); cur == nil || cur.Name != "v2" {
		t.Fatalf("head should be v2 after linear update")
	}

	// Competing branch: another child of genesis (prev=gh) that is NOT the head.
	fork := signedCard(t, owner, agent, "vFork", gh)
	if _, err := st.PutCard(fork); err != nil {
		t.Fatalf("fork put: %v", err)
	}
	f, err := st.GetFork(agent.DID)
	if err != nil {
		t.Fatal(err)
	}
	if f == nil {
		t.Fatal("expected a fork to be detected")
	}
	// The head must NOT move to the fork; it stays at the linear update (v2).
	if cur, _ := st.GetCard(agent.DID); cur == nil || cur.Name != "v2" {
		t.Fatalf("head must stay at v2 after a fork, got %v", cur)
	}
	fh, _ := fork.Hash()
	if f.CompetingHash != fh {
		t.Fatalf("fork competing hash = %s, want %s", f.CompetingHash, fh)
	}

	// Re-submitting the exact same fork card must not create a duplicate fork.
	if _, err := st.PutCard(fork); err != nil {
		t.Fatal(err)
	}
}
