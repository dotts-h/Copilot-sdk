#!/usr/bin/env bash
# eval-03 (core): the doctor exits 1 with readable GAP lines on a non-conformant
# repo (missing constitution, fat CLAUDE.md), and 0 once gaps are fixed.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"
# A fat CLAUDE.md (not a thin pointer) and no docs/ at all.
for i in $(seq 1 80); do echo "rule $i: remember this" >> "$T/CLAUDE.md"; done

set +e
out="$("$PLUGIN_ROOT/recipes/core/scripts/doctor.sh" "$T" --tier M 2>&1)"
rc=$?
set -e
[ "$rc" -eq 1 ] || fail "doctor should exit 1 on a gappy repo (got $rc)"
echo "$out" | grep -q "GAP: CLAUDE.md has 80 lines" || fail "fat CLAUDE.md not flagged"
echo "$out" | grep -q "GAP: docs/CONVENTIONS.md missing" || fail "missing constitution not flagged"
echo "$out" | grep -q "GAP: AGENTS.md missing" || fail "missing AGENTS.md not flagged"

# Fix by installing (force only replaces the fat pointer; everything else is fresh).
"$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/core" --target "$T" --tier M --force >/dev/null
"$PLUGIN_ROOT/recipes/core/scripts/doctor.sh" "$T" >/dev/null \
  || fail "doctor should pass after install repaired the gaps"

echo "PASS: core eval-03 (doctor gap report + exit codes)"
