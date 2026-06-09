#!/bin/bash
# SessionStart hook — start every session from an up-to-date main.
#
# Fetches origin and fast-forwards the local main branch to origin/main, so new
# work always branches from current code (the lesson from a stale clone shipping
# a from-scratch duplicate of work that had already landed upstream). It NEVER
# clobbers uncommitted changes and logs-and-exits-0 on any failure (e.g. offline),
# so it can never block a session from starting. Idempotent and non-interactive.
set -uo pipefail

log() { echo "[session-start] $*"; }

# Only meaningful in the remote (Claude Code on the web) environment.
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

cd "${CLAUDE_PROJECT_DIR:-.}" 2>/dev/null || exit 0
git rev-parse --git-dir >/dev/null 2>&1 || { log "not a git repo; skipping"; exit 0; }

# Fetch latest refs (best-effort; offline → log and continue, never fail).
if ! git fetch --quiet origin 2>/dev/null; then
  log "git fetch failed (offline?); leaving repo as-is"
  exit 0
fi

# Never clobber uncommitted work — fetch only, don't switch.
if ! git diff --quiet 2>/dev/null || ! git diff --cached --quiet 2>/dev/null; then
  log "uncommitted changes present; fetched but not switching branches"
  exit 0
fi

# A repo without an origin/main (renamed default?) is left as-is.
git show-ref --verify --quiet refs/remotes/origin/main || { log "origin/main missing; skipping"; exit 0; }

# 'git switch' is branch-only, so it ignores the same-named tag in this repo
# (a plain 'git checkout main' would warn 'refname main is ambiguous').
if ! git switch --quiet main 2>/dev/null; then
  log "could not switch to main; left on current branch"
  exit 0
fi

# --ff-only: advance main to origin/main, or fail cleanly if it diverged (never
# a merge commit, never a clobber).
if git merge --ff-only --quiet origin/main 2>/dev/null; then
  log "main is up to date at $(git rev-parse --short HEAD)"
else
  log "main diverged from origin/main; left as-is for manual resolution"
fi
exit 0
