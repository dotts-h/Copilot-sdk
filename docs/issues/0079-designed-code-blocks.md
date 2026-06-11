---
id: 0079
title: "Designed code blocks — language label + copy affordance + token-styled frame (R3)"
status: closed
severity: medium
group: 0076
depends_on: [0077]
github: 131
links:
  adr: []
  prs: []
  issues: [0076]
  regression:
---

## Summary

Upgrade the fenced-code block from bare `<pre><code class="language-…">` to a designed
frame: a header with the **language label** (using the language hint already parsed at
`markdown.go` but currently ignored) and a **copy** affordance, on the token ladder.
Server-side only — **no syntax-highlighting library** (a future in-house tokenizer could be
a separate slice).

## Why now

Code is the most common rich block in agent output; the language hint is already parsed and
discarded, so the data is free.

## Acceptance

- [x] `codeBlock{lang, code}` block type renders via `frag("codeBlock", …)`; code text
      stays escape-first (`html.EscapeString` before it rides into the template as trusted
      HTML — no hand-built `<pre>` string concat). `.Lang` is escaped contextually by
      html/template in both the visible label and the `language-*` class.
- [x] Copy affordance keeps the **no new JS bundle / build step** constraint — a `copyCode`
      function joins the existing inline `<script>` helpers in `index.html` (called from the
      button's `onclick`), reads the `<code>` `textContent`, and is progressive: the `<pre>`
      stays selectable and a clipboard rejection / missing async API is swallowed.
- [x] Token-styled frame (surface/border/radius from the `--panel`/`--bg`/`--radius-*`
      ladders), mono register; reuses only **guarded** semantic tokens (no new color pair),
      so the `css_tokens_test` axe + the a11y e2e scan stay green in both themes. New golden
      cases (`blocks_test`/`markdown_test`), `button` admitted to the tag whitelist, and the
      sessions e2e extended to assert the frame. `make lint && make test` + `make e2e` green.

## Notes

Highlighting deliberately deferred (would pull a dep — rejected per ADR-0001 doctrine).
Depends on 0077 (block-AST seam). Sibling: 0076 (epic).
