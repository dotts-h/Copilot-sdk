---
id: 0003
title: Budget guardrails — soft warn + hard cap (Tier 1, item 1.1)
status: closed
severity: high
group: 0001
github:
links:
  adr: ../adr/0008-budget-guardrails-soft-warn-and-hard-cap-gate.md
  prs: []
  issues: [0001]
  regression:
assets: []
---

## Summary

The meter observed spend and (item 1.2) projected the next turn, but nothing
intervened. Turn cost from a gauge into a guardrail: a **soft warn** when spend
crosses a threshold of the allowance, and an optional **hard cap** that pauses a
turn whose projected spend would breach it. Source: `docs/NEXT_FEATURES.md` item 1.1.

## Repro
1. Set a monthly allowance and a warn % (Settings) and spend past the threshold.
2. Expected: an ambient amber indicator (cost footer + statusline) once over the
   soft threshold; with a hard cap set, an over-cap turn pauses for confirmation.
3. Actual (before): the meter only displayed spend — no warning, no pause.

## Resolution

- `telemetry.Budget` gains `WarnFraction` + `HardCapCredits` and two pure, total
  predicates — `Warned(used)` and `CapExceeded(projected)` (strict `>`, zero
  disables). `config.TelemetryConfig.HardCapCredits` persists the cap (atomic,
  validated `>= 0`).
- **Soft warn:** the topbar cost footer and the statusline cost item turn amber
  via `Budget.Warned` — the topbar is the ambient banner.
- **Hard cap:** `handleSend` projects `total + EstimateTurn(model, liveContext)`
  (reusing ADR-0007) and, if it breaches the cap, holds the turn in a `budgetGate`
  and renders an inline form. `POST /budget/{action}` resolves it — **proceed**
  (keep cap), **raise** (lift + persist), **cancel** (drop). A settings save
  refreshes the live session's budget so the gate takes effect immediately.
- Design recorded in **ADR-0008** (why a dedicated app-level route, why price the
  projection from ADR-0007).

## Notes

Guarding tests: `internal/telemetry` `TestBudgetWarned`, `TestBudgetCapExceeded`;
`internal/config` `TestValidateRejectsBadValues` (hard cap `< 0`), `TestSaveLoadRoundTrip`
(hard cap persists); `internal/web` `TestStatlineTurnsAmberOverSoftThreshold`,
`TestCostFooterWarnsOverSoftThreshold`, `TestHardCapPausesOverBudgetTurn`,
`TestHardCapProceedDispatchesHeldTurn`, `TestHardCapRaiseLiftsCapAndDispatches`,
`TestHardCapCancelDropsTurn`, `TestUnderCapDispatchesNormally`,
`TestSettingsSavePersistsAndApplies` (hard cap field + live refresh); browser:
`e2e/tests/e2e.spec.ts` "a turn over the cap pauses inline, then proceeds on
confirmation". Follow-ups tracked in epic 0001 (1.3 can tighten the cap with a
persisted cache-hit ratio).
