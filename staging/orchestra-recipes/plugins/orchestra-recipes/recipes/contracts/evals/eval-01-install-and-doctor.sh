#!/usr/bin/env bash
# eval-01 (contracts): default (non-fleet) install lands only CONTRACTS.md and
# the doctor passes; fleet files appear only with fleet_member=true.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"
"$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/contracts" --target "$T" --tier S \
  --answer project_name=acme-api >/dev/null

[ -f "$T/docs/CONTRACTS.md" ] || fail "CONTRACTS.md not installed"
grep -q '^## Provides' "$T/docs/CONTRACTS.md" || fail "Provides section missing"
grep -q '^## Consumes' "$T/docs/CONTRACTS.md" || fail "Consumes section missing"
[ ! -e "$T/constellation.yaml" ] || fail "constellation.yaml must not install when fleet_member=false"
[ ! -e "$T/scripts/fleet-doctor.sh" ] || fail "fleet-doctor.sh must not install when fleet_member=false"

"$PLUGIN_ROOT/recipes/contracts/scripts/doctor.sh" "$T" --tier S >/dev/null \
  || fail "contracts doctor should pass on a non-fleet install"

# Fleet variant.
F="$(mktemp -d)"; trap 'rm -rf "$T" "$F"' EXIT
git init -q -b main "$F"
"$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/contracts" --target "$F" --tier S \
  --answer project_name=acme-api --answer fleet_member=true --answer fleet_name=acme >/dev/null
[ -f "$F/constellation.yaml" ] || fail "constellation.yaml missing on fleet install"
[ -x "$F/scripts/fleet-doctor.sh" ] || fail "fleet-doctor.sh missing/not executable on fleet install"
grep -q 'fleet: acme' "$F/constellation.yaml" || fail "fleet_name not rendered"
"$PLUGIN_ROOT/recipes/contracts/scripts/doctor.sh" "$F" --tier S >/dev/null \
  || fail "contracts doctor should pass on a fresh fleet install (self-only constellation)"

echo "PASS: contracts eval-01 (install variants + doctor green)"
