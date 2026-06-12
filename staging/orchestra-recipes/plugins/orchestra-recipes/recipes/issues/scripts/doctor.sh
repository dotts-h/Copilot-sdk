#!/usr/bin/env bash
# doctor.sh (issues) — conformance check for the `issues` recipe.
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

echo "== issues doctor (tier $tier) on $target"

if [ "$tier" = "S" ]; then
  echo "  ok: tier S — the hybrid store is not installed at this tier (by design)"
  echo "== issues doctor: 0 gap(s)"
  exit 0
fi

idx="$target/docs/issues/INDEX.md"
if [ -f "$idx" ]; then
  ok "docs/issues/INDEX.md present"
  grep -q '| id | title | status | children |' "$idx" \
    && ok "Epics table header present" || gap "INDEX.md lacks the Epics table header"
  grep -q '| id | title | status | severity | group | links |' "$idx" \
    && ok "Issues table header present" || gap "INDEX.md lacks the Issues table header"
else
  gap "docs/issues/INDEX.md missing — the store has no index"
fi

for s in scripts/new-issue.sh; do
  if [ -x "$target/$s" ]; then ok "$s present and executable"
  else gap "$s missing or not executable — issues can't be filed deterministically"; fi
done

# Every issue file must carry id/status frontmatter and appear in the INDEX.
if [ -f "$idx" ]; then
  while IFS= read -r f; do
    base="$(basename "$f")"; iid="${base:0:4}"
    grep -q "^id: ${iid}$" "$f" || gap "$base: frontmatter id missing or mismatched"
    grep -qE '^status: (open|in-progress|closed)$' "$f" || gap "$base: status missing/invalid"
    grep -q "$base" "$idx" || gap "$base exists but has no row in INDEX.md — index is stale"
  done < <(find "$target/docs/issues" -maxdepth 1 -name '[0-9][0-9][0-9][0-9]-*.md' 2>/dev/null | sort)
fi

echo "== issues doctor: $gaps gap(s)"
[ "$gaps" -eq 0 ] || exit 1
