#!/usr/bin/env bash
# eval-01 (loop): install (on top of issues) lands the playbook + scripts and
# the doctor passes; tier S correctly installs nothing.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"
"$PLUGIN_ROOT/scripts/install-recipe.sh" --recipe "$PLUGIN_ROOT/recipes/issues" --target "$T" --tier M >/dev/null
"$PLUGIN_ROOT/scripts/install-recipe.sh" --recipe "$PLUGIN_ROOT/recipes/loop" --target "$T" --tier M \
  --answer project_name=acme >/dev/null

for f in docs/DEV_LOOP.md scripts/start-fresh.sh scripts/next-issue.sh; do
  [ -e "$T/$f" ] || fail "expected $f after tier-M install"
done
[ -x "$T/scripts/start-fresh.sh" ] || fail "start-fresh.sh must be executable"
grep -q '{{' "$T/docs/DEV_LOOP.md" && fail "unsubstituted placeholder in DEV_LOOP.md" || true

"$PLUGIN_ROOT/recipes/loop/scripts/doctor.sh" "$T" --tier M >/dev/null \
  || fail "loop doctor should pass on a fresh install"

# Tier S installs no loop files.
S="$(mktemp -d)"; trap 'rm -rf "$T" "$S"' EXIT
git init -q -b main "$S"
"$PLUGIN_ROOT/scripts/install-recipe.sh" --recipe "$PLUGIN_ROOT/recipes/loop" --target "$S" --tier S >/dev/null
[ ! -e "$S/docs/DEV_LOOP.md" ] || fail "tier S must not install the loop playbook"
"$PLUGIN_ROOT/recipes/loop/scripts/doctor.sh" "$S" --tier S >/dev/null || fail "tier S doctor should be green"

echo "PASS: loop eval-01 (install + doctor, tier S lean)"
