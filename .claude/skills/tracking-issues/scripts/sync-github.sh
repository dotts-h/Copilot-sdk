#!/usr/bin/env bash
# sync-github.sh <issue-file.md> — mirror a local issue file to GitHub via gh.
# Creates the issue if `github:` is empty, else edits it. Writes the number back.
# Idempotent-ish: re-running updates body/labels; it does not delete.
set -euo pipefail
f="${1:?issue file}"
command -v gh >/dev/null || { echo "gh CLI not found" >&2; exit 1; }

field() { awk -v k="$1" -F': *' '$1==k{sub(/^[^:]*: */,"");print;exit}' "$f"; }
title=$(field title); num=$(field github); group=$(field group)

# Map a LOCAL issue id (e.g. 0002) to its GitHub number via that file's `github:`.
gh_num_of() {
  local g; g=$(ls docs/issues/"$1"-*.md 2>/dev/null | head -1) || return 0
  [ -n "$g" ] && awk -F': *' '$1=="github"{print $2;exit}' "$g"
}
# depends_on ids -> "Blocked by #N, #M" using their mirrored GitHub numbers.
deps=$(field depends_on | tr -d '[]' | tr ',' ' ')
blocked_by=""; dep_ghs=""
for d in $deps; do d=$(printf '%04d' "$((10#$d))" 2>/dev/null) || continue
  n=$(gh_num_of "$d"); [ -n "$n" ] && { blocked_by+="#$n "; dep_ghs+="$n "; }
done
epic_gh=""; [ -n "$group" ] && epic_gh=$(gh_num_of "$(printf '%04d' "$((10#$group))" 2>/dev/null)")

# Render body = a relationships header (the "bigger picture") + everything after
# the closing frontmatter '---'. The header keeps an issue from reading isolated.
rel=""
[ -n "$epic_gh" ]    && rel+="**Part of** epic #$epic_gh"$'\n'
[ -n "$blocked_by" ] && rel+="**Blocked by** ${blocked_by%% }"$'\n'
[ -n "$rel" ]        && rel+=$'\n---\n\n'
body=$(awk 'f{print} /^---$/{c++} c==2{f=1}' "$f")
tmp=$(mktemp); printf '%s%s\n' "$rel" "$body" > "$tmp"

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

# Best-effort NATIVE relationships (GA 2026). Non-fatal: the body header above is
# the always-works fallback; this adds the real hierarchy/graph when the API allows.
repo=$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null || true)
if [ -n "$repo" ] && [ -n "$epic_gh" ]; then
  # Link as a sub-issue of the epic (REST: needs the child's node/issue id).
  child_id=$(gh api "repos/$repo/issues/$num" --jq .id 2>/dev/null || true)
  [ -n "$child_id" ] && gh api -X POST "repos/$repo/issues/$epic_gh/sub_issues" \
      -f sub_issue_id="$child_id" >/dev/null 2>&1 && echo "linked #$num as sub-issue of #$epic_gh" || true
fi
for n in $dep_ghs; do
  [ -n "$repo" ] && gh api -X POST "repos/$repo/issues/$num/dependencies/blocked_by" \
      -f issue_id="$n" >/dev/null 2>&1 && echo "marked #$num blocked-by #$n" || true
done

# Close on GitHub if the file says closed.
[ "$(field status)" = "closed" ] && gh issue close "$num" >/dev/null 2>&1 && echo "closed #$num" || true
