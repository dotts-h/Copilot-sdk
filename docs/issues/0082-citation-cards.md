---
id: 0082
title: "Citation cards — inline [n] markers + server-rendered source cards (R6, stretch)"
status: closed
severity: low
group: 0076
depends_on: [0077]
github: 134
links:
  adr: [0049]
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

- [x] Inline `[n]` → anchor; a `Citation`/sources block renders token-styled source cards;
      markers with no matching source degrade to plain escaped text (research rule: make
      broken citations explicit, never fabricate).
- [x] Progressive disclosure via CSS / `<details>` / htmx only — no JS bundle. Both-theme
      axe green. New golden cases.
- [x] `make lint && make test` + `make e2e` green.

## Notes

Depends on 0077 (block-AST seam); genuinely useful only once a source-bearing producer
surface exists (noted in the epic). Sibling: 0076 (epic).

## Close-out

Shipped (ADR-0049). Reference-style `[n]: url "title"` definitions register sources;
`extractCitations` lifts them out of the prose at parse time and, when any resolve, appends
one `sourcesBlock` that closes the document. The citation scope (`*citeScope`) is derived
from that block by `scopeOf` and threaded through `renderTo`/`inline`, so an inline `[n]`
becomes a `<sup>` anchor to its source card only when a matching source exists — every other
`[n]` degrades to escaped literal text (a broken citation is never fabricated), and an unsafe
URL registers no source (no unsafe href is ever emitted). Progressive disclosure is a
CSS-only hover/focus preview popover plus a native `#cite-n` anchor — no JS. All styling
reuses guarded semantic tokens (no new color pair), so the `css_tokens` AA guard and the
both-theme axe scan stay green. The only new tag is `<sup>`. Seam identity preserved:
`renderMarkdown == renderBlocks ∘ parseBlocks`, with the golden/fuzz/XSS corpus widened for
citations and still byte-identical for citation-free input. Files: `internal/web/citations.go`
(new), `internal/web/markdown.go`, `internal/web/directives.go`,
`internal/web/templates/fragments.html`, `internal/web/static/app.css`. **Epic 0076 closes on
this slice** (R6 was the last child).
