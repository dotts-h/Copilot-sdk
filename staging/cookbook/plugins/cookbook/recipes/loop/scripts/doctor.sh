#!/usr/bin/env bash
# doctor.sh (loop) — conformance check for the `loop` recipe.
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
tier="${tier:-M}"

gaps=0
gap() { echo "GAP: $*"; gaps=$((gaps+1)); }
ok()  { echo "  ok: $*"; }

echo "== loop doctor (tier $tier) on $target"

if [ "$tier" = "S" ]; then
  echo "  ok: tier S — the engineered loop is not installed at this tier (by design)"
  echo "== loop doctor: 0 gap(s)"
  exit 0
fi

[ -f "$target/docs/DEV_LOOP.md" ] \
  && ok "docs/DEV_LOOP.md present" \
  || gap "docs/DEV_LOOP.md missing — the loop has no playbook"

for s in scripts/start-fresh.sh scripts/next-issue.sh; do
  if [ -x "$target/$s" ]; then ok "$s present and executable"
  else gap "$s missing or not executable — the ritual will be hand-rolled (and hand-rolled wrong)"; fi
done

# The loop reads the issues store — it must exist (loop requires issues).
[ -f "$target/docs/issues/INDEX.md" ] \
  && ok "docs/issues/INDEX.md present (picker has a source of truth)" \
  || gap "docs/issues/INDEX.md missing — next-issue.sh has nothing to pick from (install the issues recipe)"

# Bash-parse the installed scripts (a corrupted local edit should surface here).
for s in scripts/start-fresh.sh scripts/next-issue.sh; do
  if [ -f "$target/$s" ]; then
    bash -n "$target/$s" 2>/dev/null || gap "$s does not parse (bash -n failed)"
  fi
done

echo "== loop doctor: $gaps gap(s)"
[ "$gaps" -eq 0 ] || exit 1
