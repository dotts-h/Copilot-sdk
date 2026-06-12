#!/usr/bin/env bash
# eval-03 (loop): start-fresh.sh cuts the branch from origin/<default> explicitly,
# fails loud on a missing foundation file (--require) and on a wrong --expect-sha,
# and refuses to reuse an existing branch.
set -euo pipefail
PLUGIN_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
W="$(mktemp -d)"; trap 'rm -rf "$W"' EXIT
fail() { echo "EVAL FAIL: $*" >&2; exit 1; }

# Fixture: a bare origin with one commit, cloned to a work repo.
git init -q --bare -b main "$W/origin.git"
git clone -q "$W/origin.git" "$W/repo"
cd "$W/repo"
git config user.name eval; git config user.email eval@example.invalid
echo "seam" > seam.txt
git add seam.txt && git commit -qm "foundation"
git push -q origin main

"$PLUGIN_ROOT/scripts/install-recipe.sh" --recipe "$PLUGIN_ROOT/recipes/issues" --target . --tier M >/dev/null
"$PLUGIN_ROOT/scripts/install-recipe.sh" --recipe "$PLUGIN_ROOT/recipes/loop" --target . --tier M >/dev/null

# Happy path: branch cut from origin/main, foundation asserted.
./scripts/start-fresh.sh feat/widget --require seam.txt >/dev/null
[ "$(git rev-parse --abbrev-ref HEAD)" = "feat/widget" ] || fail "not on the new branch"
[ "$(git rev-parse HEAD)" = "$(git rev-parse refs/remotes/origin/main)" ] || fail "branch not cut from origin/main"

# Existing branch is refused.
git switch -q - >/dev/null 2>&1 || git switch -qc tmp
set +e
./scripts/start-fresh.sh feat/widget >/dev/null 2>&1 && fail "must refuse an existing branch"

# Missing foundation file fails loud, without branching.
./scripts/start-fresh.sh feat/wrong-base --require does-not-exist.go >/dev/null 2>&1
rc=$?
set -e
[ "$rc" -ne 0 ] || fail "--require on a missing file must fail"
git show-ref -q --verify refs/heads/feat/wrong-base && fail "failed run must not leave the branch behind" || true

# Wrong --expect-sha fails loud.
set +e
./scripts/start-fresh.sh feat/stale --expect-sha deadbeef >/dev/null 2>&1
rc=$?
set -e
[ "$rc" -ne 0 ] || fail "--expect-sha mismatch must fail"

echo "PASS: loop eval-03 (start-fresh verifies the base, fails loud)"
