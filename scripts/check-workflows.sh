#!/usr/bin/env bash
# check-workflows.sh — guard the CI/release workflow invariants this project learned
# the hard way, so they can't silently regress. Deterministic; run it in CI (the lint
# job) AND as a Claude Code Stop hook (.claude/settings.json), so a violation is caught
# both before merge and before a session ends. Exits non-zero with a clear message on
# any violation. — see docs/CONVENTIONS.md "Quality gates"
#
# Invariants enforced:
#   1. No feature-branch CI double-runs. A workflow triggered on `push` must list ONLY
#      `main` under push.branches — listing a feature branch (e.g. claude/**) there runs
#      the whole pipeline twice per push (the push AND pull_request events both fire).
#   2. Release version resolves from the dispatch input, not the branch ref.
#      `${GITHUB_REF_NAME:-input}` shadows the input on workflow_dispatch (GITHUB_REF_NAME
#      is the branch, "main"), publishing a release tagged after the branch.

set -uo pipefail
cd "$(git rev-parse --show-toplevel 2>/dev/null || echo .)" || exit 2

fail=0

# Rule 1 — push.branches must be exactly [main] (tag-triggered workflows are exempt:
# they have push.tags, not push.branches, so they're skipped).
if ! python3 - <<'PY'; then
import sys, glob, yaml
bad = []
for f in sorted(glob.glob(".github/workflows/*.yml")):
    try:
        wf = yaml.safe_load(open(f)) or {}
    except Exception as e:
        print(f"ERROR: {f}: unparseable YAML: {e}")
        sys.exit(2)
    # PyYAML parses a bare `on:` key as the boolean True (YAML 1.1), so check both.
    on = wf.get("on", wf.get(True, {}))
    if not isinstance(on, dict):
        continue
    push = on.get("push")
    if isinstance(push, dict) and "branches" in push:
        extra = [b for b in (push["branches"] or []) if b != "main"]
        if extra:
            bad.append((f, extra))
for f, extra in bad:
    print(f"ERROR: {f}: push.branches must be [main]; drop {extra} "
          f"— a feature-branch push trigger doubles every CI run "
          f"(push + pull_request both fire).")
sys.exit(1 if bad else 0)
PY
    fail=1
fi

# Rule 2 — release version resolution must not shadow the dispatch input. Strip
# comment lines first, so the explanatory note in release.yml (which quotes the old
# buggy form to document it) doesn't trip the guard — only a real `run:` step counts.
if grep -v '^[[:space:]]*#' .github/workflows/release.yml 2>/dev/null | grep -q 'GITHUB_REF_NAME:-'; then
    echo "ERROR: .github/workflows/release.yml resolves the version with" \
         "\${GITHUB_REF_NAME:-…}; on workflow_dispatch this tags the release after the" \
         "branch (e.g. 'main'). Use \${{ github.event.inputs.tag || github.ref_name }}."
    fail=1
fi

if [ "$fail" -ne 0 ]; then
    echo "workflow checks FAILED" >&2
    exit 1
fi
echo "workflow checks passed"
