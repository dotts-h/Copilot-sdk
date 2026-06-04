# 0001. Render markdown server-side for committed agent turns

- Status: accepted
- Date: 2026-06-04
- Deciders: Horia
- Related: backlog item #4 (roadmap memory), `internal/web/render.go`, `internal/web/tmpl.go`

## Context

Committed agent turns render through the `richtext` template function, which only
HTML-escapes and converts newlines to `<br>` (`internal/web/tmpl.go`). Agent output
is markdown — code fences, lists, headings, emphasis — and currently shows as flat
text. Backlog item #4 asks for real markdown. The roadmap memory's architecture note
floated a client-side web-component / vanilla-JS island for this. The project's core
design goal is that state reduction and HTML projection are unit-tested against a
MockClient with **no browser**, and the codebase deliberately avoids extra
dependencies (it dropped bubbletea/bubbles/lipgloss in the web cut).

## Considered options

- **Client-side web component** — vendor a JS markdown lib + sanitizer, render in the
  browser. Rejected: rendering becomes browser-only-testable (e2e), undercutting the
  core unit-test goal, and needs a vendored JS lib plus client-side sanitization.
- **Server-side via gomarkdown + bluemonday** — mature libraries. Rejected: pulls
  transitive dependencies (x/net, aho-corasick, css) into a localhost single-user
  tool for a bounded need.
- **Server-side in-house safe-subset renderer** — a small Go renderer producing
  sanitized HTML for a defined markdown subset, applied only to committed agent turns.

## Decision

We chose the **server-side in-house safe-subset renderer**: a new `markdown`
template function (alongside `richtext`) used only by the `turnAgent` fragment.
Streaming `#cur` and every other role (user/reasoning/system/tool) stay on plain
`richtext`. The renderer escapes all text first and only emits a fixed whitelist of
tags, so output is XSS-safe by construction. The deciding trade-off: it keeps
markdown rendering fully unit-testable and fuzzable in Go with zero new dependencies,
which the other two options sacrifice.

## Consequences

- Positive: markdown rendering is a pure function, unit-tested and fuzzed without a
  browser; no new go.mod dependencies; one audited escape path.
- Negative / cost we accept: only a markdown subset is supported (headings, bold,
  italic, inline code, fenced code blocks, links, unordered/ordered lists,
  blockquotes, horizontal rules, paragraphs); anything outside the subset degrades to
  escaped plain text. Streaming text stays plain until the turn commits.
- Follow-ups: a Go fuzz target asserting no unescaped `<`/unsafe attributes survive
  rendering; this ADR supersedes the line-57 "markdown web component" idea in the
  roadmap memory.
