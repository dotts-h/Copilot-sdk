#!/usr/bin/env bash
# eval-02 (issues): new-issue.sh files an epic and dependent children with
# monotonic ids, correct frontmatter (group, depends_on), and INDEX rows.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"
"$PLUGIN_ROOT/scripts/install-recipe.sh" \
  --recipe "$PLUGIN_ROOT/recipes/issues" --target "$T" --tier M >/dev/null

cd "$T"
p1=$(./scripts/new-issue.sh "Ship the widget surface" --epic)
p2=$(./scripts/new-issue.sh "Widget data model" --group 0001 --severity high)
p3=$(./scripts/new-issue.sh "Widget UI" --group 0001 --depends 2)

[ "$(basename "$p1")" = "0001-epic-ship-the-widget-surface.md" ] || fail "epic filename wrong: $p1"
[ "$(basename "$p2" | cut -c1-4)" = "0002" ] || fail "child id not monotonic: $p2"
grep -q '^title: Epic: Ship the widget surface$' "$p1" || fail "--epic did not prefix the title"
grep -q '^group: 0001$' "$p2" || fail "group not recorded on child"
grep -q '^depends_on: \[0002\]$' "$p3" || fail "depends_on not normalized to [0002]"
grep -q '^severity: high$' "$p2" || fail "severity not recorded"

# INDEX rows appended for all three.
for b in 0001 0002 0003; do
  grep -q "\[$b\]" docs/issues/INDEX.md || fail "INDEX row missing for $b"
done

"$PLUGIN_ROOT/recipes/issues/scripts/doctor.sh" "$T" --tier M >/dev/null \
  || fail "doctor should pass with a consistent store"

echo "PASS: issues eval-02 (epic + children, ids, depends_on, INDEX rows)"
