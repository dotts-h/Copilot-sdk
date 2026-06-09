#!/usr/bin/env bash
# start-fresh.sh — verify the base, then branch fresh from a trustworthy main.
#
#   start-fresh.sh <new-branch> [--expect-sha <sha>] [--require <path>[,<path>...]]
#
# Encodes the CONVENTIONS "Verify the base before branching" rule so a stale
# origin/main or the repo's `main` *tag* collision fails LOUD, not silent.
#   - never resolves the bare name `main` (a tag named `main` shadows the branch);
#     always branches from the fully-qualified refs/remotes/origin/main.
#   - fast-forwards the local main and prints the resolved SHA.
#   - optionally asserts the tip SHA and that named foundation files exist on it.
# Exit non-zero on any mismatch; the caller must stop on failure.
set -euo pipefail

branch="${1:-}"; shift || true
expect_sha=""; require=""
while [ $# -gt 0 ]; do case "$1" in
  --expect-sha) expect_sha="$2"; shift 2;;
  --require)    require="$2";   shift 2;;
  *) echo "unknown arg: $1" >&2; exit 2;;
esac; done
[ -n "$branch" ] || { echo 'usage: start-fresh.sh <new-branch> [--expect-sha <sha>] [--require a,b]' >&2; exit 2; }

note() { printf '  %s\n' "$*"; }
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

echo "==> fetch origin (prune)"
git fetch origin --prune --tags --quiet

# The repo has a TAG named `main` colliding with the BRANCH. Warn so the human
# knows why we never write the bare name.
if git rev-parse -q --verify "refs/tags/main" >/dev/null 2>&1; then
  note "heads-up: a tag named 'main' exists and shadows the branch — using refs/remotes/origin/main explicitly."
fi

remote_main="$(git rev-parse refs/remotes/origin/main)" || fail "no refs/remotes/origin/main — is the remote reachable?"
echo "==> origin/main is $remote_main"

# Assertions FIRST, against the remote ref — so a wrong/stale base fails loud
# *before* we touch any branch (never strand the caller on main on failure).
if [ -n "$expect_sha" ]; then
  case "$remote_main" in
    "$expect_sha"*) note "SHA assertion ok ($expect_sha)";;
    *) fail "origin/main is $remote_main but --expect-sha was $expect_sha (stale fetch?)";;
  esac
fi
if [ -n "$require" ]; then
  IFS=',' read -ra paths <<< "$require"
  for p in "${paths[@]}"; do
    p="$(echo "$p" | xargs)"; [ -n "$p" ] || continue
    git cat-file -e "$remote_main:$p" 2>/dev/null \
      && note "foundation present: $p" \
      || fail "expected foundation file missing on origin/main: $p (wrong base — branch was likely cut before it merged)"
  done
fi

# Best-effort fast-forward of the local main branch (assertions already passed).
# The branch we cut is taken from the remote ref regardless, so a missing or
# non-ff local main is non-fatal — and we return to the original HEAD before
# creating the new branch, so a switch here can't strand the caller.
#
# SKIP inside a LINKED WORKTREE (Agent isolation:worktree): there `main` is checked
# out in the primary worktree, so `git switch main` fails — and with the error
# swallowed the subsequent ff-merge would run against whatever branch is current,
# parking the merge on the linked checkout instead of advancing main. The branch we
# cut comes from origin/main regardless, so the local-main ff is purely cosmetic and
# safe to skip. A linked worktree is the only state where git-dir != git-common-dir
# (they're equal only in the primary worktree).
git_dir="$(git rev-parse --git-dir)"
common_dir="$(git rev-parse --git-common-dir)"
if [ "$git_dir" != "$common_dir" ]; then
  note "linked worktree detected (Agent isolation:worktree) — skipping the local-main fast-forward; the new branch is cut from origin/main directly."
elif git show-ref -q --verify refs/heads/main; then
  start_ref="$(git symbolic-ref -q --short HEAD || git rev-parse HEAD)"
  git switch main --quiet 2>/dev/null || true
  git merge --ff-only "$remote_main" --quiet 2>/dev/null \
    || note "local main is not a fast-forward of origin/main (diverged or behind a rebase) — cutting from origin/main directly."
  git switch "$start_ref" --quiet 2>/dev/null || true
fi

echo "==> create branch '$branch' from origin/main"
if git show-ref -q --verify "refs/heads/$branch"; then
  fail "branch '$branch' already exists locally — pick a fresh name or delete it first."
fi
git switch -c "$branch" "$remote_main" --quiet
echo "==> on $(git rev-parse --abbrev-ref HEAD) @ $(git rev-parse --short HEAD) (fresh from origin/main)"
