#!/usr/bin/env bash
# eval-02 (quality): build_system=npm selects the npm CI flavor (the `when:`
# condition routes between the two ci templates).
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"
echo '{"name":"acme-web","private":true}' > "$T/package.json"

"$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/quality" --target "$T" --tier S \
  --answer project_name=acme-web \
  --answer build_system=npm \
  --answer "lint_command=npm run lint" \
  --answer "test_command=npm test" \
  --answer node_version=22 >/dev/null

grep -q 'setup-node' "$T/.github/workflows/ci.yml" || fail "npm flavor should use setup-node"
grep -q 'node-version: "22"' "$T/.github/workflows/ci.yml" || fail "node_version answer not rendered"
grep -q 'run: npm test' "$T/.github/workflows/ci.yml" || fail "test_command answer not rendered"
[ ! -f "$T/docs/REGRESSIONS.md" ] || fail "REGRESSIONS.md must not be installed at tier S"

# Tier S doctor: guard + CI present is enough.
"$PLUGIN_ROOT/recipes/quality/scripts/doctor.sh" "$T" --tier S >/dev/null \
  || fail "quality doctor should pass at tier S npm flavor"

echo "PASS: quality eval-02 (npm flavor via when-condition)"
