#!/usr/bin/env bash
# Recipe-stable entry point (cookbook next-issue); the canonical implementation
# lives with its skill — one fact, one home. Do not add logic here.
exec "$(dirname "$0")/../.claude/skills/get-next/scripts/next-issue.sh" "$@"
