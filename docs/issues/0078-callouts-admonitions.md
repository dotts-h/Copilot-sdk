---
id: 0078
title: "Callouts / admonitions — GitHub-alert blockquotes as designed callout components (R2)"
status: closed
severity: medium
group: 0076
depends_on: [0077]
github: 130
links:
  adr: [0046]
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

- [x] `calloutBlock{kind, title, children}` block type; body is recursively rendered
      through the block-AST (escape-first preserved — body is the already-escaped child
      renderers' output, the title rides the `inline()` pass).
- [x] One `frag("callout", …)` template; CSS in the `components` layer using only existing
      semantic tokens (note→`--accent2` / tip→`--good` / important→`--accent` /
      warning→`--warn` / caution→`--bad`), passing the `css_tokens_test` axe in both
      themes; glyph + text label, never color alone (WCAG).
- [x] Unknown `[!FOO]` degrades to a normal blockquote (marker text intact); malformed
      input degrades safely. New golden + XSS cases + fuzz seeds; `div`/`span` admitted to
      the tag whitelist. `make lint && make test` + `make e2e` green (the one perf-budget
      failure under full-suite CPU contention passes in isolation — unrelated to this slice).

## Notes

ADR: [ADR-0046](../adr/0046-callout-blocks-token-mapping.md) (callout syntax + token
mapping). Depends on 0077 (block-AST seam). Sibling: 0076 (epic). The demo `demo-sess-1`
transcript now seeds a NOTE + WARNING callout so the resumed-session e2e covers the
rendered component.
