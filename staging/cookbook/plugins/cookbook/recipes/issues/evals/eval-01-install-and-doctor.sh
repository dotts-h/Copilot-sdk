#!/usr/bin/env bash
# eval-01 (issues): tier-M install lands the hybrid store skeleton and the
# doctor passes on an empty (but structured) store.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"
"$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/issues" --target "$T" --tier M \
  --answer project_name=acme >/dev/null

for f in docs/issues/INDEX.md docs/issues/TEMPLATE.md scripts/new-issue.sh scripts/sync-github.sh; do
  [ -e "$T/$f" ] || fail "expected $f after install"
done
[ -x "$T/scripts/new-issue.sh" ] || fail "new-issue.sh must be executable"
grep -q "acme" "$T/docs/issues/INDEX.md" || fail "project_name not rendered into INDEX"

"$PLUGIN_ROOT/recipes/issues/scripts/doctor.sh" "$T" --tier M >/dev/null \
  || fail "issues doctor should pass on a fresh install"

echo "PASS: issues eval-01 (install + doctor green)"
