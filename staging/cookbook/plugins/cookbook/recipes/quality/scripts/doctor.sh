#!/usr/bin/env bash
# doctor.sh (quality) — conformance check for the `quality` recipe.
#
#   doctor.sh <target-repo> [--tier S|M|L]
#
# Exit codes: 0 conformant · 1 gaps (readable "GAP:" lines) · 2 usage error.
set -uo pipefail

target="${1:-}"; shift || true
tier=""
while [ $# -gt 0 ]; do case "$1" in
  --tier) tier="$2"; shift 2;;
  *) echo "unknown arg: $1" >&2; exit 2;;
esac; done
[ -n "$target" ] && [ -d "$target" ] || { echo "usage: doctor.sh <target-repo> [--tier S|M|L]" >&2; exit 2; }

if [ -z "$tier" ] && [ -f "$target/.recipes/lock.json" ]; then
  tier=$(TARGET="$target" python3 - <<'PY'
import json, os
try:
    lock = json.load(open(os.path.join(os.environ["TARGET"], ".recipes", "lock.json")))
    print(next((r.get("tier","") for r in lock.get("recipes", []) if r.get("name")=="quality"), ""))
except Exception:
    print("")
PY
)
fi
tier="${tier:-M}"

gaps=0
gap() { echo "GAP: $*"; gaps=$((gaps+1)); }
ok()  { echo "  ok: $*"; }

echo "== quality doctor (tier $tier) on $target"

# The guard script.
if [ -f "$target/scripts/check-workflows.sh" ]; then
  ok "scripts/check-workflows.sh present"
  [ -x "$target/scripts/check-workflows.sh" ] || gap "scripts/check-workflows.sh is not executable"
else
  gap "scripts/check-workflows.sh missing — the workflow invariants have no guard"
fi

# A CI workflow.
if compgen -G "$target/.github/workflows/*.y*ml" >/dev/null; then
  ok "CI workflow(s) present under .github/workflows/"
else
  gap "no workflow under .github/workflows/ — the gates aren't enforced anywhere"
fi

# Run the guard itself (the target's copy if installed, else this recipe's template).
guard="$target/scripts/check-workflows.sh"
[ -f "$guard" ] || guard="$(cd "$(dirname "$0")/.." && pwd)/templates/check-workflows.sh.tmpl"
if out=$( (cd "$target" && bash "$guard") 2>&1 ); then
  ok "workflow invariants hold (guard passed)"
else
  gap "workflow guard FAILED:"
  echo "$out" | sed 's/^/      /'
fi

# The guard must be *wired*, not just present (enforce with hooks, not memory).
if grep -rqs "check-workflows" "$target/.github/workflows/" "$target/Makefile" "$target/package.json" 2>/dev/null; then
  ok "guard is wired into CI and/or the local lint gate"
else
  gap "check-workflows.sh is not referenced by any workflow/Makefile/package.json — wire it into the lint gate"
fi

# Tier M+: the regressions register.
if [ "$tier" != "S" ]; then
  [ -f "$target/docs/REGRESSIONS.md" ] \
    && ok "docs/REGRESSIONS.md present" \
    || gap "docs/REGRESSIONS.md missing — every fixed bug needs a registered guard (tier $tier)"
fi

echo "== quality doctor: $gaps gap(s)"
[ "$gaps" -eq 0 ] || exit 1
