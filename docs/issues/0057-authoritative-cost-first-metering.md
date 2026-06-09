---
id: 0057
title: "Authoritative-cost-first metering — ReportedAIU is actual spend, price book is the estimate (roadmap v10, P0)"
status: open
severity: high
group: 0050
depends_on: []
github:
links:
  adr: [33]        # ADR-0033 (the framing decision, written first per ADR-0004)
  prs: []
  issues: [0050]
  regression: [3]
assets: []
---

> **Status (2026-06-09, in progress on `feat/billing-authoritative-cost-first`).**
> Shipped the authoritative-cost-first framing: `telemetry.ActualCredits`/`HasReported`
> (the table-tested estimate-vs-reported selection), `Meter.ActualCredits`/`HasReported`,
> `SpendRecord.EstimateCredits`/`ActualCredits`/`HasReported`, and the pure
> `MonthToDateActual` aggregate; the reported AIU is now folded into the per-session
> meter too. The statusline cost cell and the topbar cost footer
> (`renderActualCostFooter`) surface the actual figure tagged reported / est / mixed,
> with the estimate shown beside the reported figure when they diverge. The price book
> stays the deterministic estimate (pre-flight + forecast + offline fallback). Decision
> recorded in [ADR-0033](../adr/0033-reportedaiu-is-source-of-truth-for-actual-spend-price-book-is-the-estimate.md).
> Out of scope (sibling lanes): the per-model breakdown table (0058) and the
> estimate-vs-reported drift row (0060). `make lint && make test` green; coverage 88.4%.

## Summary
Re-frame the meter around a **three-tier source hierarchy**: the SDK's `ReportedAIU` is the truth for
*actual* per-turn spend when present; the static price book is demoted to an **estimate** (pre-flight
composer + forecast) and the offline fallback. Today the price book is treated as the source of truth
for real spend, which is what lets the meter drift from GitHub's billing (epic 0050, finding 1).

## Scope / Touches
- `internal/telemetry` — `Meter`/`SpendRecord` carry **estimated vs. reported** credits; `recordUsage`
  prefers `ReportedAIU` for actual, keeps the price-book figure as the estimate.
- `internal/copilot` — ensure `ReportedAIU` is plumbed through the normalized event (already captured).
- `internal/web` — surface estimate-vs-reported where spend is shown (statusline/footer).
- **ADR (reserved 0033):** "ReportedAIU is the source of truth for actual spend; price book is estimate."

## Dependencies
- `depends_on: []` — this is the **foundation** of the billing-fidelity epic; P1/P3 build on the
  estimated-vs-reported framing it introduces.

## Acceptance
- `ReportedAIU` is the actual-spend source of truth; the price book is explicitly the estimate/fallback.
- Estimate-vs-reported is visible; the price book stays deterministic.
- Failing test first; `make lint && make test` (floor 65%) + `make e2e` green; born in its PR; ADR-0033.

## Notes
First in the v10 sequence (P0 → P1 → P2 → audit → P3/P4). Re-frames the seam the rest of the epic
extends — see [epic 0050](0050-epic-billing-fidelity.md) and NEXT_FEATURES "Roadmap v10".
