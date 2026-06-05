---
id: 0005
title: "Epic: make it an orchestra (Tier 2)"
status: closed
severity: high
group:
github:
links:
  adr:
  prs: []
  issues: [0006, 0010]
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
- [x] **2.1 — Multi-agent run / handoff surface** → [0010](0010-multi-agent-run-handoff.md)
      (ADR-0013). A forge **Workflow** runs as **lanes**: sequential handoff (each
      lane's output feeds the next) or parallel fan-out, each a sub-run on the seam's
      session lifecycle, watched in a `#lanes` panel. Pure `Workflow` type +
      `Validate` + `CompileWorkflow`; pure `workflowRun` engine; Workflows CRUD page
      with a ▶ run control; per-lane metered cost. Sequential end-to-end (demo +
      e2e); parallel in the model/engine (TECH_DEBT #12).

## Status

**Epic complete.** 2.2 closed the forge-CRUD gap; 2.1 ships the orchestration
surface that cashes the product's name. Remaining roadmap candidates are Tier-3
polish (3.3 / 3.4) or a fresh research pass.

## Notes

Recommended sequencing (NEXT_FEATURES): 2.2 first (small, unblocking), then 3.2/3.1,
then 2.1. Keep `ctxforge` pure; reuse the validated-builder + rollback discipline.
