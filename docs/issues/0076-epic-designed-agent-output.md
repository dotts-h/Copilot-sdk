---
id: 0076
title: "Epic: Designed agent output — rich, token-styled components from agent markdown (roadmap v13, R1–R6)"
status: open
severity: medium
group:
depends_on: []
github: 128
links:
  adr: []
  prs: []
  issues: [0077, 0078, 0079, 0080, 0081, 0082]
  regression:
---

## Charter

Today committed agent turns render through an in-house, escape-first markdown subset
(`internal/web/markdown.go`, ADR-0001): a monolithic line-by-line regex `string → HTML`
transform supporting headings, lists, code fences, blockquotes, links, emphasis.
Everything outside the subset degrades to escaped text. Meanwhile the *designed* surfaces
of the UI — tool cards (`renderToolCard`), diff review (`permReview`), lane cards
(`laneCard`), sub-agent rows (`subagentRow`) — are built the right way: data → `frag()`
template → token-styled HTML.

This epic bridges the two. We own the entire server-side render pipeline, so we can
translate agent output into **designed components** (callouts, designed code blocks,
tables, cards/collapsibles, citations) instead of plain markdown — the differentiator the
leading agent UIs lean on. A research pass (internal render-pipeline map + external scan
of Claude Artifacts / Vercel v0 generative-UI / Raycast / Perplexity citations / GitHub
alerts; see [NEXT_FEATURES.md](../NEXT_FEATURES.md) roadmap-v13 section) established that
**~80% of the perceived richness is achievable server-side with zero client JS**, and that
the remaining 20% (live-executing artifacts, Mermaid) is exactly the part that fights our
no-build-chain / sandboxing constraints — so it stays out of scope.

**Hard constraint (the doctrine, non-negotiable):** the external scan recommends
`goldmark + Chroma + bluemonday`; we **reject** that — ADR-0001 + REGRESSIONS already
rejected `gomarkdown`+`bluemonday` (transitive deps vs. one audited escape path). Every
slice is an **in-house** extension of the existing renderer, preserving: escape-first
whitelist (zero attacker-controlled markup), zero dependencies, one CSS file on the token
ladder, both-theme WCAG-AA (the `css_tokens_test` axe), no build chain, no new JS,
unit-testable without a browser.

## Architecture

The renderer is a direct `string→HTML` transform with no extension point. R1 introduces a
small typed **block-AST** between parse and render, so rich blocks become first-class and
each maps to a token-styled `frag()` template — the pattern the rest of the UI already
uses:

```
markdown string → parseBlocks() → []Block{Para, Heading, Code, Callout, Table, Card…}
                → renderBlock() dispatches each → frag("<block>", data) → trusted HTML
```

## Children

- [ ] **R1 · Block-AST seam** ([0077](0077-block-ast-seam.md), M; ADR) — the keystone.
      Refactor `renderMarkdown` to parse into a typed `[]Block` then render, with
      **byte-identical** output for today's subset under the existing fuzz/XSS tests.
- [ ] **R2 · Callouts / admonitions** ([0078](0078-callouts-admonitions.md), S) —
      GitHub-alert syntax `> [!NOTE|TIP|WARNING|CAUTION]` → designed callout (icon +
      semantic token), degrades to blockquote. Highest visual ROI; the model already
      emits this syntax.
- [x] **R3 · Designed code blocks** ([0079](0079-designed-code-blocks.md), S/M) —
      language label + copy affordance + token-styled frame, using the language hint we
      already parse but ignore. No highlighting lib — server-side only.
- [ ] **R4 · Tables** ([0080](0080-tables.md), M) — GFM pipe tables → token-styled table
      component. The one common block genuinely missing.
- [ ] **R5 · Container directives → cards / collapsibles** ([0081](0081-container-directives.md), M/L; ADR) —
      in-house `:::card` / `:::details{summary=…}` block parser → fragment; an allowlisted,
      model-authorable designed-block vocabulary (safe by construction).
- [ ] **R6 · Citation cards** ([0082](0082-citation-cards.md), M; stretch) — inline `[n]`
      markers + server-rendered source cards (Perplexity pattern); pairs with future
      tool-result/RAG surfaces.

## Acceptance (epic)

- [ ] R1 lands with byte-identical output for the current subset (golden + fuzz + XSS
      suites unchanged), establishing the block-AST as the single render path.
- [ ] Each new block type is escape-first (every input byte escaped before markup),
      renders only whitelisted/templated HTML, and adds no dependency, no JS, no second
      stylesheet.
- [ ] Every added color pair passes WCAG-AA in both themes (`css_tokens_test`); geometry
      uses only the radius/space/type ladders.
- [ ] Each child: failing test first, ADR where it sets semantics, `make lint && make
      test` (floor 65%) + `make e2e` green, born in its PR, SemVer minor.

## Out of scope (fights the stack)

Live-executing artifacts (need sandboxed cross-origin iframe + CSP) and Mermaid/diagrams
(no mature pure-Go renderer — opt-in progressive enhancement at best). The research is
explicit these are low-ROI under our constraints; the designed-rendering layer gives ~80%
of the perceived quality with none of the sandboxing/JS burden.

## Sequencing

R1 → {R2, R3, R4 in parallel} → R5 → R6 (stretch). R1 is the keystone (everything renders
through the block-AST); R2/R3/R4 are disjoint block types touching one new `frag` each;
R5 generalizes the container shape R2 introduces into a governed vocabulary; R6 is
inline + card and can slip without blocking the epic.

## Risks

- **Escape-first regression is the cardinal risk.** Every block renderer must escape input
  before layering markup; R1's ADR fixes the invariant and the fuzz/XSS suites guard it on
  every slice.
- **Token sprawl.** New components must consume existing semantic tokens (or add a *pair*
  that passes the axe), never raw color or `--p-*` primitives.
- **Streaming.** Rich blocks render on *committed* turns only (streaming stays plain until
  commit, per ADR-0001); a streamed-component pass is explicitly deferred.

## Notes

Research: internal render-pipeline map + external generative-UI/rich-rendering scan
(roadmap-v13 section of NEXT_FEATURES.md). Builds on ADR-0001 (server-side escape-first
subset), the `frag()`/token system (ADR-0025/0036/0038), and the existing designed-component
precedents (tool cards, diff review, lane cards, sub-agent rows).
