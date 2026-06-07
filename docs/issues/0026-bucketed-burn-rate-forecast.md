---
id: 0026
title: Per-workflow / per-agent bucketed burn-rate forecast (roadmap v4, item F3/V7)
status: closed
severity: medium
group: 0024
github:
links:
  adr:
  prs: []
  issues: [0024, 0017, 0016]
  regression:
assets: []
---

## Summary

Cost is **predictive** (a trailing-window burn-rate `Forecast`, ADR-0019) and **attributable**
per agent/workflow (the A2 spend tags, ADR-0018) — but the two never meet for *prediction*.
`Forecast` is **account-wide only**: it slopes the whole ledger to project when the monthly
allowance runs out. `AgentShares`/`WorkflowShares` show each agent/workflow's **historical**
share, but there is no per-bucket **trajectory** — *"at this pace, the `review` workflow burns
its share by …"*. F3 is that convergence: bucket `DailyTotals` by the A2 agent/workflow tag and
run the **same** `Forecast` slope per bucket, surfacing a per-bucket rate + month-projection
sentence beside each share bar on the Telemetry page. Pure readers over existing records, **no
schema change**. Source: `docs/NEXT_FEATURES.md` item F3/V7.

## Repro
1. Open the Telemetry page after a few days of mixed agent/workflow spend.
   - **Expected:** beside each "Cost by agent" / "Cost by workflow" bar, a small trajectory line
     — *"at ~X cr/day, on pace for ~Y cr this month"* — so the page shows each tag's *pace*, not
     just its accumulated share; an idle/too-new bucket says so instead of inventing a figure.
   - **Actual (before F3):** the per-agent / per-workflow view shows only the historical share
     bar; the burn-rate projection (A3) exists only account-wide.

## Proposed resolution (pure readers — no schema change)

- **`internal/telemetry` (pure, unit-tested):** `DailyTotalsBy(records, keyOf, includeEmpty)
  map[string][]DayTotal` buckets the per-day series by agent/workflow tag (mirroring `shareBy`'s
  `keyOf`), and `BucketForecasts(records, budget, now, keyOf, includeEmpty) []BucketProjection`
  runs the **existing `Forecast` unchanged** over each bucket's own daily series, sorted by spend
  descending then key ascending (deterministic, like `*Shares`). Reusing `Forecast` keeps the
  three ADR-0019 slope gotchas single-sourced (elapsed-observed-days denominator clamped to ledger
  age **per bucket**, the `⌈DaysToCap⌉` match, the single `now`). Per-bucket framing: a bucket has
  no own allowance, so the account-wide `Forecast` is run per bucket only to compute the bucket's
  **own rate**; the view surfaces rate + month projection, **not** a per-bucket exhaustion date.
- **`internal/web` (Telemetry view):** `spendShares(now)` joins a per-bucket trajectory sentence
  onto each `AgentShares`/`WorkflowShares` row (keyed by raw id before it is resolved to a label
  under `forgeMu`), threading **one** `now` into both the per-bucket `Forecast` and the month
  projection. Degenerate cases are guarded (no budget → no trajectory line; an idle/too-new bucket
  → the Idle sentence, never a bogus date). All values through `html/template` (ADR-0001).
- **Tests:** unit — `BucketForecasts` splits a mixed-tag ledger and runs the slope per bucket (a
  heavy bucket projects a steeper rate than a light one; a single-day bucket clamps to one observed
  day, not a near-zero rate; a window-empty bucket → `ProjectionIdle`; determinism; empty / no-budget;
  the workflow bucketing excludes the empty key). web — the page renders a `trajectory` cell per
  agent+workflow bucket (the OK pace, the Idle sentence on the degenerate path) and renders cleanly
  with no spend store / no budget (no panic, no trajectory). e2e — assert the trajectory STRUCTURE
  (`li.trajectory`), never figures (the demo ledger is shared + append-only).

## Resolution (shipped)

Pure-reader convergence landed, no schema change. `internal/telemetry` (`bucketforecast.go`):
`DailyTotalsBy` buckets the per-day series by `keyOf` (skipping the empty key when `includeEmpty`
is false, like `shareBy`); `BucketForecasts` runs the **existing `Forecast` unchanged** over each
bucket's own `DailyTotals`, returning `[]BucketProjection{Key, Credits, Projection}` sorted by spend
descending then key ascending — so a high-spend bucket projects a steeper rate than a quiet one, a
single-day bucket clamps to one observed day (not a mostly-absent week), and a window-empty bucket is
`ProjectionIdle`. The account-wide `DaysToCap`/`ExhaustionDate` are intentionally **not** surfaced per
bucket (a bucket has no own allowance). `internal/web` (`pages.go`): `spendShares(now)` joins a
per-bucket trajectory sentence (`bucketTrajectoryText` → *"at ~X cr/day, on pace for ~Y cr this
month"*, the Idle sentence, or empty for no-budget) onto each agent/workflow share row keyed by raw
id under `forgeMu`, threading one `now` per render into both the per-bucket `Forecast` and the month
projection (`daysLeftInMonth`); the `telemetryPage` template renders a `li.trajectory` cell when
present. Tests: unit (`TestBucketForecastsSplitsAndRunsSlopePerBucket`,
`TestBucketForecastsClampsSingleDayBucket`, `TestBucketForecastsIdleBucket`,
`TestBucketForecastsNoBudget`, `TestBucketForecastsEmptyLedger`,
`TestBucketForecastsWorkflowExcludesEmpty`, `TestBucketForecastsDeterministic`); web
(`TestTelemetryPageShowsBucketTrajectory`, `TestTelemetryBucketTrajectoryIdle`,
`TestTelemetryBucketTrajectoryNoBudget`, `TestTelemetryBucketTrajectoryNoStoreNoPanic`); e2e (the
Telemetry spec asserts `li.trajectory` is visible — structure only). Docs: CONTRACTS §4 (the
bucketed-forecast readers + the per-bucket framing) + §3 (the Telemetry-page trajectory surface).
No REGRESSIONS entry: the per-bucket denominator clamp and the single-`now` threading were guarded
preemptively by the unit/web tests (reusing `Forecast` unchanged meant the ADR-0019 gotchas stayed
single-sourced); no real bug was found-and-fixed. Shipped on branch
`claude/bucketed-burn-forecast`.

## Notes
- **No ADR:** a pure-reader composition over the existing ledger — the cost-prediction rationale of
  ADR-0019 (account-wide `Forecast`) ⋈ the attribution rationale of ADR-0018 (per-agent/workflow
  spend tags). A bucketed projection over existing records: no schema change, no new IO. Pre-blessed
  by those two ADRs (per ADR-0004 an ADR leads only a *non-obvious* decision).
- **Differentiator:** cost prediction ⋈ cost attribution — where A3 and A2 compound; a pure-reader
  follow-on, small + compounding.
- **Numbering:** issue **0026** (next free after 0025), second build of epic **0024** (roadmap v4).
  No ADR consumed.
