# 0019. Budget burn-rate forecast: a trailing-window average over the daily ledger

- Status: accepted
- Date: 2026-06-06
- Deciders: Horia
- Related: `internal/telemetry` (`Forecast`/`Projection` over `DailyTotals` +
  `Budget`), `internal/web` (`pages.go` `telemetryPartial`, `render.go`
  `renderStatline`, `server.go` `forecast()`), `docs/NEXT_FEATURES.md` item A3,
  [ADR-0008](0008-budget-guardrails-soft-warn-and-hard-cap-gate.md) (the
  reactive guardrails this builds on),
  [ADR-0009](0009-persisted-spend-history-append-only-ledger.md) (the ledger /
  `DailyTotals` slope source),
  [ADR-0016](0016-ledger-is-source-of-truth-for-account-wide-budget-accounting.md)
  (the account-wide month-to-date read this projects forward),
  issue [0017](../issues/0017-budget-burn-rate-forecast.md)

> **Shipped (A3 / issue 0017).** Written first as the architectural pick from the
> 2026-06-06 next-features research pass (ADR-0004 lead-with-a-decision), then
> built: `telemetry.Forecast` plus a Telemetry-page forecast line and a compact
> statusline cell. Guarded by `internal/telemetry` `TestForecast`/
> `TestForecastDeterministic` and `internal/web` `TestTelemetryPageShowsForecast`/
> `TestStatlineShowsForecastCell`.

## Context

Cost is *active* end to end — a pre-flight estimate (ADR-0007), a soft warn at a
threshold and a hard-cap turn gate (ADR-0008), a persisted append-only ledger
with trends (ADR-0009), and an account-wide month-to-date read off that ledger
(ADR-0016). But every signal is **reactive**: the warn and the cap only fire
*once you are already near the line*. The data to be **predictive** already
exists — `DailyTotals` (ADR-0009) gives a per-day spend slope and ADR-0016 put
month-to-date on the same persisted series — so the project can answer the
question the guardrails can't: *"at this rate, when do I run out?"* (item A3).

This is a pure reader: no new IO, no schema change, a function beside
`MonthToDate` / `DailyTotals` over the existing v1 records. Three questions had
to be answered: *how to estimate the slope*, *what to project toward*, and *what
to show in the degenerate cases* (no allowance, no recent spend, history too
short, already over budget).

## Considered options

- **Slope: trailing-N-day average vs. linear regression.** Choose the **trailing
  7-day average** (sum the last 7 UTC days' credits ÷ the days actually
  observed). Rejected linear regression: on a sparse, noisy single-user series it
  overfits, and a recent slow-down yields a *negative* slope — a nonsensical
  "exhausts in the past / never" that's worse than useless. A trailing average is
  the conventional burn-rate estimator, matches the simple-aggregation style of
  the rest of the package (`DailyTotals`/`ModelShares`), is trivially
  deterministic, and reads plainly in the UI ("at your recent ~X cr/day"). A week
  smooths weekday/weekend bumps while still tracking a genuine change in pace.
- **Denominator for the average.** Divide the window's spend by the **elapsed
  observed days** = the window length (7), *clamped to how long the ledger has
  actually existed*. So a brand-new or single-day ledger is **not** divided by a
  mostly-absent week (which would understate the rate to near-zero), while idle
  days *inside* an established history **do** correctly drag the average down
  (a quiet week lowers the projected burn). Rejected dividing by the count of
  days that have records (ignores genuine zero-spend days, overstating the rate).
- **What to project toward.** Project to the **monthly allowance**
  (`Budget.AllowanceCredits`), the figure the burn-rate question is about ("when
  do I use up this month's budget"). The hard cap (ADR-0008) is a *per-turn*
  guardrail, a different concept, so it is deliberately not the forecast target.
  "Used" is the **month-to-date** sum of the current UTC month (consistent with
  `MonthToDate` / ADR-0016), independent of the 7-day rate window.
- **Degenerate cases.** Make them **explicit in a `Status` enum** rather than
  sentinel numbers the UI must reverse-engineer: `NoBudget` (no allowance set →
  nothing to project toward), `Idle` (no spend in the window → never reached at
  the current rate), `Exhausted` (month-to-date ≥ allowance → already gone),
  `OK` (a positive rate and remaining allowance → a finite days/turns/date
  projection). A single-day series is **not** a degenerate case — it projects
  from the one day observed (robustness requirement of A3).

## Decision

Add a pure `telemetry.Forecast(daily []DayTotal, budget Budget, now time.Time)
Projection` beside `MonthToDate` / `DailyTotals` — no IO, no clock beyond the
`now` passed in, same totality guarantees as the rest of the package. It:

1. returns `Status: ProjectionNoBudget` when no allowance is configured;
2. sums month-to-date "used" over `now`'s UTC month and, if that already meets
   the allowance, returns `ProjectionExhausted`;
3. computes the **trailing-7-day** credit total ÷ elapsed observed days (window
   length clamped to ledger age) as `DailyRate`, and ÷ the window's turn count as
   `PerTurnRate`; a zero rate returns `ProjectionIdle`;
4. otherwise returns `ProjectionOK` with `DaysToCap = remaining / DailyRate`,
   `TurnsToCap = remaining / PerTurnRate`, and `ExhaustionDate = now's UTC day +
   ⌈DaysToCap⌉`.

The web layer reads it account-wide off the ledger (like the other ADR-0016
rows): the **Telemetry page** shows a forecast line ("at ~X cr/day you'll reach
your N cr budget around <date> — ~M turns left", or the right degenerate
message), and the **statusline** shows a compact cell (`cap ~Nd`) that turns
amber when the projected exhaustion falls on or before the end of the current
UTC month (you're on track to blow this month's budget). No new normalized event
and **no persisted-schema change** — `Forecast` is a new pure reader over the
existing v1 `records` (CONTRACTS §4), so there is no migration.

## Consequences

- Positive: cost moves from **reactive to predictive** — the user sees the bill
  coming, not just the line once crossed. Pure and fully unit-tested; reuses the
  ledger queries ADR-0009/0016 established; degenerate cases are total and
  explicit. The window is a constant, so a configurable horizon is an additive
  tweak later.
- Trade-off we accept: a trailing average **lags a step change** in pace (a
  sudden spike takes a few days to fully move the projection) and is **only as
  good as a short, noisy single-user series** — so the UI frames it as "~" and a
  date, never a precise promise, exactly like the ADR-0007 estimate. It is a
  heads-up, not an accountant.
- Trade-off: the forecast read joins the Telemetry render path and the statusline
  refresh — both O(n) over the ledger, the same bounded single-user volume as
  ADR-0016's month-to-date read. If volume ever grows, cache alongside the
  month total (noted, not built).
- Trade-off: a UTC calendar month (inherited from ADR-0016) won't match a user
  whose plan resets mid-month; the same pure-function window makes a
  billing-anchor day an additive follow-up for both surfaces at once.
- Follow-ups: a configurable forecast horizon / billing-anchor day; folding the
  observed cache-hit ratio into the per-turn rate once it's persisted.
