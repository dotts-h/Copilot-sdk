---
id: 0081
title: "Container directives — :::card / :::details as an allowlisted model-authorable block vocabulary (R5)"
status: open
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

- [ ] `Directive{Name, Attrs, Body}` block; **unregistered names degrade** to escaped text /
      plain blocks (never render unknown markup). Attribute parsing is escape-first and
      bounded.
- [ ] Registry maps name → `frag` template; `:::card` and `:::details{summary=…}` ship, both
      token-styled, both-theme axe green; `:::details` collapsible via native `<details>`
      (no JS).
- [ ] ADR records the directive grammar + the allowlist invariant. New golden + XSS/fuzz
      cases. `make lint && make test` + `make e2e` green.

## Notes

ADR [0047](../adr/0047-container-directives-grammar-and-allowlist.md) records the directive
grammar + allowlist invariant (written ahead of the build). Depends on 0077 (seam) and 0078
(shares the container shape). Sibling: 0076 (epic).
