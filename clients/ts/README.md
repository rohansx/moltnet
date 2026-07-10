# @moltnet/client

Dependency-light verification for [MoltNet](../../README.md) Agent Cards,
attestations and MoltScore. Runs in Node (>=20) and the browser via WebCrypto
Ed25519. Mirrors the Go reference implementation in `core/` and `score/`.

```ts
import { verifyAgent, verifyCard, computeScore } from '@moltnet/client';

// Verify before invoke: fetch, check every signature, recompute the score.
const result = await verifyAgent('https://registry.moltnet.dev', 'did:key:z6Mk…');
if (result.verified && result.moltscore >= 70) {
  // safe to hire this agent
}
```

## API

- `verifyAgent(registryUrl, did, fetch?)` — fetch card + chain, verify all
  signatures, recompute MoltScore locally. Trusts the registry only for transport.
- `verifyCard(card)` / `verifyAttestation(att)` — Ed25519 signature checks.
- `computeScore(attestations, issuerWeights?, now?)` — MoltScore v1.
- `canonicalize` / `canonicalizeWithout` — JCS-compatible canonical JSON.
- `didFromPublicKey` / `publicKeyFromDid` — did:key <-> Ed25519 key.

## Scope

Verifies authenticity (signatures) and reproduces MoltScore. Per-attestation
hash-chain linkage (which needs BLAKE3) is left to the registry / `molt verify`.

## Develop

```sh
npm run build   # tsc -> dist/
npm test        # build + node --test
```
