#!/usr/bin/env bash
# sync-github.sh [docs/issues/NNNN-*.md ...] — mirror local markdown issues to
# GitHub Issues via the gh CLI. The local file is canonical; GitHub holds the
# conversation and the cross-team view.
#
# Per file: create the issue if its `github:` field is empty (writing the number
# back), otherwise update title/body/state. The `group:` field becomes a label
# (epic-NNNN) and a "Part of #…" header line so the issue never reads isolated;
# `depends_on:` becomes a "Blocked by …" header line.
#
# Best-effort by design: no gh (or no network) exits 0 with a notice — the local
# store remains the source of truth either way.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

command -v gh >/dev/null 2>&1 || { echo "gh not installed — skipping mirror (local store stays canonical)"; exit 0; }
gh auth status >/dev/null 2>&1 || { echo "gh not authenticated — skipping mirror"; exit 0; }

files=("$@")
if [ ${#files[@]} -eq 0 ]; then
  while IFS= read -r f; do files+=("$f"); done \
    < <(find docs/issues -maxdepth 1 -name '[0-9][0-9][0-9][0-9]-*.md' | sort)
fi

for f in "${files[@]}"; do
  [ -f "$f" ] || { echo "skip: $f (not a file)"; continue; }
  meta="$(FILE="$f" python3 - <<'PY'
import os, re
lines = open(os.environ["FILE"]).read().splitlines()
fm, seen = [], 0
for ln in lines:
    if ln.strip() == "---":
        seen += 1
        if seen == 2: break
        continue
    if seen == 1: fm.append(ln)
def field(name):
    for ln in fm:
        m = re.match(rf"\s*{name}:\s*(.*)$", ln)
        if m: return m.group(1).strip()
    return ""
deps = re.findall(r"\d+", field("depends_on") or "")
print(field("id")); print(field("title")); print(field("status"))
print(field("group")); print(field("github")); print(",".join(deps))
PY
)"
  iid=$(sed -n 1p <<<"$meta"); title=$(sed -n 2p <<<"$meta"); status=$(sed -n 3p <<<"$meta")
  group=$(sed -n 4p <<<"$meta"); ghnum=$(sed -n 5p <<<"$meta"); deps=$(sed -n 6p <<<"$meta")

  header=""
  [ -n "$group" ] && header="Part of epic ${group}. "
  [ -n "$deps" ] && header="${header}Blocked by: ${deps}. "
  body="$(printf '%s\n\n%s\n\n_Mirrored from %s — the markdown file is canonical._' \
          "${header}" "$(sed '1,/^---$/d' "$f" | sed '1,/^---$/d')" "$f")"

  label_args=()
  if [ -n "$group" ]; then
    gh label create "epic-${group}" --force >/dev/null 2>&1 || true
    label_args=(--label "epic-${group}")
  fi

  if [ -z "$ghnum" ]; then
    url="$(gh issue create --title "[$iid] $title" --body "$body" "${label_args[@]}")"
    num="${url##*/}"
    # Write the number back into the frontmatter (the only field gh owns).
    sed -i.bak -E "s/^github:.*$/github: ${num}/" "$f" && rm -f "$f.bak"
    echo "created #$num for $iid"
  else
    gh issue edit "$ghnum" --title "[$iid] $title" --body "$body" >/dev/null
    if [ "$status" = "closed" ]; then gh issue close "$ghnum" >/dev/null 2>&1 || true; fi
    echo "updated #$ghnum for $iid"
  fi
done
