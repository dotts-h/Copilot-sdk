#!/usr/bin/env bash
# eval-01 (release): install renders the workflow + playbook with answers and
# the doctor passes (correct version resolution, dispatch path, checksums).
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"
"$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/release" --target "$T" --tier M \
  --answer project_name=acme --answer artifact_name=acme-cli \
  --answer "release_build_command=printf 'bin' > dist/acme-cli-\$VERSION-linux-amd64" >/dev/null

[ -f "$T/.github/workflows/release.yml" ] || fail "release.yml not installed"
[ -f "$T/docs/RELEASING.md" ] || fail "RELEASING.md missing at tier M"
grep -q 'github.event.inputs.tag || github.ref_name' "$T/.github/workflows/release.yml" \
  || fail "correct version resolution not rendered"
grep -q 'acme-cli' "$T/.github/workflows/release.yml" || fail "artifact_name not rendered"
grep -q 'acme-cli-<tag>' "$T/docs/RELEASING.md" || fail "artifact_name not rendered in playbook"

"$PLUGIN_ROOT/recipes/release/scripts/doctor.sh" "$T" >/dev/null \
  || fail "release doctor should pass on a fresh install"

echo "PASS: release eval-01 (install + doctor green)"
