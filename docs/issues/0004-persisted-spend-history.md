---
id: 0004
title: Persisted spend history + trends (Tier 1, item 1.3)
status: closed
severity: high
group: 0001
github:
links:
  adr: ../adr/0009-persisted-spend-history-append-only-ledger.md
  prs: []
  issues: [0001]
  regression:
assets: []
---

## Summary

The `telemetry.Meter` was in-memory only, so all spend accounting died on
restart — undercutting the "never surprises you on the bill" promise. Persist
per-session/per-day spend in an append-only ledger and add a trend view (spend
over time, per-model share, CSV export) to the Telemetry page. Source:
`docs/NEXT_FEATURES.md` item 1.3. Last Tier-1 item (1.2 → 1.1 → 1.3).

## Repro
1. Spend some credits, then restart the app.
2. Expected: the Telemetry page still shows the prior spend as a trend; spend is
   queryable over time, per model, and exportable as CSV.
3. Actual (before): the meter reset to zero on every restart — no history at all.

## Resolution

- New `telemetry.SpendStore` / `SpendRecord` (`history.go`): a versioned JSON
  ledger at `<configDir>/spend.json`, written **atomically** (temp-file + rename,
  like config), missing = empty, invalid = error, forward-readable via a
  `version` tag. An empty dir → ephemeral in-memory store (demo/tests never touch
  disk). Pure aggregations: `DailyTotals`, `ModelShares`, `WriteCSV`.
- The `EvUsage` reducer appends one record per metered turn (best-effort; a disk
  error is logged, not surfaced). `bootstrap.Build` loads the ledger for the real
  app and seeds a deterministic ephemeral one for demo.
- The Telemetry page gains a **Spend history** section: a 14-day "spend over
  time" bar list, a "per-model share" bar list, and a `GET /telemetry/export.csv`
  download. Existing meter-based summary/per-model rows are unchanged.
- Design recorded in **ADR-0009** (why telemetry-sibling + atomic full-rewrite
  over JSONL; why the trend reads the ledger while the budget rows still read the
  live meter — a noted follow-up).

## Notes

Guarding tests: `internal/telemetry` `TestSpendStoreAppendPersistsAndReloads`,
`TestSpendStoreEphemeralNeverWrites`, `TestLoadSpendStoreRejectsCorruptFile`,
`TestLoadSpendStoreToleratesNewerSchema`, `TestDailyTotals`, `TestModelShares`,
`TestWriteCSV`; `internal/web` `TestUsagePersistsSpendRecord`,
`TestUsageWithoutLedgerDoesNotPanic`, `TestTelemetryPageShowsTrendFromLedger`,
`TestTelemetryPageEmptyHistoryNote`, `TestSpendExportReturnsCSV`;
`internal/bootstrap` `TestSeedSpendPopulatesDeterministicHistory`,
`TestBuildDemoTelemetryShowsTrend`; browser: `e2e/tests/e2e.spec.ts` "the
Telemetry page shows the spend trend and exports CSV". Schema + route registered
in CONTRACTS §3/§4; gotchas in REGRESSIONS; follow-ups (synchronous-write,
month-to-date reconciliation) in TECH_DEBT #8/#9. Closes Tier-1 of epic 0001.
