---
id: 0037
title: "Per-lane cost roll-up — Cost by lane on the Runs page (roadmap v6, item V14)"
status: open
severity: low
group: 0031
github:
links:
  adr:
  prs:
  issues: [0031]
  regression:
assets: []
---

## Summary

The Runs page already rolls the run history up **per workflow** (`RunAggregates`, V1):
run count, failure rate, total & average cost, average duration. But a workflow with
several lanes hides *which lane* carries the cost — or fails — inside that per-workflow
total. A run record already carries everything needed (`RunRecord.Lanes` →
`RunLane{Index, AgentID, Status, Credits}`), but nothing reads it at the
**(workflow, lane)** grain.

**V14 closes that gap** with a `LaneShares`-style **pure reader** in
`internal/telemetry/runs.go` keyed by **(workflow, lane)** over the run history,
surfacing *"which lane in a workflow costs / fails most?"* — the **finest
orchestration-attribution grain**, the per-lane cousin of `RunAggregates`. It is the
**last child** of epic [0031](0031-epic-orchestration-accountability.md) (roadmap v6 —
orchestration accountability / Runs-surface cost parity); on its merge the epic closes.
Source: `docs/NEXT_FEATURES.md` "roadmap v6" section (V14 entry).

## Repro
1. Open the Runs page for a multi-lane workflow run.
2. The per-workflow summary shows the workflow's total/average cost and failure rate, but
   no per-lane breakdown of *which* lane within it is expensive or failure-prone.
   - **Expected:** a "Cost by lane" roll-up showing each lane's share of credits and its
     failure count, so the costliest / most failure-prone lane is identifiable.
   - **Actual (before V14):** lane-level cost/failure attribution is computed nowhere.

## Proposed resolution

- **`internal/telemetry` (`runs.go`):** add `LaneShares(records []RunRecord) []LaneShare`
  where `LaneShare{WorkflowID, LaneIndex, AgentID, Runs, Failures, Credits, Fraction}`,
  keyed by (workflow, lane index). A skipped lane contributes **zero cost** (`RunLane.Credits`
  is already zero) but still counts toward `Runs`; a lane whose `Status` is `"failed"`
  counts as a failure. `AgentID` is the **raw id** (the web layer resolves the label),
  the latest seen in chronological/append order. Sorted by **credits descending** (ties
  broken by workflow id ascending then lane index ascending — a total, deterministic
  order), mirroring the `*Shares` spend readers' ordering choice. Empty history → empty
  slice. **Pure** (no web/forge deps).
- **`internal/web` (`runs.go`):** `runsPartial` resolves each lane share's ids → labels
  under `forgeMu` (`workflowLabel` / `agentLabel`, like the run rows) via a
  `laneShareRow`, and passes the rows to the `runsPage` template.
- **`internal/web/templates/fragments.html` (`runsPage`):** a "Cost by lane" share list
  below the per-workflow summary, reusing the existing `.trend` / `.meter` markup (the
  Telemetry cost-share rows), each row naming the lane by workflow · step · agent with a
  credit-fraction bar and an optional failure count.
- **No schema change, no new store, no new ADR** — a pure telemetry reader returning ids
  (the web layer resolves labels), with no cross-package seam.

## Resolution (shipped)

_(filled on merge — PR number recorded here, in the epic, and in INDEX.)_

## Notes
- **No ADR:** a pure telemetry reader (returns ids; the web layer resolves labels) with
  no persisted-contract change and no cross-package seam. Captured as a pure-reader / UI
  composition in CONTRACTS §3 (the Runs page) and §4 (the new `LaneShares` reader beside
  `RunAggregates`), like the prior Runs-surface additions.
- **Differentiator:** advances the **accountable** half of the orchestration story — the
  Runs surface reaches the finest cost-attribution grain (per lane), below the
  per-workflow roll-up. **Last child** of epic 0031; on merge the epic closes.
- **Numbering:** issue **0037** (next free after 0036), the fourth and final build of epic
  **0031**. No ADR consumed (highest ADR stays 0022).
