---
id: 0081
title: "Container directives — :::card / :::details as an allowlisted model-authorable block vocabulary (R5)"
status: closed
severity: medium
group: 0076
depends_on: [0077, 0078]
github: 133
links:
  adr: [0047]
  prs: []
  issues: [0076]
  regression:
---

## Summary

An in-house container-directive block parser (the `remark-directive` *syntax* without the
Node toolchain): fenced `:::name{attrs}` … `:::` blocks resolved against a **Go
registry/allowlist** of designed components — initially `:::card` and
`:::details{summary=…}` (collapsible). The model can only name registered components;
attributes are parsed and validated — safe by construction (allowlist > blocklist).

## Why now

Generalizes R2's container shape into an open, governed vocabulary so future designed blocks
are additive (register a name + a `frag`), not new bespoke regex. This is the "designed
component the model can author" mechanism the research flagged as the safest generative-UI
path for a server-rendered stack.

## Acceptance

- [x] `directiveBlock{name, attrs, children}` block; **unregistered names degrade** to escaped
      text / plain blocks (never render unknown markup). Attribute parsing is escape-first and
      bounded (space-separated `key=value` / `key="quoted value"`, span-capped).
- [x] `directiveRegistry` maps name → `frag` template + projector; `:::card` and
      `:::details{summary=…}` ship, both token-styled (guarded semantic tokens only, no new
      color pair → css_tokens AA guard + both-theme axe stay green); `:::details` collapsible
      via native `<details>` (no JS).
- [x] ADR-0047 records the directive grammar + the allowlist invariant. New golden +
      parse/render + attribute + XSS/fuzz cases. `make lint && make test` + `make e2e` green.

## Close-out note

Built on the ADR-0045 block-AST seam: `directiveAt` finds a fence-balanced `:::name` …
`:::` only when the name is in `directiveRegistry` (the trust boundary), and `parseLinesDepth`
threads a bounded nesting depth so a pathological nest degrades to text rather than blowing the
parse stack. Unregistered / over-depth / unterminated all degrade to ordinary escaped markdown.
The demo `demo-sess-1` transcript now seeds a `:::card` + `:::details{summary=…}` so the
resumed-session e2e covers both rendered components (and the native collapse). Attribute grammar
is intentionally small: unquoted values are a single space-delimited token — multi-word needs
quotes (the ADR's normative "space-separated list" rule; the unquoted `summary=Build steps`
example in the ADR prose is informal). Fuzzed 20s over the new fence surface, no crashers.

## Notes

ADR [0047](../adr/0047-container-directives-grammar-and-allowlist.md) records the directive
grammar + allowlist invariant (written ahead of the build). Depends on 0077 (seam) and 0078
(shares the container shape). Sibling: 0076 (epic).
