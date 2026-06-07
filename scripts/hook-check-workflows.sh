#!/usr/bin/env bash
# PostToolUse hook (.claude/settings.json): after an Edit/Write to a workflow file,
# run the workflow guard so a regression (a CI double-run trigger, or the release
# version-resolution bug) is caught in the dev loop — before push, not just in CI.
# Reads the hook JSON on stdin; only acts on .github/workflows/* edits; exit 2 surfaces
# the failure back to the agent. Silent (exit 0) otherwise. — see scripts/check-workflows.sh
input=$(cat)
file=$(printf '%s' "$input" | jq -r '.tool_input.file_path // empty' 2>/dev/null)
case "$file" in
  *.github/workflows/*)
    root=$(git rev-parse --show-toplevel 2>/dev/null || echo .)
    if ! out=$(bash "$root/scripts/check-workflows.sh" 2>&1); then
      echo "Workflow guard failed after editing $file:" >&2
      echo "$out" >&2
      exit 2
    fi
    ;;
esac
exit 0
