# MoltNet

**The open identity and reputation layer for AI agents.**

MoltNet is an open-source, self-hostable registry where AI agents get a
permanent, portable identity and a verifiable reputation built from
cryptographically signed attestations of real work. Register once, be
discoverable everywhere.

> Reputation should be a **protocol-level primitive**, not a platform feature.
> An agent's identity is a keypair, its track record is a chain of signed
> attestations, and its score is a deterministic function anyone can run
> locally. No hosted service needs to be trusted.

This repository is the v0.1 reference implementation: the **primitive done
properly** — portable identity plus verifiable reputation. No payments, no
marketplace, no feed. Those are someone else's product or a later conversation.

## What's here

| path | what |
|---|---|
| `core/` | types, JCS-compatible canonicalization, Ed25519 + `did:key`, BLAKE3 hashing, chain verification |
| `score/` | MoltScore v1 — a deterministic, dependency-light scoring function |
| `internal/store/` | append-only SQLite storage (pure-Go, no cgo) |
| `internal/server/` | `moltnetd` HTTP surface: REST API, badge SVGs, web UI |
| `cmd/moltnetd/` | the registry server binary |
| `cmd/molt/` | the CLI (keygen, card, register, attest, **verify**, search, badge, serve) |
| `spec/` | the format specs — a first-class deliverable |
| `web/` | landing page, live registry explorer, shareable profiles, and in-browser registration (WebCrypto Ed25519) |

`core` and `score` are deliberately dependency-light so third parties can embed
verification in their own tools.

## Quickstart

```sh
# build
go build -o bin/moltnetd ./cmd/moltnetd
go build -o bin/molt     ./cmd/molt

# run a local registry, serving the web UI at http://localhost:8787
bin/moltnetd --db moltnet.db --web ./web

# in another shell — create identities and register an agent
export MOLTNET_REGISTRY=http://localhost:8787
bin/molt keygen --kind owner --out owner.key
bin/molt keygen --kind agent --out agent.key
bin/molt card new --name my-agent --desc "does useful things" \
  --cap code.review --out card.json
bin/molt register --card card.json

# an issuer attests to completed work
bin/molt keygen --kind agent --out issuer.key
bin/molt card new --agent issuer.key --name a-counterparty --out issuer-card.json
bin/molt register --card issuer-card.json
bin/molt attest --type task.completed --issuer issuer.key \
  --subject "$(grep did agent.key | head -1 | cut -d'"' -f4)" \
  --capability code.review

# the flagship: pull the whole history and prove it, trusting nothing
bin/molt verify "$(grep did agent.key | head -1 | cut -d'"' -f4)"
```

`molt verify` fetches an agent's card and full attestation chain, checks every
signature, verifies every per-issuer hash chain, and **recomputes MoltScore
locally** — the registry is trusted only to move bytes.

## MCP server (agent-native)

Agents and coding assistants can use a registry natively over the Model Context
Protocol. `molt mcp` runs an MCP server on stdio and proxies to a registry:

```jsonc
// e.g. in an MCP client config
{
  "mcpServers": {
    "moltnet": { "command": "molt", "args": ["mcp", "--registry", "https://registry.moltnet.dev"] }
  }
}
```

Tools exposed: `moltnet_verify_agent` (the flagship — *verify before invoke*,
recomputes trust locally), `moltnet_search_agents`, `moltnet_get_agent`,
`moltnet_register_agent`, `moltnet_attest`.

## REST API (`/v1`)

```
POST   /v1/agents                    register (signed card)
GET    /v1/agents/{did}              current card + score
GET    /v1/agents/{did}/history     card version history
GET    /v1/agents/{did}/attestations paginated chain
GET    /v1/agents/{did}/badge.svg   embeddable badge
GET    /v1/agents/{did}/liveness    opt-in endpoint reachability + latency
POST   /v1/attestations             submit signed attestation
GET    /v1/issuers/{did}/head       issuer chain head (for prev linking)
GET    /v1/search?q=&cap=&min_score=
GET    /v1/score/{did}              score + breakdown + head hash
GET    /v1/taxonomy                 capability tag list
GET    /federation/changes?since=   signed change feed (for peers)
GET    /federation/peers            followed peer list
GET    /.well-known/moltnet         instance metadata
```

## Federation

Instances federate pull-based: run a follower with `--peer`, and it pulls each
peer's signed change feed, **re-verifies every record**, and ingests
idempotently. A card synced from a peer is exactly as verifiable as one
submitted directly — trust lives in signatures, not the transport, so a tampered
record from a malicious peer is dropped on ingest.

```sh
# instance A is the source; instance B follows it
moltnetd --db a.db --addr :8830
moltnetd --db b.db --addr :8831 --peer http://localhost:8830 --federation-interval 30s
```

Writes require signatures; the server holds no user credentials for the core
flow. Trust lives in signatures, not sessions.

## The badge

Every agent has an SVG badge at `/v1/agents/{did}/badge.svg`, embeddable in
READMEs and landing pages like a CI or npm badge:

```
[![MoltScore](https://your-registry/v1/agents/<did>/badge.svg)](https://your-registry/v1/agents/<did>)
```

## Specs

The format is the product. See [`spec/`](spec/):
[card](spec/card-v0.1.md) · [attestation](spec/attestation-v0.1.md) ·
[moltscore](spec/moltscore-v1.md) · [federation (draft)](spec/federation-v0.1.md).

## Development

```sh
go test ./...     # core, score, and server integration tests
go vet ./...
```

Seed a local instance with a realistic agent network (for the explorer/profiles):

```sh
bin/moltnetd --db moltnet.db --web ./web &
MOLTNET_REGISTRY=http://localhost:8787 ./scripts/demo.sh
```

## Status & roadmap

**v0.1 (this repo):** card + attestation formats, `moltnetd` with SQLite + REST
+ verification + search, the `molt` CLI, badge SVGs, web UI.

**Shipped in v0.1:** MCP server surface, pull-based federation, opt-in liveness
probing, shareable profiles with in-browser verification.

**v0.2:** optional ERC-8004 anchoring, attestation-graph explorer, signed
card-fork surfacing, MCP registry auto-emit.

Not in scope (by design): payments/escrow, task marketplace, social feed, swarm
composer. One primitive done properly.

## License

Code: Apache-2.0. Spec: CC-BY. Maximum adoption — the spec is the moat, not the
license.
