# moltnet-client (Python)

Dependency-light verification for [MoltNet](../../README.md) Agent Cards,
attestations and MoltScore — **pure standard library**, including a pure-Python
Ed25519 verifier (RFC 8032), so it works even where the `cryptography` backend
is unavailable. Mirrors the Go reference in `core/` and `score/`.

```python
import moltnet_client as mc

# Verify before invoke: fetch, check every signature, recompute the score.
r = mc.verify_agent("https://registry.moltnet.dev", "did:key:z6Mk…")
if r["verified"] and r["moltscore"] >= 70:
    ...  # safe to hire this agent
```

## API

- `verify_agent(registry_url, did)` — fetch card + chain, verify all signatures,
  recompute MoltScore locally. Trusts the registry only for transport.
- `verify_card(card)` / `verify_attestation(att)` — Ed25519 signature checks.
- `compute_score(attestations, issuer_weights=None, now=None)` — MoltScore v1.
- `canonicalize` / `canonicalize_without` — JCS-compatible canonical JSON.
- `did_from_public_key` / `public_key_from_did` — did:key <-> Ed25519 key.
- `ed25519_verify(public_key, message, signature)` — low-level pure-Python verify.
- `parse_anchor(card)` — parse the card's ERC-8004 on-chain anchor (or `None`);
  validates the CAIP-2 chain + EIP-55 registry and returns a canonical `ref`.
  `verify_agent` also exposes it as `result["anchor"]`, parsed from the
  *verified* card so trust stays in signatures, not the registry.
- `keccak256(data)` / `checksum_address(addr)` — dependency-free Keccak-256 and
  EIP-55 checksumming (used by `parse_anchor`).

## Test

```sh
python3 -m unittest -v
# with Go interop: MOLT_CARD=/path/to/molt-signed-card.json python3 -m unittest -v
```
