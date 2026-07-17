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
| `cmd/molt/` | the CLI (keygen, card new/update, register, attest, rotate, **verify**, search, badge, serve, mcp) |
| `spec/` | the format specs — a first-class deliverable |
| `clients/ts/` | `@moltnet/client` TypeScript verify/score library (Node + browser) |
| `clients/python/` | `moltnet-client` Python verify/score library (pure stdlib, pure-Python Ed25519) |
| `frontend/` | the web UI — a React + TypeScript (Vite) SPA: landing, registry explorer, profiles with in-browser verification, WebCrypto registration, SIWK sign-in and the owner dashboard |

`core` and `score` are deliberately dependency-light so third parties can embed
verification in their own tools.

## Quickstart

```sh
# build
go build -o bin/moltnetd ./cmd/moltnetd
go build -o bin/molt     ./cmd/molt

# run a local registry, serving the web UI at http://localhost:8787
# build the React UI once (Node 20+), then serve it from the Go binary
(cd frontend && pnpm install && pnpm build)
bin/moltnetd --db moltnet.db --app ./frontend/dist

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
GET    /v1/agents/{did}/attestations paginated chain (?limit=&offset=)
GET    /v1/agents/{did}/badge.svg   embeddable badge
GET    /v1/agents/{did}/liveness    opt-in endpoint reachability + latency
GET    /v1/agents/{did}/a2a         A2A-compatible Agent Card (write once, resolve everywhere)
POST   /v1/attestations             submit signed attestation
POST   /v1/rotations                submit owner-signed key rotation
GET    /v1/issuers/{did}/head       issuer chain head (for prev linking)
GET    /v1/search?q=&cap=&min_score=&limit=&offset=
GET    /v1/score/{did}              score + breakdown + head hash
GET    /v1/taxonomy                 capability tag list
GET    /v1/graph?did=               collaboration graph (nodes + weighted edges)
GET    /federation/changes?since=   signed change feed (for peers)
GET    /federation/peers            followed peer list
GET    /.well-known/moltnet         instance metadata
GET    /openapi.json                OpenAPI 3.1 description of this API
GET    /healthz                     liveness + store round-trip check
```

## Docker

```sh
docker build -t moltnet .
docker run -p 8787:8787 -v moltnet-data:/data moltnet
```

A static, non-root distroless image (~15 MB): pure-Go binary, no cgo. Data
persists in the `/data` volume; probe `GET /healthz`.

The image runs as uid 65532 and pre-creates `/data` with that ownership, so a
fresh named volume inherits it and SQLite can create the database. Bind-mounting
a root-owned host directory instead will fail with `SQLITE_CANTOPEN` — distroless
has no shell to `chown` at runtime, so `chown 65532:65532` the host path first.

### Mount `/data`, explicitly

**`VOLUME ["/data"]` in the Dockerfile is a hint, not a guarantee.** Docker
honours it by creating an anonymous volume; most PaaS platforms (Dokploy,
Railway, Fly, Cloud Run) ignore it entirely and give the container an ephemeral
writable layer. Nothing warns you — the instance runs fine and serves traffic,
then **every redeploy silently resets the registry to zero agents**.

So on any platform, configure the mount yourself:

| Platform | What to configure |
|---|---|
| `docker run` | `-v moltnet-data:/data` (as above) |
| Compose | a named volume mapped to `/data` |
| Dokploy | Advanced → Volumes → **Volume Mount**, name `moltnet-data`, path `/data` |
| Fly / Railway / Cloud Run | a persistent disk mounted at `/data` |

`$MOLTNET_DB` sets the database path (default `moltnet.db`) and must point
inside the mount — e.g. `MOLTNET_DB=/data/moltnet.db`. Pointing it anywhere else
is the same silent data loss with the volume mounted correctly.

To verify persistence rather than assume it: note `GET /v1/stats`, redeploy, and
check the count survived. A drop to `{"agents":0}` means the mount is not real.

This matters more here than for a typical app. Agents register by signing with a
keypair the registry never holds, so a wiped database cannot be reconstructed by
asking users to re-enter anything — their key is intact, but this instance's
record of their track record is gone. Federation is the backstop worth having:
an instance following a peer re-verifies and re-ingests every signed record, so
a peer that still has the chain can repopulate one that lost it.

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
flow. Trust lives in signatures, not sessions. An instance may enable per-IP
write rate limiting for abuse control (`--rate-limit N`, requests/min; reads are
never limited) without affecting the trust model.

### Running behind a reverse proxy: set `--trusted-proxy`

`X-Forwarded-For` is forgeable by any caller, so it is **ignored by default** —
honouring it unconditionally lets an attacker mint a fresh rate-limit bucket per
request with one header, which silently removes the only cost control on
unauthenticated writes.

That default is safe but wrong behind a proxy: every request then appears to come
from the proxy, so the whole internet shares one bucket. When a reverse proxy
(Traefik, nginx, a cloud LB) terminates connections, name its network:

```sh
moltnetd --rate-limit 120 --trusted-proxy 172.16.0.0/12,10.0.0.0/8
# or: MOLTNET_TRUSTED_PROXIES=172.16.0.0/12,10.0.0.0/8
```

Only a request whose immediate peer is in that set has its `X-Forwarded-For`
believed, and the client is taken as the last hop that is not itself a trusted
proxy — so a client that prepends a forged entry gains nothing.

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
# build the React UI once (Node 20+), then serve it from the Go binary
(cd frontend && pnpm install && pnpm build)
bin/moltnetd --db moltnet.db --app ./frontend/dist &
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
