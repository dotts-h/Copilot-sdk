---
id: 0001
title: "Epic: make cost active (Tier 1)"
status: in-progress
severity: high
group:
github:
links:
  adr:
  prs: []
  issues: [0002]
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
- [ ] **1.1 — Budget guardrails (soft warn + hard cap).** Amber statusline + ambient
      banner at a soft threshold; optional hard cap that pauses the next turn via the
      permission-form pattern. Can reuse `EstimateTurn` to project an over-budget turn.
- [ ] **1.3 — Persisted spend history + trends.** Append-only per-session/per-day
      ledger (atomic write) + a trend view on the Telemetry page.

## Notes

Recommended sequencing (NEXT_FEATURES): 1.2 → 1.1 → 1.3. Every piece reuses existing
primitives (PriceBook, the live meter, the inline-approval UX). Keep domain logic pure.
