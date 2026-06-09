---
id: 0057
title: "Authoritative-cost-first metering — ReportedAIU is actual spend, price book is the estimate (roadmap v10, P0)"
status: open
severity: high
group: 0050
depends_on: []
github:
links:
  adr: []          # reserves ADR-0033 (written first when built)
  prs: []
  issues: [0050]
  regression: [3]
assets: []
---

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
