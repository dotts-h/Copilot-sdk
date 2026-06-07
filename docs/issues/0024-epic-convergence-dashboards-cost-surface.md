---
id: 0024
title: "Epic: convergence dashboards & cost-surface completion (roadmap v4)"
status: open
severity: high
group:
github:
links:
  adr:
  prs: []
  issues: [0025]
  regression:
assets: []
---

## Charter

Roadmap **v3** (epic 0022: extensibility unlocked via MCP secret references, and the
first cost ⋈ orchestration join — Runs aggregations + duration) is shipped and closed.
The two differentiators — **cost-awareness** (the meter) and **orchestration** (the
name) — are now both *deep* **and** beginning to *converge*: V1 (PR #49) gave the Runs
page a per-workflow roll-up (run count / failures / avg cost / avg duration) and a
`RunRecord.Duration()` helper, and the spend ledger is attributable per
agent/workflow/lane (A2/ADR-0018).

But the convergence is still **one surface deep**. The other orchestration and cost
surfaces remain single-store views:

- **The Workflows page is purely navigational.** It is the orchestration entry point,
  yet each row shows only name + step summary — no signal of whether a workflow has ever
  run, how it last ended, how often, or what it costs. The join that V1 built for the
  Runs page (RunStore ⋈ SpendStore keyed by workflow id) applies one surface over.
- **Cost prediction is account-wide only.** `Forecast` projects the whole budget, but
  the A2 attribution tags mean a *bucketed* forecast (per-workflow / per-agent
  trajectory) is a pure reader away.
- **The cost surface has hand-edit-only knobs.** Price overrides, per-session cost, and
  the spend-window selector each close a "still requires editing JSON / hardcoded"
  gap — small, self-contained, compounding.

This epic carries the **v4 convergence + cost-surface** picks. The first build is V4
(Workflows last-run + cost badges). The rest (F3 bucketed forecast; G1 price-override
editor; G2 per-session cost; G3 spend-window selector; the I-tier polish) stay
candidates in `NEXT_FEATURES.md` until promoted.

## Tasks

- [ ] **V4 — Workflow list "last run" + cost badges** →
      [0025](0025-workflow-last-run-cost-badges.md) (no ADR — pure-reader composition
      pre-blessed by the same convergence rationale as ADR-0022 / V1). Each Workflows
      row gains a last-run outcome glyph + relative age, a run count, and total spend —
      joining `RunStore` (last-run signal + count, via `RunAggregates`) and `SpendStore`
      (per-workflow credits, via `WorkflowShares`) keyed by workflow id. Turns the
      orchestration entry point into a cost-aware dashboard. **Build first.**
- [ ] **F3 — Per-workflow / per-agent bucketed burn-rate forecast** (candidate). A
      bucketed `Forecast` variant projecting *"at this pace, the `review` workflow burns
      its share by <date>"* from `DailyTotals` bucketed by the A2 agent/workflow tag.
      Trajectory, not just the historical share. Pure reader, no schema change.
- [ ] **G1 — Settings price-override editor** (candidate). A per-model rate table on the
      Settings page for `config.TelemetryConfig.PriceOverrides` (the last cost knob with
      no UI). Closes the hand-edit-JSON rate step.
- [ ] **G2 — Per-session cost on the Sessions page** (candidate). A `SessionShares`
      reader (parallel to `AgentShares`) so the Sessions picker shows credits + turn
      count per session. Pure reader; `SessionID` is already tagged.
- [ ] **G3 — Telemetry spend-window selector** (candidate). A 30/90-day selector
      threaded through `DailyTotals` truncation, replacing the hardcoded 14-day window.

## Status

**Open.** Build-first pick **V4** (the Workflows last-run + cost badges — the second
cost ⋈ orchestration surface) is the first child. F3/G1/G2/G3 remain candidates in
`NEXT_FEATURES.md` until promoted; this epic stays **open** while they are unbuilt.

## Notes

Per CONVENTIONS: write the failing test first; keep domain logic pure
(`telemetry`/`ctxforge`/`config` dependency-free); `make lint && make test` (floor 65%)
+ `make e2e` for UI; fold ADR/CONTRACTS/REGRESSIONS into the feature branch that
motivates them (ADR-0004). V4 needs **no ADR** — it is a pure-reader composition over
the existing RunStore + SpendStore (no schema change, no new IO), pre-blessed by the
same convergence rationale as ADR-0022.

## Numbering

Highest on disk before this pass: issues → **0023**, epic → **0022**, ADRs → **0022**.
This epic takes **0024**; its first child (V4) takes issue **0025**. No ADR consumed (V4
is pre-blessed by ADR-0022).
