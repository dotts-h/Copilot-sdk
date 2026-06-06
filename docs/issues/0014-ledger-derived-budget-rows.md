---
id: 0014
title: Ledger-derived budget rows (roadmap v2, item A1)
status: open
severity: high
group: 0013
github:
links:
  adr: ../adr/0016-ledger-is-source-of-truth-for-account-wide-budget-accounting.md
  prs: []
  issues: [0013]
  regression:
assets: []
---

## Summary

The account-wide budget accounting on the Telemetry page ("Total cost / Monthly
budget / Remaining"), the cost footer, and the hard-cap projection all read the
**live, in-process `telemetry.Meter`**, which resets on restart. Make them read
**month-to-date from the persisted `SpendStore`** so "remaining this month"
survives a restart — the last amnesiac gap in the cost differentiator's headline
promise. Source: `docs/NEXT_FEATURES.md` item A1; ADR-0016; promotes TECH_DEBT #9.

## Repro
1. Run a few turns so the meter shows spend; note "Remaining" on the Telemetry page.
2. Restart the app (the `Meter` is in-memory; the ledger on disk is not).
3. Open Telemetry.
   - **Expected:** "Total cost / Monthly budget / Remaining" reflect this month's
     persisted spend; the hard cap still knows how close you were.
   - **Actual:** the rows reset to zero (a fresh `Meter`), even though
     `spend.json` and the trend view below them still show the month's history.
     The gauge and the ledger disagree across restarts.

## Resolution (planned — not yet built)

- **Pure aggregation (`internal/telemetry/history.go`):** add
  `MonthToDate(records []SpendRecord, now time.Time) Cost` beside `DailyTotals` /
  `ModelShares` — UTC calendar-month bucket, dependency-free, same totality
  guarantees, a table-driven unit test (empty, single-month, month boundary,
  prior-month excluded).
- **Read-source swap (`internal/web`):** `telemetryPartial` (`pages.go`),
  `renderCostFooter` (`render.go`), and the hard-cap `overCap` projection
  (`server.go` `budget()` baseline) read month-to-date from the ledger instead of
  `s.meter.Totals()`. The **per-session statusline** (`sessionMeter`, ADR-0011)
  and the live token split stay on the in-process meter — one source per surface
  (see the REGRESSIONS "two meters" gotcha, now three sources: session meter =
  statusline; ledger = account-wide accounting; process meter = live token split).
- **Contract:** additive — no schema change (`MonthToDate` is a new pure reader
  over the existing v1 `records`). Note the read-source shift in CONTRACTS §4 and
  ARCHITECTURE's telemetry section.

## Notes

- **Decision:** ADR-0016 (the persisted ledger is the source of truth for
  account-wide budget accounting) — written first, lead-with-a-decision (ADR-0004).
  It supersedes the "history vs. live meter" split in ADR-0009 for the
  account-wide rows.
- **Guard tests to add:** `internal/telemetry` `TestMonthToDate*`; `internal/web`
  a test that account-wide rows reflect a seeded ledger after a fresh meter (the
  restart proxy) and that the cap baseline reads the ledger — mirror
  `TestTelemetryPageStaysAccountWide`. Watch the shared-demo-ledger gotcha
  (assert structure / relative, not exact figures).
- **Unblocks:** A2 (attribute records to agent/workflow → per-agent month-to-date
  is the same query + filter) and A3 (burn-rate forecast over `DailyTotals`).
