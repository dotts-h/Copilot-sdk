---
id: 0079
title: "Designed code blocks — language label + copy affordance + token-styled frame (R3)"
status: open
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

- [ ] `CodeBlock{Lang, Code}` block type renders via `frag("codeBlock", …)`; code text
      stays escape-first (no hand-built `<pre>` string concat).
- [ ] Copy affordance keeps the **no new JS bundle / build step** constraint — if a copy
      button needs script, use the existing minimal inline pattern and keep it progressive
      (block still readable/selectable without it).
- [ ] Token-styled frame (surface/border/radius from the ladders), mono register; both-theme
      axe green. New golden cases. `make lint && make test` + `make e2e` green.

## Notes

Highlighting deliberately deferred (would pull a dep — rejected per ADR-0001 doctrine).
Depends on 0077 (block-AST seam). Sibling: 0076 (epic).
