#!/usr/bin/env bash
# eval-03 (release): the doctor requires a tag-driven workflow — a repo whose
# only "release" runs on push to main (or has no release workflow) gets gaps.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"

# No workflows at all -> gap.
set +e
out="$("$PLUGIN_ROOT/recipes/release/scripts/doctor.sh" "$T" --tier S 2>&1)"; rc=$?
set -e
[ "$rc" -eq 1 ] || fail "doctor should exit 1 with no release workflow (got $rc)"
echo "$out" | grep -q "no workflow with a push.tags trigger" || fail "missing tag trigger not reported"

# A branch-triggered "release" workflow still isn't tag-driven -> gap persists.
mkdir -p "$T/.github/workflows"
cat > "$T/.github/workflows/release.yml" <<'EOF'
name: Release
on:
  push:
    branches: [main]
jobs:
  ship:
    runs-on: ubuntu-latest
    steps:
      - run: echo ship
EOF
set +e
"$PLUGIN_ROOT/recipes/release/scripts/doctor.sh" "$T" --tier S >/dev/null 2>&1; rc=$?
set -e
[ "$rc" -eq 1 ] || fail "a branch-triggered release must not satisfy the tag-driven check (got $rc)"

# Installing the recipe's workflow fixes it (force to replace the bad one).
"$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/release" --target "$T" --tier S --force >/dev/null
"$PLUGIN_ROOT/recipes/release/scripts/doctor.sh" "$T" --tier S >/dev/null \
  || fail "doctor should pass after the recipe workflow is installed"

echo "PASS: release eval-03 (tag-driven requirement enforced)"
