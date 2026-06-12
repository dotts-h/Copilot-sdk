#!/usr/bin/env bash
# eval-02 (core): tier S stays genuinely lean (pointers + CONVENTIONS-lite only),
# and the installer never clobbers an existing file (brownfield safety).
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"
echo "PRE-EXISTING USER CONTENT" > "$T/CLAUDE.md"   # brownfield file

"$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/core" --target "$T" --tier S \
  --answer project_name=tiny-fn >/dev/null

# Lean: exactly the S set — no glossary, no ADR log, no tier-L extras.
for f in AGENTS.md docs/CONVENTIONS.md; do
  [ -f "$T/$f" ] || fail "expected $f at tier S"
done
for f in docs/CONTEXT.md docs/adr/README.md docs/TECH_DEBT.md docs/RETROS/README.md scripts/codemap.sh; do
  [ ! -e "$T/$f" ] || fail "$f must NOT be installed at tier S (lean)"
done
grep -q '## Decisions' "$T/docs/CONVENTIONS.md" \
  || fail "CONVENTIONS-lite must carry the '## Decisions' graduation section"

# Never clobber: the pre-existing CLAUDE.md is untouched and reported as a skip.
[ "$(cat "$T/CLAUDE.md")" = "PRE-EXISTING USER CONTENT" ] \
  || fail "installer overwrote an existing CLAUDE.md (must never clobber)"

out="$("$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/core" --target "$T" --tier S \
  --answer project_name=tiny-fn)"
echo "$out" | grep -q "SKIP  CLAUDE.md" || fail "re-install did not report SKIP for existing CLAUDE.md"

"$PLUGIN_ROOT/recipes/core/scripts/doctor.sh" "$T" --tier S >/dev/null \
  || fail "core doctor should pass at tier S"

echo "PASS: core eval-02 (tier-S lean set + no-clobber)"
