---
id: 0035
title: "Total cost on the per-workflow Runs summary (roadmap v6, item V13)"
status: closed
severity: low
group: 0031
github:
links:
  adr:
  prs: []
  issues: [0031]
  regression:
assets: []
---

## Summary

The Runs page's **per-workflow summary** (`telemetry.RunAggregates` → `runSummaryRow`,
ADR-0022/V1) shows run count, failure rate, **average** cost, and average duration — but
not the workflow's **cumulative** orchestrated spend. That total is **already computed**
on `RunAggregate.TotalCredits` (it's the numerator of the average) — it's just not
surfaced. The cost surface's analogue, the Telemetry "Cost by workflow" share, reads
*total* spend; the Runs summary should too, so a high run count × a high per-run cost is
distinguishable from one expensive run.

**V13 closes that parity gap**: surface `RunAggregate.TotalCredits` as a "Total cost"
column beside "Avg cost" in the `runsPage` summary table. Second child of epic
[0031](0031-epic-orchestration-accountability.md) (roadmap v6 — orchestration
accountability / Runs-surface parity). Source: `docs/NEXT_FEATURES.md` "roadmap v6"
section.

## Repro
1. Open the Runs page after a workflow has run more than once.
2. The per-workflow roll-up shows **Avg cost** but no cumulative total.
   - **Expected:** the workflow's *cumulative* orchestrated spend reads beside its
     average — the orchestration analogue of the Telemetry per-workflow share's total.
   - **Actual (before V13):** only the average is shown; the total (already on
     `RunAggregate`) is invisible, so a workflow that ran 10 cheap times and one that ran
     once expensively can read the same.

## Proposed resolution

- **`internal/web` (`runs.go`):** add `"TotalCredits": telemetry.FormatCredits(a.TotalCredits)`
  to `runSummaryRow` — no telemetry change (the field already exists on `RunAggregate`).
- **`internal/web/templates/fragments.html` (`runsPage`):** add a `Total cost` column
  header and a `run-summary-totalcost` cell, placed before the existing `Avg cost`
  column so cumulative-then-average reads naturally. Update the summary caption.
- **`internal/web/static/app.css`:** a `.run-summary-totalcost` rule (the good-colour,
  bold treatment, mirroring `.run-summary-name`).
- **No schema change, no new store, no new ADR** — a pure presentation-layer slice over
  an already-computed aggregate field.

## Resolution (shipped)

Built as specified. `runSummaryRow` (`internal/web/runs.go`) now maps
`RunAggregate.TotalCredits` through `telemetry.FormatCredits`; the `runsPage` summary
table (`fragments.html`) gained a "Total cost" header + a `run-summary-totalcost` cell
before "Avg cost", and the caption now reads "total &amp; average cost". A
`.run-summary-totalcost` CSS rule styles the cell. No telemetry change — `TotalCredits`
was already rolled up by `RunAggregates`.

Tests (failing-first): `internal/web` `TestRunsSummaryShowsTotalAndAvgCredits` — two runs
of one workflow (credits 2.6 + 1.0) so total (`3.60 cr`) ≠ avg (`1.80 cr`), asserting
both render plus the new `run-summary-totalcost` cell and `Total&nbsp;cost` header. e2e:
the Runs spec (`e2e.spec.ts`) asserts the `.run-summary-totalcost` cell is visible
(structural, never figures — the demo run store is shared + append-only across the
suite). The existing telemetry + web + bootstrap tests stayed green unchanged. Gates
green (`make lint && make test`; telemetry coverage 96.0%, web 88.9%).

Docs: CONTRACTS §3 (the Runs page summary now lists total & average cost) and §4 (the
`RunAggregates` reader entry notes `TotalCredits` is surfaced beside `AvgCredits`). No
new ADR (pure presentation-layer slice over an existing computed field). No REGRESSIONS
entry — no bug was found-and-fixed.

## Notes
- **No ADR:** a pure presentation-layer slice over an already-computed `RunAggregate`
  field — no persisted-contract change and no cross-package seam. Captured as a
  pure-reader/UI composition in CONTRACTS §3/§4, like the prior Runs-summary additions.
- **Differentiator:** advances the **accountable** half of the orchestration story — the
  Runs summary now reads a workflow's cumulative orchestrated spend, the analogue of the
  Telemetry per-workflow share's total. Second child of epic 0031.
- **Numbering:** issue **0035** (next free after 0034), second build of epic **0031**. No
  ADR consumed (highest ADR stays 0022).
