#!/usr/bin/env bash
# new-issue.sh "<title>" [--group <epic-id>] [--severity <low|medium|high|critical>] [--depends id,id]
# Creates docs/issues/NNNN-title.md from the template and prints its path.
set -euo pipefail
title="${1:-}"; shift || true
group=""; sev="medium"; deps=""
while [ $# -gt 0 ]; do case "$1" in
  --group) group="$2"; shift 2;;
  --severity) sev="$2"; shift 2;;
  --depends) deps="$2"; shift 2;;   # comma/space-separated issue ids this one is blocked by
  *) shift;; esac; done
[ -n "$title" ] || { echo 'usage: new-issue.sh "<title>" [--group id] [--severity s] [--depends id,id]' >&2; exit 2; }
# Normalize deps ("1, 2") to a zero-padded YAML inline list ([0001, 0002]).
dep_yaml="[]"
if [ -n "$deps" ]; then
  dep_yaml="[$(printf '%s\n' "$deps" | grep -oE '[0-9]+' \
    | while read -r d; do printf '%04d, ' "$((10#$d))"; done | sed 's/, $//')]"
fi

dir="docs/issues"; mkdir -p "$dir/assets"
last=0
for f in "$dir"/[0-9][0-9][0-9][0-9]-*.md; do
  [ -e "$f" ] || continue; n=$(basename "$f" | cut -c1-4); n=$((10#$n)); [ "$n" -gt "$last" ] && last=$n
done
id=$(printf "%04d" $((last+1)))
slug=$(printf '%s' "$title" | tr '[:upper:]' '[:lower:]' | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//')
path="$dir/${id}-${slug}.md"

cat > "$path" <<EOF
---
id: ${id}
title: ${title}
status: open
severity: ${sev}
group: ${group}
depends_on: ${dep_yaml}
github:
links:
  adr:
  prs: []
  issues: []
  regression:
assets: []
---

## Summary

## Repro
1.
Expected:
Actual:

## Evidence

## Notes
EOF
echo "$path"
