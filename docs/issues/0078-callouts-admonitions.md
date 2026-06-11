---
id: 0078
title: "Callouts / admonitions — GitHub-alert blockquotes as designed callout components (R2)"
status: open
severity: medium
group: 0076
depends_on: [0077]
github: 130
links:
  adr: []
  prs: []
  issues: [0076]
  regression:
---

## Summary

Intercept a blockquote whose first line is `> [!NOTE|TIP|IMPORTANT|WARNING|CAUTION]` and
render a designed **callout** block (icon glyph + semantic token surface + title),
degrading to a plain blockquote anywhere else. Highest visual ROI per hour — the model
already emits this syntax natively.

## Why now

First rich block on the R1 seam; proves the block→`frag()`→token-styled-HTML path end to
end with a self-contained, well-known syntax.

## Acceptance

- [ ] `Callout{Kind, Title, Body}` block type; body is recursively rendered through the
      block-AST (escape-first preserved).
- [ ] One `frag("callout", …)` template; CSS in the `components` layer using only existing
      semantic tokens (info/success/warn/danger → role tokens), passing the
      `css_tokens_test` axe in both themes; glyph + text label, never color alone (WCAG).
- [ ] Unknown `[!FOO]` degrades to a normal blockquote; malformed input degrades safely.
      New golden + XSS cases. `make lint && make test` + `make e2e` green.

## Notes

ADR: callout token mapping (or folded into the R1 ADR). Depends on 0077 (block-AST seam).
Sibling: 0076 (epic).
