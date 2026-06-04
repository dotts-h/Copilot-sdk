# Grouping: isolated vs connected, and epics

The core idea you asked for: things that can be fixed in isolation stay isolated; everything else is
connected so it doesn't lose context.

## Isolated issues
A self-contained bug/task with no theme — a typo, a one-line fix, a standalone request. Leave `group:`
empty. It lives as its own file and its own GitHub issue. Don't manufacture connections it doesn't have;
false grouping is as bad as none.

## Connected issues + epics
When several issues are facets of one effort (a "responsive layout pass", a "streaming edge-cases" sweep,
an "a11y conformance" push):
- Create an **epic** file: `docs/issues/NNNN-epic-<name>.md` with `status`, a short charter, and a
  **task list** linking its children.
- Each child sets `group: <epic-id>` in frontmatter and links back to the epic.
- On GitHub, the epic is a tracking issue with a checkbox task-list of the child issues; the group name
  is a **label** applied to every child, so the whole theme is one click away.

## Cross-links beyond the group
Independent of grouping, always attach the connective tissue:
- **ADR** — the decision that caused or constrains the issue.
- **Learning** — once fixed, the `REGRESSIONS.md` entry that guards it.
- **Tech-debt** — if the fix is deferred, the `TECH_DEBT.md` row.
- **Sibling issues** — anything that shares a root cause.

## Why both labels and back-links
Labels give GitHub a fast filter; the frontmatter back-links keep the relationship in the source of truth
so it survives even if the GitHub mirror is rebuilt. Belt and suspenders, cheap to maintain.
