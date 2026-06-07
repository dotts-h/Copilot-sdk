---
id: 0040
title: "Per-lane cost⋈run reconciliation — Ledger vs runs by lane on the Telemetry page (roadmap v7, item V16)"
status: open
severity: medium
group: 0038
github:
links:
  adr:
  prs:
  issues: [0038]
  regression:
---

## Summary

V15 ([0039](0039-cost-run-reconciliation.md)) made orchestrated spend **reconcilable**
across the two persisted stores at the **per-workflow** grain (`telemetry.WorkflowReconcile`
→ a "Ledger vs runs" table on the Telemetry page). But the per-workflow delta can **hide
which lane diverges**: a workflow whose total agrees can still have one lane over-metered in
the ledger and another under-metered in the run history, netting to zero. The ledger already
carries per-`(workflow, lane)` attribution (`SpendRecord.LaneIndex`, ADR-0018) and the run
history already carries per-lane credits (`RunLane.Credits`, the `LaneShares` grain, V14) —
so the **same join one grain finer** is available with no schema change.

**V16 closes that gap** with a `telemetry.LaneReconcile`-style **pure cross-store reader**
that joins the two stores per `(workflow, lane)` and surfaces the **delta**, rendered as a
**"Ledger vs runs by lane"** comparison on the Telemetry page (below the per-workflow
"Ledger vs runs" table). It is the **second child** of epic
[0038](0038-epic-cost-run-reconciliation.md) (roadmap v7 — cost⋈run reconciliation /
converge the two persisted stores); the epic stays **open**. Source:
`docs/NEXT_FEATURES.md` "roadmap v7" section (V16 entry).

> A per-**session** reconciliation was considered and is **not well-supported**:
> `RunRecord` carries no session id (unlike `SpendRecord.SessionID`), so there is no key to
> join the run history on per session. The per-**lane** join is the natural finer grain.

## Repro
1. Run a workflow whose lanes meter unevenly across the ledger and the run history (e.g. a
   turn metered outside one lane's run, or a lane metered under a different attribution), so
   the **per-lane** figures diverge but the **per-workflow** total nets out.
2. Open the Telemetry page. The per-workflow "Ledger vs runs" row shows delta ≈ 0.
   - **Expected:** a "Ledger vs runs **by lane**" view showing, per `(workflow, lane)`, the
     ledger credits beside the recorded-run credits and their delta — so a divergence is
     locatable at the **exact step**, not just the workflow total.
   - **Actual (before V16):** only the per-workflow total is reconciled; a lane-level
     divergence that nets to zero across a workflow is invisible.

## Proposed resolution

- **`internal/telemetry` (`reconcile.go`):** add `LaneReconcile(spend []SpendRecord, runs
  []RunRecord) []LaneRecon` where `LaneRecon{WorkflowID, LaneIndex, LedgerCredits,
  RunCredits, Delta}`, keyed by `(workflow, lane)`. `LedgerCredits` groups workflow-
  attributed spend by `(WorkflowID, LaneIndex)` (the empty-workflow chat bucket excluded,
  like `WorkflowShares`); `RunCredits` sums each recorded run lane's metered cost by
  `(WorkflowID, lane Index)` (a skipped lane adding zero — the `LaneShares` grain); `Delta =
  LedgerCredits − RunCredits`. A `(workflow, lane)` present in **one** store but not the
  other appears with the other side zero; a lane that metered **zero on both sides** (e.g. a
  skipped run lane with no ledger spend) has nothing to reconcile and is **omitted**. Sorted
  by **absolute delta descending** (the biggest discrepancy first; ties → ledger credits
  desc, then workflow id asc, then lane index asc — a total deterministic order over the
  unique key). Empty inputs → empty slice. **Pure** (no web/forge deps; returns ids).
- **`internal/web` (`telemetry_render.go`):** `laneReconcile()` calls the reader over
  `s.spend.Records()` + `s.runs.Records()`, names each row `"<workflow> · step <n>"` (n =
  lane index + 1) resolving the workflow id → label under `forgeMu` (`workflowLabel`, like
  the Runs page's "Cost by lane"), ambers a non-trivial delta (magnitude ≥ the V15
  display-tied epsilon), and `telemetryPartial` passes the rows to the `telemetryPage`
  template. Empty unless **both** stores are wired.
- **`internal/web/templates/fragments.html` (`telemetryPage`):** a **"Ledger vs runs by
  lane"** comparison table (`table.grid.recon.lane-recon`) below the per-workflow "Ledger vs
  runs" table — columns Lane · Ledger · Runs · Delta, the delta cell ambered when the stores
  disagree.
- **No schema change, no new store, no new ADR** — a pure cross-record reader returning ids
  (the web layer resolves labels), with no cross-package seam (exactly like
  `WorkflowReconcile`).

## Resolution (shipped)

_To be filled on merge._

## Notes
- **No ADR:** a pure cross-record reader (takes two record slices, returns ids; the web
  layer resolves labels) with no persisted-contract change and no cross-package seam — the
  per-lane cousin of `WorkflowReconcile` (V15), captured as a pure-reader composition in
  CONTRACTS §3 (the Telemetry page) and §4 (the reconciliation reader joining the lane-
  tagged ledger + `LaneShares`).
- **Differentiator:** converges the two mature surfaces at the **finest** grain —
  orchestrated spend becomes reconcilable per lane, so a divergence is locatable at the
  exact step. **Second child** of epic 0038; on merge the epic records the PR and stays open.
- **Numbering:** issue **0040** (next free after 0039), the second build of epic **0038**.
  No ADR consumed (highest ADR stays 0022).
