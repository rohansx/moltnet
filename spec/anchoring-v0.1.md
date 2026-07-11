# On-chain Anchoring — `anchors.erc8004` — v0.1

Status: draft, tracks the reference implementation in `core/anchor.go`.

MoltNet identity is an Ed25519 keypair; reputation is a chain of signed
attestations. **Anchoring** is an *optional* bridge from that off-chain identity
to an on-chain one, so an agent can carry a single portable identity across both
worlds. v0.1 defines one anchor type: [ERC-8004][erc8004] ("Trustless Agents"),
whose Identity Registry assigns an agent a numeric id on an EVM chain.

An anchor is **a claim, not a trusted fact.** It lives inside the card, so it is
covered by both the agent and owner signatures — the owner is cryptographically
asserting "this DID corresponds to that on-chain agent id." MoltNet validates
that the claim is *well-formed* and surfaces it. A verifier who trusts the chain
can then independently check that the on-chain entry points back at this card;
until they do, the binding is only as trustworthy as the card's signers. Trust
still lives in signatures, never in the registry.

## Shape

```jsonc
{
  "anchors": {
    "erc8004": {
      "chain":    "eip155:8453",                                  // required, CAIP-2
      "registry": "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",   // required, Identity Registry
      "agent_id": "42",                                           // required, uint256 as string
      "tx":       "0x…64 hex…",                                   // optional, anchoring tx
      "card_uri": "https://…"                                     // optional, on-chain → card pointer
    }
  }
}
```

| field | type | required | rules |
|---|---|---|---|
| `chain` | string | ✓ | CAIP-2 `eip155:<n>`, no leading zero (e.g. `eip155:1`, `eip155:8453`) |
| `registry` | string | ✓ | ERC-8004 Identity Registry address; 20 hex bytes, EIP-55 checksum enforced when mixed-case |
| `agent_id` | string | ✓ | on-chain agent id (uint256). Kept as a **decimal string** so large ids survive JSON canonicalization; a JSON integer is accepted on input and normalized |
| `tx` | string | | anchoring transaction hash, `0x` + 64 hex |
| `card_uri` | string | | the off-chain pointer the on-chain entry advertises, letting a verifier confirm the reverse link |

## Validation

An `erc8004` anchor that is present but malformed makes the **whole card
invalid** (`Card.Verify` fails), so a registry rejects it at ingest exactly like
a bad signature. A card with no `erc8004` anchor, or with other unrecognized
anchor keys, is unaffected.

- `registry` given all-lowercase or all-uppercase is accepted and normalized to
  its EIP-55 mixed-case form. A genuinely mixed-case address must already carry a
  correct EIP-55 checksum, so a transcription typo is rejected rather than
  silently accepted. (EIP-55 is computed with Keccak-256; MoltNet ships its own
  dependency-free implementation.)
- `agent_id` must be a non-negative decimal integer with no leading zeros.

## Derived identifiers

Two identifiers are computed from a validated anchor and surfaced on the agent
response (`GET /v1/agents/{did}` → `anchor`):

- **CAIP-10 account**: `<chain>:<checksummed-registry>` — the Identity Registry
  contract as a chain-agnostic account id.
- **Ref**: `<chain>:<checksummed-registry>/<agent_id>` — a stable, globally
  unique reference to the on-chain agent entry. Two cards anchoring the same
  on-chain identity produce byte-identical refs, so anchors are comparable across
  instances and federated peers.

## Surfaced response

```jsonc
// GET /v1/agents/{did}
{
  "card":  { … },
  "score": { … },
  "anchor": {
    "protocol": "erc8004",
    "chain":    "eip155:8453",
    "registry": "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
    "agent_id": "42",
    "caip10":   "eip155:8453:0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
    "ref":      "eip155:8453:0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed/42"
  }
}
```

## Not in v0.1

Reading the chain to confirm the on-chain entry resolves back to this card
(true bidirectional verification) is deliberately out of scope for v0.1: it
requires an EVM RPC endpoint and would put network trust on the verification
path. v0.1 delivers the signed, well-formed, canonically-referenced claim; live
on-chain resolution is a bounded, opt-in follow-up.

[erc8004]: https://eips.ethereum.org/  "ERC-8004: Trustless Agents"
