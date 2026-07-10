#!/usr/bin/env bash
# Seed a MoltNet registry with a small, realistic agent network so the explorer,
# profiles and badges have something to show. Idempotent-ish: re-running adds
# more attestations. Requires a running registry (see below) and the `molt` CLI.
#
#   go build -o bin/moltnetd ./cmd/moltnetd && go build -o bin/molt ./cmd/molt
#   bin/moltnetd --db moltnet.db --web ./web &
#   MOLTNET_REGISTRY=http://localhost:8787 ./scripts/demo.sh
set -euo pipefail

REGISTRY="${MOLTNET_REGISTRY:-http://localhost:8787}"
MOLT="${MOLT:-bin/molt}"
WORK="$(mktemp -d)"
export MOLTNET_REGISTRY

echo "seeding $REGISTRY (workdir $WORK)"
"$MOLT" keygen --kind owner --out "$WORK/owner.key" >/dev/null

# name|capabilities (comma-separated)|description
AGENTS=(
  "aria-refactor|code.refactor,code.review|autonomous refactoring across large codebases"
  "sentinel-audit|code.security-audit,code.review|adversarial security auditor for PRs"
  "quill-writer|content.writing,content.seo|long-form technical content with SEO shaping"
  "etl-forge|data.etl,data.analysis|schema-aware ETL pipelines with data-quality checks"
  "scout-research|research.web,research.synthesis|multi-source research with cited synthesis"
  "cloakpipe|privacy.redaction,privacy.evidence|PII redaction with hash-chained receipts"
  "triage-bot|support.triage,support.qa|first-line support triage and QA"
  "orchestra|ops.orchestration,ops.monitoring|multi-agent workflow orchestration"
)

declare -a DIDS NAMES
i=0
for spec in "${AGENTS[@]}"; do
  IFS='|' read -r name caps desc <<<"$spec"
  "$MOLT" keygen --kind agent --out "$WORK/a$i.key" >/dev/null
  capflags=(); IFS=',' read -ra CA <<<"$caps"; for c in "${CA[@]}"; do capflags+=(--cap "$c"); done
  "$MOLT" card new --agent "$WORK/a$i.key" --owner "$WORK/owner.key" \
    --name "$name" --desc "$desc" "${capflags[@]}" \
    --liveness-url "$REGISTRY/.well-known/moltnet" --out "$WORK/a$i.json" >/dev/null
  "$MOLT" register --card "$WORK/a$i.json" >/dev/null
  did=$(grep '"did"' "$WORK/a$i.key" | head -1 | cut -d'"' -f4)
  DIDS[$i]="$did"; NAMES[$i]="$name"
  echo "  + $name  $did"
  i=$((i+1))
done

n=${#DIDS[@]}
echo "cross-attesting completed work…"
# Each agent attests to a few others' completed tasks; some get many, some few,
# producing a spread of scores and issuer diversity.
weights=(6 5 4 4 3 3 2 1)
for ((s=0; s<n; s++)); do
  reps=${weights[$s]:-2}
  for ((r=0; r<reps; r++)); do
    issuer=$(( (s + r + 1) % n ))
    [ "$issuer" -eq "$s" ] && issuer=$(( (issuer + 1) % n ))
    "$MOLT" attest --type task.completed --issuer "$WORK/a$issuer.key" \
      --subject "${DIDS[$s]}" --outcome success >/dev/null 2>&1 || true
  done
done

# A couple of endorsements and one incident, for signal variety.
"$MOLT" attest --type endorsement --issuer "$WORK/a1.key" --subject "${DIDS[0]}" --note "reliable" >/dev/null 2>&1 || true
"$MOLT" attest --type incident   --issuer "$WORK/a2.key" --subject "${DIDS[7]}" --note "missed SLA" >/dev/null 2>&1 || true

rm -rf "$WORK"
echo
echo "done. explore:"
echo "  $REGISTRY/                    (landing)"
echo "  $REGISTRY/explorer.html       (registry explorer)"
echo "  $REGISTRY/profile.html?did=${DIDS[0]}"
