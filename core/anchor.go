package core

import (
	"fmt"
	"strings"
)

// ERC-8004 ("Trustless Agents") lets an agent identity be anchored to an
// on-chain Identity Registry entry on an EVM chain. A MoltNet card MAY carry
// such an anchor under `anchors.erc8004`. The anchor is a *claim*, signed by the
// agent and owner keys along with the rest of the card: it asserts "this DID
// corresponds to that on-chain agent id." MoltNet validates the claim is
// well-formed and surfaces it; a verifier that trusts the chain can then check
// the on-chain entry points back at this card. Trust still lives in signatures,
// not in the registry.

// AnchorERC8004 is the key under which the ERC-8004 anchor lives in card.Anchors.
const AnchorERC8004 = "erc8004"

// ERC8004Anchor is a parsed, validated ERC-8004 identity anchor.
type ERC8004Anchor struct {
	// Chain is a CAIP-2 chain identifier, always eip155:<n> (e.g. eip155:8453
	// for Base mainnet).
	Chain string
	// Registry is the ERC-8004 Identity Registry contract address, stored in
	// its EIP-55 checksummed form.
	Registry string
	// AgentID is the on-chain agent identifier (a uint256), kept as a decimal
	// string so arbitrarily large ids survive JSON canonicalization intact.
	AgentID string
	// Tx is the optional anchoring transaction hash (0x + 64 hex).
	Tx string
	// CardURI is the optional off-chain pointer the on-chain entry advertises,
	// letting a verifier confirm the on-chain side points back at this card.
	CardURI string
}

// CAIP10 returns the CAIP-10 account identifier of the registry contract:
// <chain>:<checksummed-registry>.
func (a *ERC8004Anchor) CAIP10() string {
	return a.Chain + ":" + a.Registry
}

// Ref is a stable, globally-unique reference to the on-chain agent entry:
// <chain>:<checksummed-registry>/<agentId>. Two cards anchoring the same
// on-chain identity produce identical refs.
func (a *ERC8004Anchor) Ref() string {
	return a.CAIP10() + "/" + a.AgentID
}

// View returns the surfaced JSON representation of the anchor: the validated,
// normalized fields plus the derived CAIP-10 account id and stable ref. Empty
// optional fields are omitted.
func (a *ERC8004Anchor) View() map[string]any {
	v := map[string]any{
		"protocol": AnchorERC8004,
		"chain":    a.Chain,
		"registry": a.Registry,
		"agent_id": a.AgentID,
		"caip10":   a.CAIP10(),
		"ref":      a.Ref(),
	}
	if a.Tx != "" {
		v["tx"] = a.Tx
	}
	if a.CardURI != "" {
		v["card_uri"] = a.CardURI
	}
	return v
}

// ParseERC8004 extracts and validates the ERC-8004 anchor from a card's anchors
// map. It returns (nil, false, nil) when no erc8004 anchor is present, and a
// non-nil error only when an anchor is present but malformed.
func ParseERC8004(anchors map[string]any) (*ERC8004Anchor, bool, error) {
	if anchors == nil {
		return nil, false, nil
	}
	raw, ok := anchors[AnchorERC8004]
	if !ok {
		return nil, false, nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, true, fmt.Errorf("anchor erc8004: must be an object")
	}

	chain, err := anchorString(obj, "chain", true)
	if err != nil {
		return nil, true, err
	}
	if err := validateChain(chain); err != nil {
		return nil, true, err
	}

	registryRaw, err := anchorString(obj, "registry", true)
	if err != nil {
		return nil, true, err
	}
	registry, err := ChecksumAddress(registryRaw)
	if err != nil {
		return nil, true, fmt.Errorf("anchor erc8004: registry: %w", err)
	}

	agentID, err := anchorUint(obj, "agent_id")
	if err != nil {
		return nil, true, err
	}

	tx, err := anchorString(obj, "tx", false)
	if err != nil {
		return nil, true, err
	}
	if tx != "" {
		if err := validateTxHash(tx); err != nil {
			return nil, true, err
		}
	}

	cardURI, err := anchorString(obj, "card_uri", false)
	if err != nil {
		return nil, true, err
	}

	return &ERC8004Anchor{
		Chain:    chain,
		Registry: registry,
		AgentID:  agentID,
		Tx:       tx,
		CardURI:  cardURI,
	}, true, nil
}

// validateChain checks a CAIP-2 eip155 chain identifier.
func validateChain(chain string) error {
	rest, ok := strings.CutPrefix(chain, "eip155:")
	if !ok {
		return fmt.Errorf("anchor erc8004: chain %q must be a CAIP-2 eip155 identifier (e.g. eip155:8453)", chain)
	}
	if rest == "" || !isDigits(rest) {
		return fmt.Errorf("anchor erc8004: chain %q has a non-numeric eip155 reference", chain)
	}
	if len(rest) > 1 && rest[0] == '0' {
		return fmt.Errorf("anchor erc8004: chain %q has a leading zero", chain)
	}
	return nil
}

// validateTxHash checks a 0x-prefixed 32-byte hex hash.
func validateTxHash(tx string) error {
	h, ok := strings.CutPrefix(tx, "0x")
	if !ok || len(h) != 64 || !isHex(h) {
		return fmt.Errorf("anchor erc8004: tx %q must be a 0x-prefixed 32-byte hex hash", tx)
	}
	return nil
}

// ChecksumAddress validates a 20-byte hex Ethereum address and returns it in
// EIP-55 mixed-case checksum form. All-lowercase or all-uppercase input is
// accepted (no checksum information to verify); genuinely mixed-case input must
// already carry a correct EIP-55 checksum, so a typo is rejected rather than
// silently normalized.
func ChecksumAddress(addr string) (string, error) {
	body, ok := strings.CutPrefix(addr, "0x")
	if !ok {
		return "", fmt.Errorf("address %q must be 0x-prefixed", addr)
	}
	if len(body) != 40 || !isHex(body) {
		return "", fmt.Errorf("address %q must be 20 hex bytes", addr)
	}
	lower := strings.ToLower(body)
	checksummed := eip55(lower)

	mixed := body != lower && body != strings.ToUpper(body)
	if mixed && body != checksummed {
		return "", fmt.Errorf("address %q has an invalid EIP-55 checksum", addr)
	}
	return "0x" + checksummed, nil
}

// eip55 applies the EIP-55 checksum to a lowercase, unprefixed 40-char hex
// address, returning the mixed-case body.
func eip55(lower string) string {
	hash := Keccak256([]byte(lower))
	out := []byte(lower)
	for i := 0; i < 40; i++ {
		if out[i] < 'a' || out[i] > 'f' {
			continue // digits are never uppercased
		}
		// nibble i of the hash: high nibble for even i, low for odd.
		var nibble byte
		if i%2 == 0 {
			nibble = hash[i/2] >> 4
		} else {
			nibble = hash[i/2] & 0x0f
		}
		if nibble >= 8 {
			out[i] = out[i] - 'a' + 'A'
		}
	}
	return string(out)
}

// anchorString reads a string field. If required and absent/empty it errors;
// otherwise a missing field yields "".
func anchorString(obj map[string]any, key string, required bool) (string, error) {
	v, ok := obj[key]
	if !ok {
		if required {
			return "", fmt.Errorf("anchor erc8004: missing %q", key)
		}
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("anchor erc8004: %q must be a string", key)
	}
	if required && s == "" {
		return "", fmt.Errorf("anchor erc8004: %q must not be empty", key)
	}
	return s, nil
}

// anchorUint reads the agent_id field, accepting either a decimal string or a
// JSON integer, and normalizes to a decimal string.
func anchorUint(obj map[string]any, key string) (string, error) {
	v, ok := obj[key]
	if !ok {
		return "", fmt.Errorf("anchor erc8004: missing %q", key)
	}
	switch t := v.(type) {
	case string:
		if t == "" || !isDigits(t) {
			return "", fmt.Errorf("anchor erc8004: %q must be a decimal integer", key)
		}
		if len(t) > 1 && t[0] == '0' {
			return "", fmt.Errorf("anchor erc8004: %q must not have leading zeros", key)
		}
		return t, nil
	case float64:
		if t < 0 || t != float64(int64(t)) {
			return "", fmt.Errorf("anchor erc8004: %q must be a non-negative integer", key)
		}
		return fmt.Sprintf("%d", int64(t)), nil
	default:
		return "", fmt.Errorf("anchor erc8004: %q must be a decimal string or integer", key)
	}
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return len(s) > 0
}
