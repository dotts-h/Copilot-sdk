#!/usr/bin/env bash
# eval-02 (loop): the picker recommends the unblocked child, lists the blocked
# one with its blocker, flags a childless epic for breakdown, and surfaces the
# parallelizable set.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

git init -q -b main "$T"
"$PLUGIN_ROOT/scripts/install-recipe.sh" --recipe "$PLUGIN_ROOT/recipes/issues" --target "$T" --tier M >/dev/null
"$PLUGIN_ROOT/scripts/install-recipe.sh" --recipe "$PLUGIN_ROOT/recipes/loop" --target "$T" --tier M >/dev/null

cd "$T"
./scripts/new-issue.sh "Data plane" --epic >/dev/null                      # 0001
./scripts/new-issue.sh "Schema module" --group 0001 --severity high >/dev/null   # 0002 (unblocked)
./scripts/new-issue.sh "API over schema" --group 0001 --depends 2 >/dev/null     # 0003 (blocked by 0002)
./scripts/new-issue.sh "Docs sweep" --group 0001 --severity low >/dev/null       # 0004 (unblocked)
./scripts/new-issue.sh "Control plane" --epic >/dev/null                   # 0005 (childless epic)

out="$(./scripts/next-issue.sh)"

echo "$out" | grep -q '\[1\] BUILD issue 0002' || fail "unblocked high-severity child 0002 not recommended first"
echo "$out" | grep -q 'issue 0003 (epic 0001) is BLOCKED by open: 0002' || fail "0003 not listed as blocked by 0002"
echo "$out" | grep -q '\[2\] OPEN epic 0005 has NO child issues yet' || fail "childless epic 0005 not flagged for breakdown"
echo "$out" | grep -q 'no open blocker: 0002, 0004' || fail "parallelizable set should be 0002, 0004"

# Close the blocker -> 0003 becomes buildable.
python3 - <<'PY'
import pathlib, re
p = next(pathlib.Path("docs/issues").glob("0002-*.md"))
p.write_text(p.read_text().replace("status: open", "status: closed"))
idx = pathlib.Path("docs/issues/INDEX.md")
idx.write_text(re.sub(r"(\[0002\][^|]*\| [^|]*\| )open", r"\1closed", idx.read_text()))
PY
./scripts/next-issue.sh | grep -q '\[1\] BUILD issue 0003' || fail "0003 should be buildable once 0002 closed"

echo "PASS: loop eval-02 (picker ranks, blocks, and parallelizes correctly)"
