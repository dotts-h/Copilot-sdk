---
id: 0005
title: "Epic: make it an orchestra (Tier 2)"
status: open
severity: high
group:
github:
links:
  adr:
  prs: []
  issues: [0006]
  regression:
assets: []
---

## Charter

The product is named for orchestration it doesn't yet fully expose. Tier 2 turns
passive surfaces into active control surfaces: the missing forge-CRUD page, and
(later) a multi-agent run/handoff surface that cashes the name. Source:
`docs/NEXT_FEATURES.md` Tier 2.

## Tasks

- [x] **2.2 — MCP server management page + curated defaults** → [0006](0006-mcp-server-management-page.md)
      (ADR-0010). The last forge entity with no UI: an MCP nav page with
      add/edit/toggle/delete, curated stdio servers seeded **disabled** with an
      `exec.LookPath` preflight badging unavailable ones.
- [ ] **2.1 — Multi-agent run / handoff surface** — the big bet; sequential
      handoff first, then parallel lanes. Lead with an ADR. Do after Tier-3 polish
      (3.2 + 3.1) per the recommended sequencing.

## Status

2.2 shipped (closes the forge-CRUD gap). 2.1 deferred per sequencing
(… → 2.2 → 3.2 → 3.1 → 2.1) until the Tier-3 polish and the cost accounting it
leans on have hardened.

## Notes

Recommended sequencing (NEXT_FEATURES): 2.2 first (small, unblocking), then 3.2/3.1,
then 2.1. Keep `ctxforge` pure; reuse the validated-builder + rollback discipline.
