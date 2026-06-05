---
id: 0008
title: Per-session telemetry totals (Tier 3, item 3.2)
status: closed
severity: medium
group: 0007
github:
links:
  adr: ../adr/0011-per-session-telemetry-meter-for-the-statusline.md
  prs: []
  issues: [0007]
  regression:
assets: []
---

## Summary

The chat **statusline** read the process-global `telemetry.Meter` that `Hub.New`
shares across every cookie-keyed session, so its token split, cache-hit %,
credits, and pre-flight estimate showed **every** conversation's combined spend
instead of *this* one (TECH_DEBT #2). Scope the statusline to the current
session. Source: `docs/NEXT_FEATURES.md` item 3.2. Pairs with 1.3 — the persisted
ledger already tags `SessionID`.

## Repro
1. Open two browser sessions (distinct cookies) against the same app.
2. Spend credits in session A.
3. Expected: session B's statusline shows *its own* (zero/low) totals.
4. Actual (before): both statuslines showed the shared account-wide total.

## Resolution

- New `Server.sessionMeter`: a per-conversation `telemetry.Meter` built in
  `newSession` on the **account-wide meter's price book** (via the new pure
  `telemetry.Meter.PriceBook()` accessor), so per-session credits/estimates stay
  penny-consistent with the global gauge.
- The `EvUsage` reducer records each turn into **both** meters; `renderStatline`
  reads `sessionMeter` for tokens, cache-hit, credits, the estimate, and the
  soft-warn tint.
- The **topbar cost footer** (ambient budget banner), the **hard-cap projection**
  (`overCap`), and the **Telemetry page** month-to-date / per-model rows stay on
  the account-wide `s.meter` — budget enforcement and accounting must be
  cumulative. The per-session soft-warn under-warns when spend is spread across
  conversations; the topbar gauge remains the cumulative signal.
- Design recorded in **ADR-0011** (scoped sibling meter vs ledger-derived vs
  keep-global; what stays account-wide; where the soft-warn lives).

## Notes

Guarding tests: `internal/telemetry` `TestMeterPriceBookExposed`; `internal/web`
`TestStatlineScopesTotalsToThisSession`, `TestTelemetryPageStaysAccountWide`
(plus the existing `budget_test.go` amber/footer tests, whose `recordSpend` now
folds into both meters). No on-disk schema, route, or statusline-field change —
only the data source behind the existing statusline fields, so CONTRACTS is
unchanged. Gotcha (two-meter split) logged in REGRESSIONS; TECH_DEBT #2 paid,
#9 (ledger-derived budget rows) revisited as the remaining pairing step. Closes
the first Tier-3 item of epic 0007.
