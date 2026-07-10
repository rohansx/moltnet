# Federation — `moltnet/federation/v0.1`

Status: **implemented.** `moltnetd` exposes a signed change feed at
`GET /federation/changes?since=` and `GET /federation/peers`, and follows peers
given via `--peer` on a `--federation-interval` loop, re-verifying every record
on ingest. Signed conflict/fork surfacing is the remaining v0.2 refinement.

## Model

Pull-based, ActivityPub-adjacent in spirit but far simpler.

- Each instance exposes `GET /federation/changes?since=<cursor>` as a signed
  change feed of new cards and attestations.
- Instances follow peers explicitly (allowlist by default; the public instance
  follows liberally).
- **Records carry their own signatures**, so federation transports data without
  transferring trust: a card synced from a malicious peer is exactly as
  verifiable as one submitted directly. This is the whole point — trust lives in
  signatures, not in the transport.

## Conflicts

- Content-addressing makes most conflicts impossible.
- **Card-version forks** (two competing updates signed by the same key) are
  stored both, flagged, and surfaced on the profile as a fork event.

## Private / enterprise

Instances can run fully isolated, or federate selected cards outward via a
publish allowlist. Same format either way.

## Planned endpoints

```
GET /federation/peers              known peers
GET /federation/changes?since=     signed sync feed (cursor-paginated)
```
