---
name: tracking-issues
description: Files and links issues using a hybrid store — markdown under docs/issues/ as the source of truth, mirrored to GitHub Issues via the gh CLI. Groups related issues under epics/labels, attaches screenshots, and cross-links to ADRs, PRs, and the learnings log. Use this whenever a bug or task should be captured, when grouping related work so it doesn't lose context, or when linking an issue to a decision, a test, or a screenshot.
allowed-tools: Read, Write, Edit, Bash, Grep, Glob
---

# Tracking issues (hybrid)

Issues lose value when they lose context — a screenshot with no repro, a bug with no link to the
decision that caused it, ten related tickets no one connected. This skill keeps issues **reproducible
and connected**: markdown files in the repo are the source of truth (diffable, offline, greppable),
mirrored to GitHub so the team gets the familiar UI, labels, and notifications.

## Store model
- **Source of truth:** `docs/issues/NNNN-title.md` with frontmatter (id, title, status, severity,
  group, links, assets). Screenshots in `docs/issues/assets/`.
- **Mirror:** a GitHub issue per file, kept in sync via `gh`. The local file holds the canonical text;
  GitHub holds the conversation + the cross-team view.
- **Index:** `docs/issues/INDEX.md` (regenerated) so the whole set is visible at a glance.

## Workflow
1. **File:** `scripts/new-issue.sh "Topbar overflows at tablet width"` creates the local file from
   [references/issue-template.md](references/issue-template.md). Fill repro, severity, and **evidence**
   (drop the screenshot in `assets/` and link it) — a bug with no repro is noise.
2. **Connect or isolate** (see [references/grouping-and-epics.md](references/grouping-and-epics.md)):
   - **Isolated** (a self-contained fix) → leave `group:` empty; it stands alone.
   - **Connected** → set `group: <epic-id>` and back-link the epic file; link related issues to each
     other, and link the ADR/learning/tech-debt item that gives the issue its *why*.
   - **Blocked by** → record hard ordering as `depends_on: [ids]` (`new-issue.sh --depends id,id`).
     This is the directional dependency graph (distinct from non-blocking `links.issues`): the
     `get-next` picker won't offer a blocked item and uses it to compute what's parallelizable.
     Reserve it for *real* blockers; never form a cycle.
3. **Mirror to GitHub:** `scripts/sync-github.sh` creates/updates the issue via `gh`, applies the
   group as a label, prepends a **relationships header** (`Part of` epic #, `Blocked by` #…) so the
   issue never reads isolated, best-effort links the native **sub-issue + blocked-by** relationships
   (GA 2026), and writes the issue number back. See [references/gh-sync.md](references/gh-sync.md).
4. **Close the loop:** when fixed, link the PR, set `status: closed`, and if it must never regress,
   hand the repro to `authoring-tests`; record it in `logging-learnings`.
   - **Closing an epic closes its body too** (RETROS 0006 A1 — drifted twice: 0069, 0086): tick the
     epic's own acceptance checkboxes and populate its `links`, not just status + INDEX + children.
   - **A new ADR isn't landed until `docs/adr/README.md` has its row** (RETROS 0006 A2) — same
     discipline as CODEMAP regen for moved symbols.

## Why grouping matters
A lone bug fixed in isolation is fine — but most bugs are symptoms of a theme (a layout pass, a
streaming edge, an a11y sweep). An epic that links its children keeps that theme's context in one place,
so picking up issue #7 three weeks later doesn't mean re-deriving why #4, #5, #7 are the same problem.

## This repo
The team already works in GitHub PRs (#11–15) with `gh` available; design goal #3 ("context is
reproducible") is why the markdown is the source of truth. Exploratory findings from `exploring-quality`
land here with their screenshots.
