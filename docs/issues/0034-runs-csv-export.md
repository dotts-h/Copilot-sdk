---
id: 0034
title: "Runs CSV export — the orchestration sibling of the spend ledger export (roadmap v6, item V11)"
status: closed
severity: medium
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

The persisted **spend ledger** (`telemetry.SpendStore`) is an *accountable* surface: the
Telemetry page exports it as CSV (`telemetry.WriteCSV` → `GET /telemetry/export.csv`) so
spend can be analysed outside the app — the "accountable-ledger" promise the README leads
with. Its orchestration sibling, the **workflow-run history** (`telemetry.RunStore`,
ADR-0022), has a read-only Runs view but **no export**: the run data — including
**skipped** branches that leave no spend record (the reason the run store exists
alongside the ledger) — can't leave the tool.

**V11 closes that parity gap**: a pure `telemetry.WriteRunsCSV` reader + a
`GET /runs/export.csv` route + an "Export CSV" link on the Runs page, mirroring the spend
export exactly. First child of epic [0031](0031-epic-orchestration-accountability.md)
(roadmap v6 — orchestration accountability). Source: `docs/NEXT_FEATURES.md` "roadmap v6"
section.

## Repro
1. Open the Telemetry page → "Spend history" carries an **Export CSV** link; `GET
   /telemetry/export.csv` streams the ledger.
2. Open the Runs page → a per-workflow roll-up + per-lane history, but **no export
   affordance** and **no `/runs/export.csv` route**.
   - **Expected:** the orchestration history is exportable like the cost ledger, and the
     export captures the run-store-unique data (skipped lanes) the spend CSV can't.
   - **Actual (before V11):** the run history can only be read in-app.

## Proposed resolution

- **`internal/telemetry` (`runs.go`):** add `WriteRunsCSV(w io.Writer, records
  []RunRecord) error` — a pure function (the writer the only IO, the caller's), the
  sibling of `WriteCSV`. **One row per lane** (run-level columns repeated on each), so a
  branched run's **skipped** lane is first-class. Fixed columns: `run, workflow, name,
  mode, startedAt, finishedAt, durationSeconds, outcome, lane, agent, status, credits`.
  Credits come straight off `RunLane.Credits` (already in credits, not USD); a zero
  timestamp exports a blank cell, not the `0001-01-01…` zero value.
- **`internal/web` (`pages.go`, `hub.go`):** add `handleRunsExport` (sibling of
  `handleSpendExport`, filename `my-orchestra-runs.csv`, header-only when no run store is
  wired) and register `GET /runs/export.csv`.
- **`internal/web/templates/fragments.html`:** add an "Export CSV" link to the `runsPage`
  header (in a `.trends-head` flex row, mirroring the Telemetry "Spend history" head),
  shown only when run history exists.
- **No schema change, no new store, no new ADR** — a pure additive reader + a GET export
  route over the existing v1 run records.

## Resolution (shipped)

Built as specified. `telemetry.WriteRunsCSV` (`internal/telemetry/runs.go`) flattens the
run history to one CSV row per lane with the fixed 12-column header above (a `csvTime`
helper blanks zero timestamps; `csvFloat` formats durationSeconds + lane credits). The
web layer adds `handleRunsExport` (`pages.go`) and the `GET /runs/export.csv` route
(`hub.go`), and the `runsPage` template gained an "Export CSV" link (shown only when rows
exist). The skipped-lane grain — which leaves no spend record, so the spend CSV can't
carry it — is exported with zero credits, making the run export strictly richer than the
spend export for orchestration analysis.

Tests (failing-first): `internal/telemetry` `TestWriteRunsCSV` (header + one row per lane
incl. a skipped lane row with 0 credits, run-level columns repeated, fixed column order)
and `TestWriteRunsCSVHeaderOnlyWhenEmpty`; `internal/web` `TestRunsExportReturnsCSV`
(text/csv + attachment + header/data), `TestRunsExportHeaderOnlyWithoutStore` (no store →
header-only, never a 500), and `TestRunsPageHasExportLink` (the link renders when history
exists). e2e: the Runs spec (`e2e.spec.ts`) now asserts the export link is visible and
`GET /runs/export.csv` streams a `text/csv` body with the `run,workflow,name` header
(structural, never figures — the demo run store is shared + append-only across the
suite). Gates green (`make lint && make test`; telemetry coverage 96.2%, web 88.9%).

Docs: CONTRACTS §3 (the new `GET /runs/export.csv` route under Telemetry/export, noting
the per-lane flatten + skipped grain) and §4 (the `RunStore` entry notes `WriteRunsCSV`
as a pure-reader export sibling of `WriteCSV`). No new ADR (pure additive reader + route,
pre-blessed by the ADR-0009 export precedent). No REGRESSIONS entry — no bug was
found-and-fixed; the empty-store header-only path and the zero-timestamp blank cell were
guarded preemptively.

## Notes
- **No ADR:** a pure additive reader + a GET export route — no persisted-contract change
  (the on-disk `runs.json` envelope is untouched) and no cross-package seam change.
  Captured as a pure-reader/route composition in CONTRACTS §3/§4, the same way the
  Sessions/Workflows pure-reader compositions were.
- **Differentiator:** completes the **accountable** half of the orchestration story —
  the run history is now exportable like the cost ledger, and the export carries the
  skipped-branch data unique to the run store. First child of epic 0031 (roadmap v6 —
  orchestration accountability / Runs-surface parity).
- **Numbering:** issue **0034** (next free after 0033), first build of epic **0031**. No
  ADR consumed (highest ADR stays 0022).
