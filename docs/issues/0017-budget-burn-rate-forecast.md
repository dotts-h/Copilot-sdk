---
id: 0017
title: Budget burn-rate projection / forecast (roadmap v2, item A3)
status: closed
severity: medium
group: 0013
github:
links:
  adr: ../adr/0019-budget-burn-rate-forecast-trailing-window-average.md
  prs: []
  issues: [0013, 0014, 0016]
  regression:
assets: []
---

## Summary

Cost was *active* but entirely **reactive**: the soft warn (80%) and the hard cap
(ADR-0008) only fire once you are already near the line. The data to be
**predictive** already existed — `DailyTotals` (ADR-0009) gives a per-day spend
slope and A1/ADR-0016 put month-to-date on the same persisted series — so the
project can answer the question the guardrails can't: *"at this rate, when do I
run out?"* Add a pure `telemetry.Forecast` over `DailyTotals` + the `Budget`
allowance that projects days/turns-to-cap and an exhaustion date, surfaced on the
Telemetry page and (compact) in the statusline. Source: `docs/NEXT_FEATURES.md`
item A3; ADR-0019; builds on A1 (issue 0014) and A2 (issue 0016).

## Repro
1. Spend steadily for a few days against a configured monthly allowance.
2. Open the Telemetry page (or watch the statusline).
   - **Expected:** a projection — "at ~X cr/day, your N cr budget runs out around
     &lt;date&gt; (~M turns left)" — and a compact `cap ~Nd` statusline cell that
     ambers when on track to exceed the budget before month-end.
   - **Actual (before):** nothing predictive; the only signals were the after-the-
     fact warn and the per-turn cap. No "you'll blow the budget by the 20th."

## Resolution (shipped)

Built on `claude/next-features-research-8aBvS`:

- **Telemetry (`internal/telemetry/forecast.go`, pure):** `Forecast(daily
  []DayTotal, budget Budget, now time.Time) Projection`. The burn rate is a
  **trailing-7-day average** (sum the window's credits ÷ the elapsed days actually
  observed — the window length, clamped to ledger age so a new/single-day ledger
  isn't divided by a mostly-absent week, while idle days inside an established
  history drag the average down). "Used" is month-to-date over `now`'s UTC month
  (consistent with `MonthToDate`). `Projection.Status` makes every degenerate case
  explicit: `ProjectionNoBudget` (no allowance), `ProjectionIdle` (no recent
  spend), `ProjectionExhausted` (already at/over allowance), `ProjectionOK`
  (finite days/turns/date). No IO, no schema change.
- **Web (`internal/web`):** `Server.forecast(now)` reads it account-wide off the
  ledger like the other ADR-0016 rows; `telemetryPartial` renders a forecast
  sentence (`forecastView`, ambered when the exhaustion date falls within the
  month via `forecastSoon`), and `renderStatline` adds a compact `cap ~Nd` cell
  (`statlineForecast`), shown only for a finite `ProjectionOK` projection. A
  single `now` is threaded per render so the date and the warn never disagree;
  the displayed day count uses `⌈DaysToCap⌉` to match the exhaustion date.

Guarded by `internal/telemetry` `TestForecast` (table: no-budget / idle /
older-than-window / exhausted / flat-week / single-day / window-vs-month) and
`TestForecastDeterministic`; `internal/web` `TestTelemetryPageShowsForecast` /
`TestTelemetryForecastNoBudgetHint` / `TestStatlineShowsForecastCell` /
`TestStatlineNoForecastCellWithoutBudget`; e2e asserts the "Forecast" label on the
Telemetry page (structure, not figures — shared demo ledger).

## Notes

- **Decision:** ADR-0019 (trailing-window average over linear regression;
  degenerate-case behavior) — lead-with-a-decision (ADR-0004).
- **Watch:** the shared-demo-ledger gotcha — the e2e asserts the *label*, never
  figures (the append-only demo ledger grows as the suite runs).
- **Deferred (consistent with ADR-0016):** the forecast read is O(n) on the
  render path / statusline refresh, like `MonthToDate`; caching the projection is
  noted, not built.
- **Closes** Tier A's cost-accountability loop (A1 → A2 → A3).
