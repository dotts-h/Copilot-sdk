#!/usr/bin/env bash
# Recipe-stable entry point (cookbook new-issue); the canonical implementation
# lives with its skill — one fact, one home. Do not add logic here.
exec "$(dirname "$0")/../.claude/skills/tracking-issues/scripts/new-issue.sh" "$@"
