---
id: 0001
title: "Epic: make cost active (Tier 1)"
status: closed
severity: high
group:
github:
links:
  adr:
  prs: []
  issues: [0002, 0003, 0004]
  regression:
assets: []
---

## Charter

The meter today only **observes** spend; `telemetry.Budget.Remaining/FractionUsed`
are pure reads and nothing enforces, warns, or projects. The product leads with
"a coding session never surprises you on the bill" — this epic turns cost from a
passive gauge into an active guardrail-and-ledger. Source: `docs/NEXT_FEATURES.md`
Tier 1.

## Tasks

- [x] **1.2 — Pre-flight turn cost estimate** → [0002](0002-pre-flight-turn-cost-estimate.md)
      (ADR-0007). Project the next turn's cost at the current context in the composer.
- [x] **1.1 — Budget guardrails (soft warn + hard cap)** → [0003](0003-budget-guardrails.md)
      (ADR-0008). Amber statusline + cost footer at a soft threshold; an optional hard
      cap that pauses the next turn (`/budget/{action}`) when `total + EstimateTurn`
      would breach it.
- [x] **1.3 — Persisted spend history + trends** → [0004](0004-persisted-spend-history.md)
      (ADR-0009). Append-only per-session/per-day ledger (`<configDir>/spend.json`,
      atomic write) + a Spend-history trend view (spend over time, per-model share,
      CSV export) on the Telemetry page.

## Status

**Tier 1 complete** — cost is now active end to end: projected before a turn (1.2),
guarded by a soft warn + hard cap (1.1), and persisted as an accountable ledger with
trends (1.3). Follow-ups carried forward: reconcile the budget/month-to-date rows
against the ledger and per-session totals (TECH_DEBT #8/#9, item 3.2).

## Notes

Recommended sequencing (NEXT_FEATURES): 1.2 → 1.1 → 1.3. Every piece reuses existing
primitives (PriceBook, the live meter, the inline-approval UX). Keep domain logic pure.
