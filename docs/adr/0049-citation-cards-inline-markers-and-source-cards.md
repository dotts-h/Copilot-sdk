# 0049. Citation cards — inline `[n]` markers + reference-defined source cards

- Status: accepted
- Date: 2026-06-11
- Deciders: Horia
- Related: issue 0082 (epic 0076), [ADR-0045](0045-block-ast-seam-for-markdown-rendering.md), [ADR-0046](0046-callout-blocks-token-mapping.md), [ADR-0047](0047-container-directives-grammar-and-allowlist.md), [ADR-0001](0001-render-markdown-server-side-for-committed-agent-turns.md), [ADR-0025](0025-semantic-color-token-tier.md), `internal/web/markdown.go`, `internal/web/citations.go`

## Context

R6 is the last (stretch) slice of epic 0076: numbered citation markers in agent
prose that resolve to server-rendered **source cards** — the Perplexity pattern, where
`[n]` is a marker you can hover for a preview and click through to the source. Unlike
R2–R5, a citation is **both** an inline thing (the `[n]` mid-sentence) and a block thing
(the list of source cards), and the two halves must agree: a marker resolves only when a
matching source exists, and a marker with **no** source must degrade to plain text —
never fabricate a link (the research rule for citations: a broken citation is shown
literally).

Three things had to be settled before code: the **syntax** that declares a source, how a
marker is **resolved** to a card (and what an unmatched marker does), and how the
document-level set of sources reaches the **inline** pass without breaking the
ADR-0045 seam (`renderMarkdown == renderBlocks ∘ parseBlocks`, with the existing
golden/fuzz/XSS corpus pinning byte-identical output for citation-free input).

## Considered options

- **A `:::sources` container directive (ADR-0047) holding a list of sources.** Rejected as
  the primary mechanism: the directive vocabulary is *block* shaped, but the marker is
  *inline* — the `[n]` still needs an inline pass and a document-level lookup, so the
  directive would only relocate the source list, not the hard part. Reference-style
  definitions are also the established markdown idiom for "declare a target, refer to it by
  number" (link reference definitions), so they read naturally in agent prose.
- **Adopt a markdown library's footnote/citation extension.** Rejected for the reason the
  whole epic rejects libraries (ADR-0045): a foreign rendering path and the escape-first
  invariant re-established around it.
- **Thread the source set through a render context on every `renderTo`.** This is the
  mechanism, but the question was *where the source set lives*. Carrying it as a separate
  parse-time return value threaded beside the blocks would have made `renderMarkdown` no
  longer a pure composition of the seam (a caller of `renderBlocks` with hand-built blocks
  would get no citations).
- **Carry the sources *inside* the block list and derive the scope from it.** Chosen — see
  Decision.

## Decision

### Syntax — reference-style definitions, inline numeric markers

- A **definition** is a line that, trimmed, is `[n]: <url>` with an optional quoted title:
  `[1]: https://go.dev "Go"`. The number is `\d+`, the URL is whitespace-free, the title is
  optional. A definition-shaped line **never renders as prose** — it is lifted out at parse
  time (`extractCitations`).
- A **marker** is an inline `[n]`. In the inline pass (after code spans and links are
  stashed, so only bare markers remain), a marker whose number has a matching source becomes
  a `<sup>` anchor to the source card; **any other `[n]` is left as escaped literal text.**

### Resolution — the source set is the trust/escape boundary

A definition registers a source only when `n ≥ 1`, the URL passes `safeURL` (the same
scheme allowlist the link pass uses — `javascript:`/`data:`/`vbscript:` register *nothing*,
so their markers degrade and no unsafe href is ever emitted), and `n` is not already
defined (first definition wins). Resolved sources are sorted by number and appended to the
block list as a **single `sourcesBlock`** that closes the document.

> **The citation scope travels inside the block list.** `renderBlocks` derives the set of
> defined numbers by finding the `sourcesBlock` (`scopeOf`) and threads it (`*citeScope`,
> nil when there are no citations) through `renderTo`/`inline`. So `renderMarkdown` stays
> exactly `renderBlocks ∘ parseBlocks` for **all** input, a hand-built block list resolves
> markers iff it carries a `sourcesBlock`, and citation-free input renders byte-identically
> to the pre-R6 subset (nil scope ⇒ the inline citation pass is skipped entirely).

### Escape-first, no new tag surface but one

Every dynamic part is escape-first: the marker number is an integer; the title rides the
`inline()` pass (whitelist-only tags); the origin and URL are escaped contextually (the URL
in the card's `href`, validated by `safeURL` at parse). The inline marker is **hand-built**
(like the code/link spans beside it) rather than routed through `frag()`, because `inline()`
must not statically reference the template engine — `frag → pageTemplates → funcMap →
renderMarkdown → … → inline` would be a package-initialization cycle (the `renderTo` methods
reach `frag` only through interface dispatch, which the cycle analyzer cannot follow). The
`sourcesBlock` cards *do* render through `frag()` (a `renderTo` method, interface-dispatched,
so no cycle). The only new tag is `<sup>`; the source list reuses `ol`/`li`/`a`/`span`.

### Progressive disclosure — CSS only

The marker is a native in-page anchor (`#cite-n`) — click-through is the browser's. The
hover/focus **preview** (title + origin) is a CSS-only popover (`.cite:hover .cite-pop`,
`.cite:focus-within .cite-pop`), so there is no JS and no second stylesheet. All citation
styling reuses **guarded semantic tokens** (`--accent2` link, `--dim` origin, the
`--panel`/`--subtle` surface ladder, the radius/shadow ladders) — **no new color pair** — so
the `css_tokens_test` AA guard and the both-theme axe scan stay green.

## Consequences

- Positive: the epic's last slice lands without widening the trust surface — one new tag,
  one escape path, no dependency, no JS, no second stylesheet. The seam identity
  (`renderMarkdown == renderBlocks ∘ parseBlocks`) is preserved by carrying the source set
  inside the block list, so the existing composition/fuzz/XSS pins still hold and a broken
  citation provably degrades to text.
- Negative / cost we accept: citation resolution is document-flat (definitions are lifted
  from anywhere and pooled, not scoped per section); the marker grammar is bare `[n]` (no
  `[n,m]` ranges, no author-named labels); the preview popover duplicates the card's
  title/origin inline (the price of a CSS-only, content-local preview). These are acceptable
  for a stretch slice and can be revisited if a source-bearing producer (tool-result/RAG)
  surfaces real demand.
- Follow-ups: epic 0076 closes on this slice. A future RAG/web-search/tool-result surface
  that emits sources is the natural consumer; if per-section citation scoping or richer
  marker grammar is ever needed, it extends `extractCitations` + the inline pass without
  touching the seam.
