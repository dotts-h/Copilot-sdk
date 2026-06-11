# 0045. Block-AST seam for markdown rendering

- Status: accepted
- Date: 2026-06-11
- Deciders: Horia
- Related: issue 0077 (epic 0076), [ADR-0001](0001-render-markdown-server-side-for-committed-agent-turns.md), `internal/web/markdown.go`

## Context

`renderMarkdown` (ADR-0001) is a single-pass `string→HTML` transform: block
recognition, inline markup, and HTML assembly are interleaved in one loop. Roadmap v13
(epic 0076) adds rich, token-styled components — callouts, designed code blocks,
tables, container directives, citation cards — and each would otherwise bolt more
regex onto that monolith with no per-block extension point. The rest of the UI renders
data through `frag()` templates; markdown blocks should be able to join that
data→template→token-styled-HTML pattern.

## Considered options

- **Keep the monolith, extend per feature** — each new block grows the single loop.
  Rejected: block recognition and HTML emission stay entangled, every slice risks
  regressing the others, and nothing can route a block through a template.
- **Adopt a markdown AST library (goldmark/gomarkdown)** — full CommonMark tree.
  Rejected for the same reason as in ADR-0001: dependencies and surface area far
  beyond the deliberate safe subset, and the escape-first invariant would have to be
  re-established around foreign rendering hooks.
- **In-house typed block AST** — split the existing renderer into
  `parseBlocks(src) []Block` (recognition) and `renderBlocks([]Block) string`
  (emission), byte-identical for today's subset.

## Decision

We chose the **in-house typed block AST**. `Block` is a small sealed interface
implemented by one struct per block kind (heading, code fence, list, blockquote,
horizontal rule, paragraph); blockquotes hold child `[]Block`, making the tree
recursive. `renderMarkdown` becomes the composition
`renderBlocks(parseBlocks(src))`, with output byte-identical to the previous
renderer so the existing golden, XSS, and fuzz corpus pins the refactor.

The invariant carried over from ADR-0001 and now stated per block: **a block
renderer may emit only whitelisted or templated HTML** — every dynamic string passes
through `inline()`/`html.EscapeString` (escape-first) before assembly, and the set of
emitted tags stays within the fuzz-enforced whitelist. New block kinds (R2–R6) attach
by adding a struct + parser case + renderer (or `frag()` template), never by widening
what an existing renderer may emit.

## Consequences

- Positive: per-block extension point for every later slice; recognition is testable
  separately from emission; block renderers can delegate to `frag()` templates;
  still zero new dependencies and one audited escape path.
- Negative / cost we accept: an internal AST layer to maintain for a renderer that
  previously fit in one function; parse and render must agree on the block model.
- Follow-ups: R2–R6 (issues 0078–0082) each add block kinds on this seam; container
  directives (0081) will need their own ADR for grammar + allowlist.
