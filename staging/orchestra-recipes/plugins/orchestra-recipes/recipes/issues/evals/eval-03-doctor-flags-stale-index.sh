#!/usr/bin/env bash
# eval-03 (issues): the doctor flags a stale index (an issue file with no INDEX
# row) and broken frontmatter, with exit code 1.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"
"$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/issues" --target "$T" --tier M >/dev/null

# An orphan issue file added behind the index's back, with bad status.
cat > "$T/docs/issues/0001-orphan-bug.md" <<'EOF'
---
id: 0001
title: Orphan bug
status: wontfix
severity: low
group:
depends_on: []
---
## Summary
EOF

set +e
out="$("$PLUGIN_ROOT/recipes/issues/scripts/doctor.sh" "$T" --tier M 2>&1)"
rc=$?
set -e
[ "$rc" -eq 1 ] || fail "doctor should exit 1 on a stale store (got $rc)"
echo "$out" | grep -q "0001-orphan-bug.md exists but has no row in INDEX.md" || fail "stale index not flagged"
echo "$out" | grep -q "0001-orphan-bug.md: status missing/invalid" || fail "invalid status not flagged"

# Missing INDEX entirely is also a gap.
rm "$T/docs/issues/INDEX.md"
set +e
"$PLUGIN_ROOT/recipes/issues/scripts/doctor.sh" "$T" --tier M >/dev/null 2>&1
rc=$?
set -e
[ "$rc" -eq 1 ] || fail "doctor should exit 1 when INDEX.md is missing (got $rc)"

echo "PASS: issues eval-03 (doctor flags stale index + bad frontmatter)"
