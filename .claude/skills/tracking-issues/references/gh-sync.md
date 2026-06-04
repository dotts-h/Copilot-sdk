# GitHub mirroring with gh

The local markdown is canonical; GitHub is the mirror. Sync is one-directional for the body (local →
GitHub) and pulls back only the issue number and state. This avoids merge conflicts between two editable copies.

## Create / update
```bash
# create (first sync): title + body from the file, label = group, capture the number
num=$(gh issue create --title "<title>" --body-file <rendered-body.md> \
        --label "<group-or-triage>" --json number --jq .number)
# update (subsequent): edit body/labels/state
gh issue edit "$num" --body-file <rendered-body.md> --add-label "<group>"
gh issue close "$num"   # when status: closed
```
`scripts/sync-github.sh` wraps this: it renders the markdown body (stripping frontmatter, rewriting
`assets/…` image links to committed raw URLs), creates or edits the issue, applies the group label, and
writes the returned number into the file's `github:` field.

## Screenshots
GitHub issue bodies can't read local paths. Commit screenshots under `docs/issues/assets/` and reference
them by their repo raw URL (after the commit is pushed), or drag-drop into the issue once and store the
returned URL back in the local file. Keeping the image in-repo means it's never lost if the issue is.

## Epics
An epic syncs as a tracking issue whose body has a `- [ ] #childNumber` task list. As children sync and
get their numbers, update the epic body so GitHub renders live progress.

## Keep it idempotent
Re-running sync on an unchanged file should be a no-op (compare rendered body to the current issue body
before editing). That makes the script safe to run in a pre-push hook or CI without churning the issues.
