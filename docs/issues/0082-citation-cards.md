---
id: 0082
title: "Citation cards — inline [n] markers + server-rendered source cards (R6, stretch)"
status: open
severity: low
group: 0076
depends_on: [0077]
github: 134
links:
  adr: []
  prs: []
  issues: [0076]
  regression:
---

## Summary

(Stretch.) Inline numbered citation markers `[n]` rendered as anchors that resolve to a
server-rendered **source card** (title + origin), with progressive disclosure (hover
preview → click-through) — the Perplexity pattern. Pure HTML/CSS + htmx, no JS framework.

## Why now

Pairs naturally with any future tool-result / RAG / web-search surface where agent output
cites sources; low structural risk on the R1 seam. Marked stretch — can slip past the core
five without blocking the epic.

## Acceptance

- [ ] Inline `[n]` → anchor; a `Citation`/sources block renders token-styled source cards;
      markers with no matching source degrade to plain escaped text (research rule: make
      broken citations explicit, never fabricate).
- [ ] Progressive disclosure via CSS / `<details>` / htmx only — no JS bundle. Both-theme
      axe green. New golden cases.
- [ ] `make lint && make test` + `make e2e` green.

## Notes

Depends on 0077 (block-AST seam); genuinely useful only once a source-bearing producer
surface exists (noted in the epic). Sibling: 0076 (epic).
