#!/usr/bin/env bash
# doctor.sh (core) — conformance check for the `core` recipe against a target repo.
#
#   doctor.sh <target-repo> [--tier S|M|L]
#
# Exit codes (the doctor contract, docs/SPEC.md):
#   0 = conformant   1 = gaps found (one readable "GAP:" line each)   2 = usage error
#
# Divergence from the templates is NOT a gap — the repo's copy is canonical for
# that repo. The doctor checks *presence and shape*, not byte-equality.
set -uo pipefail

target="${1:-}"; shift || true
tier=""
while [ $# -gt 0 ]; do case "$1" in
  --tier) tier="$2"; shift 2;;
  *) echo "unknown arg: $1" >&2; exit 2;;
esac; done
[ -n "$target" ] && [ -d "$target" ] || { echo "usage: doctor.sh <target-repo> [--tier S|M|L]" >&2; exit 2; }

# Tier: flag > lock > default M.
if [ -z "$tier" ] && [ -f "$target/.recipes/lock.json" ]; then
  tier=$(TARGET="$target" python3 - <<'PY'
import json, os
try:
    lock = json.load(open(os.path.join(os.environ["TARGET"], ".recipes", "lock.json")))
    print(next((r.get("tier","") for r in lock.get("recipes", []) if r.get("name")=="core"), ""))
except Exception:
    print("")
PY
)
fi
tier="${tier:-M}"

gaps=0
gap() { echo "GAP: $*"; gaps=$((gaps+1)); }
ok()  { echo "  ok: $*"; }

echo "== core doctor (tier $tier) on $target"

# Thin pointers — both hosts.
for f in CLAUDE.md AGENTS.md; do
  if [ -f "$target/$f" ]; then
    lines=$(wc -l <"$target/$f" | tr -d ' ')
    if [ "$lines" -gt 60 ]; then
      gap "$f has $lines lines — a thin pointer should stay under ~60; move rules into docs/ and point"
    else
      ok "$f present and thin ($lines lines)"
    fi
  else
    gap "$f missing — both CLAUDE.md and AGENTS.md thin pointers are required (host-agnostic)"
  fi
done

# Constitution.
if [ -f "$target/docs/CONVENTIONS.md" ]; then
  ok "docs/CONVENTIONS.md present"
  grep -qi 'one fact' "$target/docs/CONVENTIONS.md" \
    && ok "doctrine anchor ('one fact, one home') found" \
    || gap "docs/CONVENTIONS.md lacks the doctrine ('one fact, one home' anchor not found)"
else
  gap "docs/CONVENTIONS.md missing — the constitution is the core artifact"
fi

# Lock.
if [ -f "$target/.recipes/lock.json" ]; then
  python3 -c "import json,sys; json.load(open(sys.argv[1]))" "$target/.recipes/lock.json" 2>/dev/null \
    && ok ".recipes/lock.json present and valid JSON" \
    || gap ".recipes/lock.json is not valid JSON"
else
  gap ".recipes/lock.json missing — install via the recipe installer so updates stay possible"
fi

# Tier M and up.
if [ "$tier" != "S" ]; then
  [ -f "$target/docs/CONTEXT.md" ] \
    && ok "docs/CONTEXT.md present" \
    || gap "docs/CONTEXT.md (glossary) missing — required at tier $tier"
  [ -f "$target/docs/adr/README.md" ] \
    && ok "docs/adr/README.md present" \
    || gap "docs/adr/README.md (ADR index) missing — required at tier $tier"
else
  # Tier S must stay lean: an ADR log at tier S means the tier is wrong, not the repo.
  if [ -d "$target/docs/adr" ]; then
    gap "tier S but docs/adr/ exists — either graduate the lock to tier M or drop the ADR log"
  fi
fi

# Tier L.
if [ "$tier" = "L" ]; then
  [ -f "$target/docs/TECH_DEBT.md" ] \
    && ok "docs/TECH_DEBT.md present" || gap "docs/TECH_DEBT.md missing — required at tier L"
  [ -f "$target/docs/RETROS/README.md" ] \
    && ok "docs/RETROS/ present" || gap "docs/RETROS/README.md missing — required at tier L"
  [ -f "$target/scripts/codemap.sh" ] \
    && ok "scripts/codemap.sh present" || gap "scripts/codemap.sh missing — CODEMAP must be generatable at tier L"
fi

echo "== core doctor: $gaps gap(s)"
[ "$gaps" -eq 0 ] || exit 1
