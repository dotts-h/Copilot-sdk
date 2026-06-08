---
id: 0041
title: "Reconciliation CSV export — WriteReconcileCSV + GET /telemetry/reconcile.csv (roadmap v7, item V17)"
status: closed
severity: low
group: 0038
github:
links:
  adr:
  prs: [75]
  issues: [0038]
  regression:
---

## Summary

V15 ([0039](0039-cost-run-reconciliation.md)) made orchestrated spend **reconcilable**
across the two persisted stores at the **per-workflow** grain (`telemetry.WorkflowReconcile`
→ a "Ledger vs runs" table on the Telemetry page); V16
([0040](0040-per-lane-cost-run-reconciliation.md)) added the **per-`(workflow, lane)`** grain
(`telemetry.LaneReconcile` → "Ledger vs runs by lane"). Both surface the divergence **only on
the Telemetry page** (an HTML view). But the spend ledger and the run history already let
their data **leave the tool** as CSV (`telemetry.WriteCSV` → `GET /telemetry/export.csv`;
`telemetry.WriteRunsCSV` → `GET /runs/export.csv`) so it can be analysed in a spreadsheet —
the reconciliation delta could not.

**V17 closes that gap** with a `telemetry.WriteReconcileCSV`-style **pure writer** — the
export sibling of `WriteCSV`/`WriteRunsCSV` — that serializes the cross-store reconciliation
to CSV, streamed by a new `GET /telemetry/reconcile.csv` route, so the divergence is
analysable outside the app. It is the **third (and final) child** of epic
[0038](0038-epic-cost-run-reconciliation.md) (roadmap v7 — cost⋈run reconciliation / converge
the two persisted stores); on its merge the epic **closes** — the reconciliation surface is
exhausted at the workflow + lane grain, now both on-page and exportable. Source:
`docs/NEXT_FEATURES.md` "roadmap v7" section (V17 entry).

> A burn-rate **forecast annotation** ("the two stores disagree by N cr") was weighed as the
> alternative/companion and **dropped** as an altitude mismatch: the forecast answers *"when
> does the budget run out"*, not *"do the two stores agree"* — bolting a reconciliation note
> onto it mixes two concerns. The CSV export is the clean, well-precedented slice (ADR-0009).

## Repro
1. Open the Telemetry page with both stores wired and a reconcilable divergence — the "Ledger
   vs runs" and "Ledger vs runs by lane" tables show the delta.
2. Try to analyse that divergence outside the app (sort the lanes by delta in a spreadsheet,
   diff it across two snapshots, feed it to a script).
   - **Expected:** a CSV export of the reconciliation — `grain,workflow,lane,ledgerCredits,runCredits,delta`
     — the way spend (`/telemetry/export.csv`) and runs (`/runs/export.csv`) already export.
   - **Actual (before V17):** the delta is visible only in the HTML table; there is no
     reconciliation export, so it can't leave the tool.

## Proposed resolution

- **`internal/telemetry` (`reconcile.go`):** add `WriteReconcileCSV(w io.Writer, spend
  []SpendRecord, runs []RunRecord) error`, the export sibling of `WriteCSV`/`WriteRunsCSV`.
  Fixed header `grain,workflow,lane,ledgerCredits,runCredits,delta`; **one file carries both grains**
  — the per-workflow rows (`WorkflowReconcile`) first, then the per-`(workflow, lane)` rows
  (`LaneReconcile`), each labelled by a leading **`grain` column** (`"workflow"` | `"lane"`) so a
  consumer filters totals from breakdown on `grain` and never double-counts (the `lane` cell is
  blank on a workflow row, the lane index on a lane row). Credits use `csvFloat` (the
  same precision-rounded format as the sibling writers). Rows are the readers' own output, so
  ordering is deterministic (biggest |delta| first within each grain) and a chat-only/empty
  input yields the header alone. **Pure** (the `io.Writer` the caller owns is the only IO).
- **`internal/web` (`telemetry_render.go` + `hub.go`):** `handleReconcileExport` reads
  `s.spend.Records()` + `s.runs.Records()`, sets `text/csv` + `Content-Disposition:
  attachment; filename="my-orchestra-reconcile.csv"`, and calls the writer — mirroring
  `handleSpendExport`/`handleRunsExport`; wired as `GET /telemetry/reconcile.csv` in `hub.go`.
- **`internal/web/templates/fragments.html` (`telemetryPage`):** an "Export CSV" link beside
  the "Ledger vs runs" heading, with a **DISJOINT `reconcile-export` marker class** so it
  can't collide with the spend export's `a.export` selector (the V16 strict-mode lesson);
  rendered only when there is a reconciliation to show (the same `{{if .Reconcile}}` gate as
  the tables).
- **No schema change, no new store, no new ADR** — a pure writer over the existing readers,
  pre-blessed by the ADR-0009 CSV-export precedent (no cross-package seam).

## Resolution (shipped)

Shipped in **PR #75**. Built as specified. `telemetry.WriteReconcileCSV(w io.Writer, spend
[]SpendRecord, runs []RunRecord) error` serializes the cross-store reconciliation to CSV — the
export sibling of `WriteCSV` (spend) and `WriteRunsCSV` (runs). One file carries **both grains**:
the per-workflow rows (`WorkflowReconcile`, V15) first, then the per-`(workflow, lane)` rows
(`LaneReconcile`, V16), each labelled by a leading **`grain` column** (`"workflow"` | `"lane"`) so
a consumer filters totals from breakdown and never double-counts — header
`grain,workflow,lane,ledgerCredits,runCredits,delta`, credits via `csvFloat`. Rows are the readers'
own deterministic output (biggest |delta| first within each grain); empty/chat-only input →
header only. **Pure** (the writer is the only IO; no web/forge deps, no cross-package seam).
Streamed by `GET /telemetry/reconcile.csv` (`handleReconcileExport`, mirroring
`handleSpendExport`/`handleRunsExport`), surfaced as an "Export CSV" link beside the "Ledger vs
runs" heading with a **disjoint `reconcile-export`** marker class. The link renders only when a
reconciliation exists (the `{{if .Reconcile}}` gate).

Tests (failing-first): `internal/telemetry` `TestWriteReconcileCSV` (header + both grains, the
leading `grain` column labelling each, deterministic order) and `TestWriteReconcileCSVHeaderOnlyWhenEmpty`;
`internal/web` `TestReconcileExportReturnsCSV`, `TestReconcileExportHeaderOnlyWithoutStores`,
`TestTelemetryPageHasReconcileExportLink`, `TestTelemetryReconcileExportLinkHiddenWithoutReconciliation`.
e2e: a structural assertion (`a.reconcile-export[href="/telemetry/reconcile.csv"]` visible + the
route streams `text/csv` with the documented header), verified against the Go-rendered demo and
kept DISJOINT from the spend export's `a.export` selector. The existing telemetry + web +
bootstrap + e2e tests stayed green unchanged. Gates green (`make lint && make test`; telemetry
96.0%, web 89.1%).

Docs: NEXT_FEATURES "v7 update (after V17) — roadmap v7 CLOSED", CONTRACTS §3 (the new export
route) and §4 (the `WriteReconcileCSV` writer as a pure serializer of
`WorkflowReconcile`/`LaneReconcile`, the export sibling of `WriteCSV`/`WriteRunsCSV`). No new
ADR (a pure writer over existing readers; pre-blessed by ADR-0009's export precedent). No
REGRESSIONS entry — no bug shipped. On merge, **epic 0038 closes** (the reconciliation surface
exhausted at the workflow + lane grain, on-page and exportable).

## Notes
- **No ADR:** a pure writer (takes two record slices, writes to an `io.Writer` the caller owns)
  with no persisted-contract change and no cross-package seam — the export sibling of
  `WriteCSV`/`WriteRunsCSV`, pre-blessed by the ADR-0009 CSV-export precedent; captured in
  CONTRACTS §3 (the route) and §4 (the writer).
- **Differentiator:** lets the cost ⋈ orchestration *divergence* leave the tool for outside
  analysis, the convergence's natural last reach. **Third and final child** of epic 0038; on
  merge the epic records the PR and **closes** (roadmap v7 done).
- **Numbering:** issue **0041** (next free after 0040), the third build of epic **0038**. No ADR
  consumed (highest ADR stays 0022).
