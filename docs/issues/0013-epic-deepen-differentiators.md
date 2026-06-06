---
id: 0013
title: "Epic: deepen the differentiators (roadmap v2)"
status: closed
severity: high
group:
github:
links:
  adr:
  prs: []
  issues: [0014, 0015, 0016, 0017, 0018, 0020, 0021]
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

**Closed — roadmap v2 complete.** All promoted children shipped: A1
([0014](0014-ledger-derived-budget-rows.md), ADR-0016), B1
([0015](0015-real-parallel-workflow-lanes.md), ADR-0017), A2
([0016](0016-cost-attribution-rollups.md), ADR-0018), A3
([0017](0017-budget-burn-rate-forecast.md), ADR-0019), C2
([0018](0018-textarea-composer.md)), B2
([0020](0020-conditional-branching-workflow-steps.md), ADR-0021), and B3
([0021](0021-workflow-run-history.md), ADR-0022). Cost is now accountable across time
and attributable to who spent it; orchestration's parallel + branching halves are
observable and its runs are persisted history. C1 (MCP secrets / Env editor) remains a
candidate in `NEXT_FEATURES.md`, gated on a secrets-store ADR.

## Notes

Recommended sequencing (`NEXT_FEATURES.md`): **A1 → B1 → A2 → A3 → C2 → C1 →
B2/B3** — followed as A1 → B1 → A2 → A3 → C2 → B2 → B3 (C1 deferred pending its
secrets-store ADR). Kept domain logic pure (`telemetry`/`ctxforge`/`config`
dependency-free); failing test first; `make lint && make test` (floor 65%), `make e2e`
for UI; folded ADR/CONTRACTS/REGRESSIONS into each feature branch (ADR-0004).
