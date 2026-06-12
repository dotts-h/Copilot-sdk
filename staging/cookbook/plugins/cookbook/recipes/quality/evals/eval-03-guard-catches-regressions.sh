#!/usr/bin/env bash
# eval-03 (quality): the generalized workflow guard catches BOTH learned
# regressions — a feature-branch push trigger (CI double-run) and the
# ${GITHUB_REF_NAME:-...} release version bug — and the doctor surfaces them.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"
"$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/quality" --target "$T" --tier S >/dev/null

# Introduce regression 1: a feature-branch push trigger.
mkdir -p "$T/.github/workflows"
cat > "$T/.github/workflows/bad-ci.yml" <<'EOF'
name: BadCI
on:
  push:
    branches: [main, "feature/**"]
  pull_request:
    branches: [main]
jobs:
  noop:
    runs-on: ubuntu-latest
    steps:
      - run: "true"
EOF

# Introduce regression 2: the release version-resolution bug.
cat > "$T/.github/workflows/release.yml" <<'EOF'
name: Release
on:
  push:
    tags: ["v*"]
  workflow_dispatch:
    inputs:
      tag:
        required: true
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo "version=${GITHUB_REF_NAME:-${{ github.event.inputs.tag }}}" >> "$GITHUB_OUTPUT"
EOF

set +e
out="$(cd "$T" && bash scripts/check-workflows.sh 2>&1)"
rc=$?
set -e
[ "$rc" -eq 1 ] || fail "guard should exit 1 on both regressions (got $rc)"
echo "$out" | grep -q "bad-ci.yml" || fail "feature-branch push trigger not flagged"
echo "$out" | grep -q "feature/\*\*" || fail "offending branch glob not named"
echo "$out" | grep -q "release.yml" || fail "release version-resolution bug not flagged"

set +e
"$PLUGIN_ROOT/recipes/quality/scripts/doctor.sh" "$T" --tier S >/dev/null 2>&1
drc=$?
set -e
[ "$drc" -eq 1 ] || fail "quality doctor should report the guard failure as a gap (got $drc)"

# Fix both; guard and doctor go green again.
rm "$T/.github/workflows/bad-ci.yml" "$T/.github/workflows/release.yml"
(cd "$T" && bash scripts/check-workflows.sh >/dev/null) || fail "guard should pass after the fix"

echo "PASS: quality eval-03 (guard catches double-run + release-version regressions)"
