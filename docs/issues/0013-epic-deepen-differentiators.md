---
id: 0013
title: "Epic: deepen the differentiators (roadmap v2)"
status: open
severity: high
group:
github:
links:
  adr:
  prs: []
  issues: [0014, 0015]
  regression:
assets: []
---

## Charter

Roadmap v1 (items 1.1–3.4, epics 0001/0005/0007) is shipped and exhausted: the
chat loop is at parity, cost is *active* (guardrails + ledger + per-session
meter), and orchestration *exists* (workflows run as lanes). The 2026-06-06
next-features research pass (`docs/NEXT_FEATURES.md`) re-read the code as-of today
and found that **both differentiators are active but shallow**:

- **Cost-awareness (the meter):** account-wide accounting still reads the live,
  in-process meter, so "remaining this month" resets on restart (TECH_DEBT #9),
  and spend is not yet attributed to the agent/workflow that incurred it.
- **Orchestration (the name):** workflows ship the *sequential* path end-to-end,
  but *parallel* fan-out — the differentiated half — is model/engine-only and
  unobserved offline (TECH_DEBT #12), and a lane surfaces only message + usage.

This epic carries the two **build-first** picks that deepen each differentiator.
The remaining candidates (A2/A3 cost attribution + forecast, B2/B3 branching +
run history, C1/C2 MCP secrets + textarea composer) stay in `NEXT_FEATURES.md`
until promoted.

## Tasks

- [ ] **A1 — Ledger-derived budget rows** → [0014](0014-ledger-derived-budget-rows.md)
      (ADR-0016, promotes TECH_DEBT #9). The Telemetry "Total cost / Monthly
      budget / Remaining" rows, the cost footer, and the hard-cap projection
      baseline read **month-to-date from `SpendStore`** (a new pure
      `telemetry.MonthToDate`) instead of the live `Meter`, so they survive
      restart. **Build first** — it completes the cost differentiator's headline
      promise and unblocks A2/A3.
- [ ] **B1 — Real parallel workflow lanes** → [0015](0015-real-parallel-workflow-lanes.md)
      (promotes TECH_DEBT #12, extends ADR-0013). Give `MockClient` distinct
      session ids + `SessionID`-tagged demo events so a browser-driven **parallel**
      run drives concurrent lanes; surface per-lane tool cards + inline permissions.
      Completes the orchestration differentiator's currently-unobserved half.

## Status

**Open.** Both children are scoped from the v2 research pass; A1 leads (ADR-0016
already written, lead-with-a-decision per ADR-0004). No code shipped this session —
the research pass was the deliverable.

## Notes

Recommended sequencing (`NEXT_FEATURES.md`): **A1 → B1 → A2 → A3 → C2 → C1 →
B2/B3**. Keep domain logic pure (`telemetry`/`ctxforge`/`config` dependency-free);
write the failing test first; `make lint && make test` (floor 65%), `make e2e` for
UI; fold ADR/CONTRACTS/REGRESSIONS into the feature branch (ADR-0004).
