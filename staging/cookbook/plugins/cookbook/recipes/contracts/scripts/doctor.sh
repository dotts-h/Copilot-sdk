#!/usr/bin/env bash
# doctor.sh (contracts) — conformance check for the `contracts` recipe.
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

echo "== contracts doctor (tier $tier) on $target"

c="$target/docs/CONTRACTS.md"
if [ -f "$c" ]; then
  ok "docs/CONTRACTS.md present"
  for h in "## Provides" "## Consumes"; do
    grep -q "^$h" "$c" \
      && ok "'$h' section present (fleet-checkable)" \
      || gap "docs/CONTRACTS.md lacks the '$h' section — fleet cross-checks have nothing to read"
  done
else
  gap "docs/CONTRACTS.md missing — the repo's stable promises have no registry"
fi

# Fleet wiring is consistent: a constellation implies the fleet doctor (and vice versa).
if [ -f "$target/constellation.yaml" ]; then
  ok "constellation.yaml present (fleet member)"
  if [ -x "$target/scripts/fleet-doctor.sh" ]; then
    ok "scripts/fleet-doctor.sh present and executable"
    if out=$( (cd "$target" && bash scripts/fleet-doctor.sh constellation.yaml) 2>&1 ); then
      ok "fleet provider/consumer pairs are satisfied"
    else
      gap "fleet-doctor reports unsatisfied contracts:"
      echo "$out" | grep '^GAP:' | sed 's/^/      /'
    fi
  else
    gap "constellation.yaml exists but scripts/fleet-doctor.sh is missing — the fleet pairs are unchecked"
  fi
else
  if [ -f "$target/scripts/fleet-doctor.sh" ]; then
    gap "scripts/fleet-doctor.sh installed but no constellation.yaml — nothing maps the fleet"
  else
    ok "not a fleet member (no constellation.yaml) — Provides/Consumes still documented locally"
  fi
fi

echo "== contracts doctor: $gaps gap(s)"
[ "$gaps" -eq 0 ] || exit 1
