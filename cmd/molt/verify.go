package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/moltnet/moltnet/core"
	"github.com/moltnet/moltnet/score"
)

// checkSubjectBinding proves the registry answered the question we asked.
//
// Signature checks alone cannot do this: in a substitution attack every record
// is genuinely signed and every chain is intact — the registry simply hands
// back a different agent's card, or another agent's track record filed under
// the requested DID. Both pass `card.Verify()` and `core.VerifyAll()` and would
// print "verified ✓" while reporting a stranger's reputation. The binding to
// the requested DID is the one property the registry chooses, so it is the one
// property that must be checked before anything else is believed.
func checkSubjectBinding(did string, card *core.Card, atts []*core.Attestation) error {
	if card == nil {
		return fmt.Errorf("agent %s not found", did)
	}
	if card.ID != did {
		return fmt.Errorf("registry returned a card for %s, but %s was requested — do not trust this registry", card.ID, did)
	}
	for _, a := range atts {
		if a.Subject != did {
			h, _ := a.Hash()
			return fmt.Errorf("attestation %s is about %s, not the requested %s — do not trust this registry", h, a.Subject, did)
		}
	}
	return nil
}

// cmdVerify is the flagship command. It pulls an agent's entire history from a
// registry and proves it locally: every card and attestation signature is
// checked, every issuer chain is verified, and the MoltScore is recomputed from
// scratch — trusting the registry for nothing but transport.
func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	registry := fs.String("registry", "", "registry base URL")
	positional := parseInterspersed(fs, args)
	if len(positional) < 1 {
		return fmt.Errorf("usage: molt verify <did>")
	}
	did := positional[0]
	reg := registryURL(*registry)

	card, atts, err := fetchAgent(reg, did)
	if err != nil {
		return err
	}
	if err := checkSubjectBinding(did, card, atts); err != nil {
		return err
	}

	fmt.Printf("VERIFY  %s\n", did)
	fmt.Printf("registry %s  (trusted for transport only)\n\n", reg)

	// 1. Card signatures.
	cardOK := true
	if err := card.Verify(); err != nil {
		cardOK = false
		fmt.Printf("  [FAIL] card signatures: %v\n", err)
	} else {
		hash, _ := card.Hash()
		fmt.Printf("  [ ok ] card signatures (agent + owner)\n")
		fmt.Printf("         name=%q version=%s hash=%s\n", card.Name, card.Version, hash)
	}

	// 2. Attestation signatures + per-issuer chains.
	chainErr := core.VerifyAll(atts)
	if chainErr != nil {
		fmt.Printf("  [FAIL] attestation chains: %v\n", chainErr)
	} else {
		fmt.Printf("  [ ok ] %d attestation(s), all signatures valid, all issuer chains intact\n", len(atts))
	}

	// Per-attestation summary.
	for _, a := range atts {
		status := "ok"
		if a.Verify() != nil {
			status = "BAD"
		}
		fmt.Printf("         [%s] %-15s from %s…\n", status, a.Type, short(a.Issuer))
	}

	// 3. Recompute MoltScore locally with default (trustless) issuer weights.
	out := score.Compute(atts, nil, nil, time.Now().UTC())
	fmt.Printf("\n  MoltScore (recomputed locally, %s): %s\n", score.Algorithm, scoreLine(out))

	if !cardOK || chainErr != nil {
		return fmt.Errorf("verification failed")
	}
	fmt.Printf("\n  RESULT: verified ✓  (no trust placed in the registry)\n")
	return nil
}

func short(did string) string {
	if len(did) <= 16 {
		return did
	}
	return did[:16]
}
