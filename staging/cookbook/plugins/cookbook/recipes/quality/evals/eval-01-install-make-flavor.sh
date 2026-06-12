#!/usr/bin/env bash
# eval-01 (quality): make-flavor install lands single-run CI + the guard, and the
# doctor passes on the result.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"
printf 'lint:\n\ttrue\ntest:\n\ttrue\n' > "$T/Makefile"

"$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/quality" --target "$T" --tier M \
  --answer project_name=acme >/dev/null

[ -f "$T/.github/workflows/ci.yml" ] || fail "ci.yml not installed"
[ -x "$T/scripts/check-workflows.sh" ] || fail "check-workflows.sh not installed/executable"
[ -f "$T/docs/REGRESSIONS.md" ] || fail "REGRESSIONS.md missing at tier M"
grep -q 'branches: \[main\]' "$T/.github/workflows/ci.yml" || fail "single-run trigger rule not rendered"
grep -q 'setup-node' "$T/.github/workflows/ci.yml" && fail "make flavor must not contain the npm CI" || true
grep -q 'COVERAGE_FLOOR: "65"' "$T/.github/workflows/ci.yml" || fail "coverage floor default not rendered"

"$PLUGIN_ROOT/recipes/quality/scripts/doctor.sh" "$T" >/dev/null \
  || fail "quality doctor should pass on a fresh make-flavor install"

echo "PASS: quality eval-01 (make flavor install + doctor green)"
