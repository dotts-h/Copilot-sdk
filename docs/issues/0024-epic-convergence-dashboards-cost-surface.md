---
id: 0024
title: "Epic: convergence dashboards & cost-surface completion (roadmap v4)"
status: closed
severity: high
group:
github:
links:
  adr:
  prs: [55]
  issues: [0025, 0026, 0027, 0028, 0029]
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

This epic carries the **v4 convergence + cost-surface** picks. The builds so far: V4
(Workflows last-run + cost badges), F3 (the per-workflow / per-agent bucketed burn-rate
forecast — cost prediction ⋈ attribution), G1 (Settings price-override editor), and now
G2 (per-session cost on the Sessions page), and now G3 (the Telemetry spend-window selector
— the **last child**, on whose merge this epic closes). The I-tier polish stays a candidate
in `NEXT_FEATURES.md` until promoted.

## Tasks

- [x] **V4 — Workflow list "last run" + cost badges** →
      [0025](0025-workflow-last-run-cost-badges.md) (no ADR — pure-reader composition
      pre-blessed by the same convergence rationale as ADR-0022 / V1). Each Workflows
      row gains a last-run outcome glyph + relative age, a run count, and total spend —
      joining `RunStore` (last-run signal + count, via `RunAggregates`) and `SpendStore`
      (per-workflow credits, via `WorkflowShares`) keyed by workflow id. Turns the
      orchestration entry point into a cost-aware dashboard. **Shipped (PR #50).**
- [x] **F3 — Per-workflow / per-agent bucketed burn-rate forecast** →
      [0026](0026-bucketed-burn-rate-forecast.md) (no ADR — pure-reader composition
      pre-blessed by ADR-0019's cost-prediction ⋈ ADR-0018's attribution rationale). A
      bucketed `Forecast` variant projecting *"at this pace, the `review` workflow burns
      ~X cr/day"* from `DailyTotals` bucketed by the A2 agent/workflow tag (reusing the
      account-wide `Forecast` slope unchanged per bucket). Trajectory, not just the
      historical share, beside each Telemetry share bar. Pure reader, no schema change.
      **Shipped.**
- [x] **G1 — Settings price-override editor** →
      [0027](0027-settings-price-override-editor.md) (no ADR — additive UI over the existing
      `PriceOverrides` config field; the live-apply seam is an obvious mirror of the startup
      price-book build + the `refreshBudget` pattern, captured in CONTRACTS + a REGRESSIONS
      note). A per-model rate table on the Settings page (three numeric fields per model),
      persisted through `editConfig` (rollback-on-invalid, with a new non-negative-rate
      `config.Validate` hook) and applied **live** by rebuilding the price book from
      `DefaultPriceBook()` + overrides (`telemetry.BuildPriceBook`) and `Replace`-ing the
      shared book in place — repricing the account meter and every per-session meter without
      a restart. Closes the last hand-edit-JSON cost knob. **Shipped.**
- [x] **G2 — Per-session cost on the Sessions page** →
      [0028](0028-per-session-cost-sessions-page.md) (no ADR — pure-reader composition
      pre-blessed by ADR-0018's attribution ⋈ the `*Shares` pattern, like V4/0025 and
      F3/0026). A `SessionShares(records) []SessionShare{SessionID, Credits, Turns}` reader
      (parallel to `AgentShares`/`WorkflowShares`, **excluding** the empty-`SessionID`
      bucket) joined onto each `copilot.SessionMeta` row by id, so the Sessions picker shows
      *"N turns · X cr"* per session (a no-spend session shows *"no cost yet"*; a
      since-deleted bucket is not shown). Pure reader; `SessionID` is already tagged (no
      schema change). **Shipped (PR #53).**
- [x] **G3 — Telemetry spend-window selector** →
      [0029](0029-telemetry-spend-window-selector.md) (no ADR — a presentation-layer slice
      over the existing pure `DailyTotals` reader, like V4/0025, F3/0026, G1/0027, G2/0028).
      A **14/30/90-day window selector** on the Telemetry "Spend over time" trend (three
      buttons, active one marked), replacing the hardcoded 14-day window: `spendTrend(window
      int)` takes the window; `handlePage` reads `?window=` (default 14, clamp to {14,30,90},
      garbage → 14) and threads it through `renderPage` → `telemetryPartial` → `spendTrend`;
      the `maxUSD` bar-scaling stays **after** the window slice (the REGRESSIONS #14
      invariant, now asserted per window). No schema change, no new store. **Shipped (PR #55).**

## Status

**Closed — all children shipped.** **V4** (Workflows last-run + cost badges — the second
cost ⋈ orchestration surface, PR #50), **F3** (per-workflow / per-agent bucketed burn-rate
forecast — cost prediction ⋈ attribution, issue 0026), **G1** (Settings price-override
editor — the last hand-edit-JSON cost knob, issue 0027), **G2** (per-session cost on the
Sessions page — issue 0028, PR #53), and **G3** (Telemetry spend-window selector — issue
0029, PR #55, the **last child**) are all **shipped**. The roadmap-v4 convergence +
cost-surface theme is complete: the Workflows page, the Telemetry per-bucket forecast, the
Settings price knobs, the Sessions picker, and the spend-over-time trend are now all
cost-aware / inspectable. This epic is **closed**.

## Notes

Per CONVENTIONS: write the failing test first; keep domain logic pure
(`telemetry`/`ctxforge`/`config` dependency-free); `make lint && make test` (floor 65%)
+ `make e2e` for UI; fold ADR/CONTRACTS/REGRESSIONS into the feature branch that
motivates them (ADR-0004). V4 needs **no ADR** — it is a pure-reader composition over
the existing RunStore + SpendStore (no schema change, no new IO), pre-blessed by the
same convergence rationale as ADR-0022.

## Numbering

Highest on disk before this pass: issues → **0023**, epic → **0022**, ADRs → **0022**.
This epic takes **0024**; its children take issue **0025** (V4), **0026** (F3),
**0027** (G1), **0028** (G2), and **0029** (G3). No ADR consumed (V4 is pre-blessed by
ADR-0022; F3 by ADR-0018+0019; G1 is additive UI over an existing config field; G2 is a
pure-reader composition over the existing ledger, pre-blessed by ADR-0018 ⋈ the `*Shares`
pattern; G3 is a presentation-layer slice over the existing pure `DailyTotals` reader —
all captured in CONTRACTS, with a REGRESSIONS note only where a real gotcha was
found-and-fixed).
