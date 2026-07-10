# Conformance vectors

Shared, frozen test vectors that every MoltNet implementation must reproduce.
They pin the two things that MUST agree byte-for-byte / number-for-number across
languages, or the "verify anywhere" promise breaks:

- **`canonical_vectors.json`** — JCS-compatible canonical JSON. `{input, expected}`.
- **`score_vectors.json`** — MoltScore v1. `{now, attestations, expected:{score, inputs}}`.
  The `now` clock is fixed so recency decay is deterministic.

The Go, TypeScript and Python clients each run these as tests:

| impl | test |
|---|---|
| Go | `go test ./core/ ./score/ -run Conformance` |
| TypeScript | `clients/ts` → `npm test` (`conformance.test.mjs`) |
| Python | `clients/python` → `python3 -m unittest test_conformance` |

Regenerate from the Go reference (the source of truth) after an intentional
change:

```sh
go run ./tools/genconformance
```

Then re-run all three suites; any implementation that disagrees fails.
