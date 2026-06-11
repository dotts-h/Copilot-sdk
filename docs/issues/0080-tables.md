---
id: 0080
title: "Tables — GFM pipe tables as token-styled table components (R4)"
status: closed
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

- [x] `Table` block with per-column alignment; cells rendered through inline escaping
      (escape-first; no raw HTML). Ragged/malformed rows degrade safely (never panic, never
      leak markup).
- [x] `frag("table", …)` template; `components`-layer CSS using ladder tokens (zebra/border
      via semantic tokens), tabular numerals where applicable; both-theme axe green.
- [x] New golden + XSS/fuzz cases for tables. `make lint && make test` + `make e2e` green.

## Notes

Depends on 0077 (block-AST seam). Sibling: 0076 (epic).

## Resolution

`tableBlock{headers, aligns, rows}` joins the block-AST (ADR-0045) at
`internal/web/markdown.go`. `startsTable` gates on GFM's own rule — a pipe-bearing
header followed by a delimiter row of *matching* cell count — so pipe-bearing prose
is never misread; `tableAligns` reads the `:--`/`:-:`/`--:` markers into a per-column
alignment. The renderer routes each cell through the existing `inline()` pass
(escape-first) and emits via a new `frag("table", …)` template; ragged rows are
padded/truncated to the header width, so a malformed row can never escape the grid.
CSS lives in the components layer and reuses only guarded surface tokens
(`--subtle` border, `--panel` header/zebra fill) — no new color pair, so the
`css_tokens_test` axe + the both-theme e2e scan stay green. Tables admitted to the
tag whitelist; new golden + XSS cases and fuzz seeds added; the sessions e2e asserts
the rendered component. No ADR (R4 sets no new semantics; it instantiates the seam).
