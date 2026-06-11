---
id: 0077
title: "Block-AST seam — parse markdown to a typed []Block, render byte-identically (R1)"
status: closed
severity: medium
group: 0076
depends_on: []
github: 129
links:
  adr: [0045]
  prs: []
  issues: [0076]
  regression:
---

## Summary

The keystone. Refactor `internal/web/markdown.go` `renderMarkdown` from a direct
`string→HTML` regex transform into **parse → typed `[]Block` → render**, with
**byte-identical** output for today's subset (headings, lists, code fences, blockquotes,
paragraphs, hr; inline emphasis/code/links unchanged). No new block types yet — this slice
only introduces the seam every later slice (R2–R6) attaches to.

## Why now

There is no per-block extension point today; each rich block would otherwise bolt more
regex onto a monolith. A typed block list lets each block map to a `frag()` template — the
data→template→token-styled-HTML pattern the rest of the UI already uses.

## Acceptance

- [x] `parseBlocks(src) []Block` + `renderBlocks([]Block) string`; `renderMarkdown` becomes
      their composition.
- [x] Output is **byte-identical** for the existing corpus — `markdown_test.go`
      golden/table cases, `TestRenderMarkdownXSS`, and `FuzzRenderMarkdown` pass unchanged
      (escape-first preserved: every input byte escaped before markup). Also verified by a
      differential dump against the pre-refactor renderer over an 80-input corpus.
- [x] No new dependency, no JS, no CSS change. `make lint && make test` (floor 65%) green.
- [x] ADR records the block model and the invariant that block renderers emit only
      whitelisted/templated HTML. — ADR-0045

## Close-out note

Fuzzing the seam surfaced a latent pre-refactor bug: `inline()`'s placeholder restore
could leak NUL sentinels (REGRESSIONS #22). Fixed on the same branch; the two crashers
are committed as fuzz seed corpus under `internal/web/testdata/fuzz/FuzzRenderMarkdown/`.

## Notes

Builds on ADR-0001 (server-side escape-first subset). ADR (new) sets the block-AST
contract. Sibling: 0076 (epic).
