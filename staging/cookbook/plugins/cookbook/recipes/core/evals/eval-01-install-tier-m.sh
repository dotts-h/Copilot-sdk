#!/usr/bin/env bash
# eval-01 (core): tier-M install renders templates with answers, writes the lock,
# and the doctor passes on the result.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"

"$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/core" --target "$T" --tier M \
  --answer project_name=acme-api \
  --answer "project_description=an example service" >/dev/null

for f in CLAUDE.md AGENTS.md docs/CONVENTIONS.md docs/CONTEXT.md docs/adr/README.md; do
  [ -f "$T/$f" ] || fail "expected $f after tier-M install"
done
[ ! -f "$T/docs/TECH_DEBT.md" ] || fail "TECH_DEBT.md is tier L only, but appeared at tier M"

grep -q "acme-api" "$T/AGENTS.md" || fail "project_name placeholder not substituted in AGENTS.md"
grep -q '{{' "$T/AGENTS.md" && fail "unsubstituted placeholder left in AGENTS.md" || true

python3 - "$T/.recipes/lock.json" <<'PY' || fail "lock.json shape wrong"
import json, sys
lock = json.load(open(sys.argv[1]))
[e] = [r for r in lock["recipes"] if r["name"] == "core"]
assert e["version"] == "0.1.0" and e["tier"] == "M", e
assert e["answers"]["project_name"] == "acme-api", e
assert e["answers"]["default_branch"] == "main", "default not merged into answers"
assert e["installedAt"], "installedAt missing"
PY

"$PLUGIN_ROOT/recipes/core/scripts/doctor.sh" "$T" >/dev/null \
  || fail "core doctor should pass on a fresh tier-M install"

echo "PASS: core eval-01 (tier-M install + lock + doctor green)"
