---
id: 0039
title: "Cost⋈run reconciliation — Ledger vs runs on the Telemetry page (roadmap v7, item V15)"
status: closed
severity: medium
group: 0038
github:
links:
  adr:
  prs: [66]
  issues: [0038]
  regression:
---

## Summary

A workflow's spend lives in **two** persisted stores, reconciled **nowhere**. The cost
ledger (`SpendStore`) attributes spend per workflow over **metered turns**
(`telemetry.WorkflowShares` → credits, rendered as "Cost by workflow" on the Telemetry
page). The run history (`RunStore`) attributes spend per workflow over **recorded runs**
(`telemetry.RunAggregates.TotalCredits`, rendered on the Runs page). These two figures
answer the same question — *how much did this workflow cost?* — from two independent
measurements, and they can **diverge**: a turn metered outside a recorded run inflates the
ledger side; a run whose lanes metered under a different attribution inflates the run
side. A user has no way to **see** — or **trust** — that the two agree.

**V15 closes that gap** with a `telemetry.WorkflowReconcile`-style **pure cross-store
reader** that joins the two roll-ups per workflow and surfaces the **delta**, rendered as
a per-workflow **"Ledger vs runs"** comparison on the Telemetry page (below "Cost by
workflow"). It is the **first child** of epic
[0038](0038-epic-cost-run-reconciliation.md) (roadmap v7 — cost⋈run reconciliation /
converge the two persisted stores); the epic is **born in its PR** (cf. epic 0031 in V11's
PR #59). Source: `docs/NEXT_FEATURES.md` "roadmap v7" section (V15 entry).

## Repro
1. Run a workflow, then open the Telemetry page (ledger) and the Runs page (runs).
2. Both surfaces show a per-workflow credit total, but nothing compares them.
   - **Expected:** a "Ledger vs runs" view showing, per workflow, the ledger credits
     beside the recorded-run credits and their delta — so a divergence (spend metered
     outside a run, or a run metered under a different attribution) is visible and the
     two stores are reconcilable, not just independently accountable.
   - **Actual (before V15):** the two figures are computed in separate stores and
     reconciled nowhere; a divergence is invisible.

## Proposed resolution

- **`internal/telemetry` (`reconcile.go`):** add `WorkflowReconcile(spend []SpendRecord,
  runs []RunRecord) []WorkflowRecon` where `WorkflowRecon{WorkflowID, LedgerCredits,
  RunCredits, Delta}`, keyed by workflow id. `LedgerCredits` sums workflow-attributed
  spend (the empty-workflow chat bucket excluded, like `WorkflowShares`); `RunCredits`
  sums each workflow's recorded runs' metered cost (`RunRecord.Credits`, a skipped lane
  adding zero — the `RunAggregates.TotalCredits` grain); `Delta = LedgerCredits −
  RunCredits`. A workflow present in **one** store but not the other appears with the
  other side zero. Sorted by **absolute delta descending** (the biggest discrepancy first
  — what a reconciliation view exists to surface; ties → ledger credits desc, then
  workflow id asc — a total deterministic order over the unique key). Empty inputs → empty
  slice. **Pure** (no web/forge deps; returns ids).
- **`internal/web` (`pages.go`):** `workflowReconcile()` calls the reader over
  `s.spend.Records()` + `s.runs.Records()`, resolves each row's workflow id → label under
  `forgeMu` (`workflowLabel`), ambers a non-trivial delta (magnitude ≥ a display-tied
  epsilon), and `telemetryPartial` passes the rows to the `telemetryPage` template. Empty
  unless **both** stores are wired.
- **`internal/web/templates/fragments.html` (`telemetryPage`):** a **"Ledger vs runs"**
  comparison table (`table.grid.recon`) below "Cost by workflow" — columns Workflow ·
  Ledger · Runs · Delta, the delta cell ambered when the stores disagree.
- **No schema change, no new store, no new ADR** — a pure cross-record reader returning
  ids (the web layer resolves labels), with no cross-package seam.

## Resolution (shipped)

Shipped in **PR #66**. Built as specified. `telemetry.WorkflowReconcile(spend
[]SpendRecord, runs []RunRecord) []WorkflowRecon{WorkflowID, LedgerCredits, RunCredits,
Delta}` joins the two per-workflow roll-ups: `LedgerCredits` sums workflow-attributed
spend USD then `/USDPerCredit` (bit-identical to the `WorkflowShares` "Cost by workflow"
figure it sits beside, chat bucket excluded), `RunCredits` sums each workflow's recorded
runs' metered credits (`RunRecord.Credits`, a skipped lane adding zero — the
`RunAggregates.TotalCredits` grain), `Delta = LedgerCredits − RunCredits`. A workflow in
**one** store but not the other yields a row with the other side zero. Sorted by
**absolute delta descending** (biggest discrepancy first; ties → ledger credits desc, then
workflow id asc — a total deterministic order over the unique key). Empty inputs → empty
slice. **Pure** (takes two record slices, returns ids; no web/forge deps, no cross-package
seam). The Telemetry page (`workflowReconcile` → `reconcileRow`) renders a **"Ledger vs
runs"** comparison table, resolving workflow ids → display names under `forgeMu` and
**ambering** a delta whose magnitude clears a display-tied epsilon (`0.005` cr — the `%.2f`
rounding boundary). It renders only when **both** a spend store and a run store are wired.

Tests (failing-first): `internal/telemetry` `TestWorkflowReconcile{JoinsBothStores,
OneSidedWorkflowsAppear, DeterministicOrder, Empty}`; `internal/web`
`TestTelemetryPageShowsLedgerVsRunsReconciliation` + `TestTelemetryReconciliationHiddenWithoutRunStore`;
`internal/bootstrap` extends `TestBuildDemoTelemetryShowsTrend` (the demo seeds one
workflow that agrees and one that diverges/ambers). **Self-review (high-effort
/code-review) caught a real bug before CI**: the "Ledger vs runs" block first sat inside
the `telemetryPage` `HasHistory` branch, so a **run-only** workflow (recorded runs but an
empty ledger → no spend trend) — the sharpest divergence — was silently dropped; the block
was hoisted out of that branch and locked by
`TestTelemetryReconciliationRendersWithoutSpendHistory`. e2e: a structural assertion
(`table.recon` + `tr.recon-row`), verified against the Go-rendered HTML. The existing
telemetry + web + bootstrap tests stayed green unchanged. Gates green (`make lint && make
test`; telemetry 96.5%, web 89.0%); CI + e2e green on PR #66.

Docs: NEXT_FEATURES "roadmap v7" section, CONTRACTS §3 (the Telemetry page renders the
reconciliation) and §4 (the `WorkflowReconcile` reader joining `WorkflowShares` +
`RunAggregates`). No new ADR (a pure cross-record reader returning ids; no cross-package
seam). No REGRESSIONS entry — the one bug was found-and-fixed within the same PR before
merge.

## Notes
- **No ADR:** a pure cross-record reader (takes two record slices, returns ids; the web
  layer resolves labels) with no persisted-contract change and no cross-package seam.
  Captured as a pure-reader composition in CONTRACTS §3 (the Telemetry page) and §4 (the
  reconciliation reader joining `WorkflowShares` + `RunAggregates`), like the prior
  cost ⋈ orchestration convergence additions (V1/V4).
- **Differentiator:** converges the two mature surfaces — orchestrated spend becomes
  *reconcilable* across the ledger and the run history, not just *accountable* on each.
  **First child** of epic 0038; on merge the epic records the PR and stays open.
- **Numbering:** issue **0039** (next free after 0037), the first build of epic **0038**.
  No ADR consumed (highest ADR stays 0022).
