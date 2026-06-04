#!/usr/bin/env bash
# sync-github.sh <issue-file.md> — mirror a local issue file to GitHub via gh.
# Creates the issue if `github:` is empty, else edits it. Writes the number back.
# Idempotent-ish: re-running updates body/labels; it does not delete.
set -euo pipefail
f="${1:?issue file}"
command -v gh >/dev/null || { echo "gh CLI not found" >&2; exit 1; }

field() { awk -v k="$1" -F': *' '$1==k{sub(/^[^:]*: */,"");print;exit}' "$f"; }
title=$(field title); num=$(field github); group=$(field group)

# Render body = everything after the closing frontmatter '---'.
body=$(awk 'f{print} /^---$/{c++} c==2{f=1}' "$f")
tmp=$(mktemp); printf '%s\n' "$body" > "$tmp"

label_args=(); [ -n "$group" ] && label_args=(--label "$group")

if [ -z "$num" ]; then
  num=$(gh issue create --title "$title" --body-file "$tmp" "${label_args[@]}" --json number --jq .number 2>/dev/null \
        || gh issue create --title "$title" --body-file "$tmp" "${label_args[@]}" | grep -oE '[0-9]+$')
  # write the number back into frontmatter
  sed -i -E "s/^github:.*/github: ${num}/" "$f"
  echo "created #$num for $f"
else
  gh issue edit "$num" --body-file "$tmp" "${label_args[@]}" >/dev/null
  echo "updated #$num for $f"
fi
rm -f "$tmp"

# Close on GitHub if the file says closed.
[ "$(field status)" = "closed" ] && gh issue close "$num" >/dev/null 2>&1 && echo "closed #$num" || true
