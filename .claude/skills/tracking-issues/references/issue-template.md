# Issue file template

`docs/issues/NNNN-kebab-title.md`:

```markdown
---
id: 0007
title: Topbar overflows at tablet width
status: open            # open | in-progress | closed
severity: medium        # low | medium | high | critical
group:                  # epic id if connected; empty if isolated
github:                 # issue number, filled by sync-github.sh
links:
  adr:                  # adr/NNNN-...md if a decision caused/relates to it
  prs: []               # PRs that address it
  issues: []            # related issue ids
  regression:           # REGRESSIONS.md entry once guarded
assets:                 # screenshots/logs under docs/issues/assets/
  - assets/0007-tablet-overflow.png
---

## Summary
One or two sentences: what's wrong and where.

## Repro
1. step
2. step
Expected: …
Actual: …

## Evidence
![tablet overflow](assets/0007-tablet-overflow.png)

## Notes
Root-cause hypotheses, scope, links to the decision/learning that gives context.
```

## Rules
- **Evidence is mandatory for bugs.** A repro and (for anything visual) a screenshot. No repro → it'll
  be ignored or closed unactioned.
- **id is zero-padded and monotonic**, matching the filename prefix.
- **links are the point** — an issue with no links is a sticky note. Attach the ADR (why), the PR (fix),
  the regression (guard), and sibling issues (theme).
