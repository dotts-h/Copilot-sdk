---
id: 0080
title: "Tables — GFM pipe tables as token-styled table components (R4)"
status: open
severity: medium
group: 0076
depends_on: [0077]
github: 132
links:
  adr: []
  prs: []
  issues: [0076]
  regression:
---

## Summary

Add GFM pipe-table parsing (`| h | h |` / `|---|:--:|` / rows) as a
`Table{Headers, Aligns, Rows}` block, rendered to a token-styled `<table>` component. The
one common markdown block genuinely missing from the subset today.

## Why now

Agents frequently emit tabular data (comparisons, metrics, plans); today it degrades to
escaped pipe soup. Cleanly expressible as a block type on the R1 seam.

## Acceptance

- [ ] `Table` block with per-column alignment; cells rendered through inline escaping
      (escape-first; no raw HTML). Ragged/malformed rows degrade safely (never panic, never
      leak markup).
- [ ] `frag("table", …)` template; `components`-layer CSS using ladder tokens (zebra/border
      via semantic tokens), tabular numerals where applicable; both-theme axe green.
- [ ] New golden + XSS/fuzz cases for tables. `make lint && make test` + `make e2e` green.

## Notes

Depends on 0077 (block-AST seam). Sibling: 0076 (epic).
