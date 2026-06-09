---
id: 0060
title: "Estimate-vs-reported reconciliation + drift on the Telemetry page (roadmap v10, P3)"
status: open
severity: medium
group: 0050
depends_on: [0057]
github:
links:
  adr: []
  prs: []
  issues: [0050]
  regression:
assets: []
---

## Summary
A Telemetry row joining our **computed** credits (price-book estimate) to the SDK's **`ReportedAIU`**
(and, optionally, GitHub's `/rest/billing/usage` records), **ambered** when drift exceeds an epsilon —
the cost cousin of the V15/V16 ledger⋈runs reconciliation. Makes the estimate-vs-reported gap visible
instead of silent.

## Scope / Touches
- `internal/telemetry` — a pure reader joining estimated vs. reported per window/model; an epsilon
  drift flag.
- `internal/web` — a reconciliation row on the Telemetry page (pattern of the existing ledger⋈runs
  reconciliation, issues 0039–0041).
- Optional: `/rest/billing/usage` (apiVersion 2026-03-10) pull, behind the same opt-in/fail-open
  posture as P4.

## Dependencies
- `depends_on: [0057]` — needs the estimated-vs-reported framing P0 introduces; independent of the
  pricing work (0059), so it can run as a **parallel lane after 0057 lands** (disjoint seam: a new
  reconciliation reader + row, not the price book).

## Acceptance
- Estimate-vs-reported drift is visible on the Telemetry page, ambered past epsilon.
- Pure reader; failing test first; `make lint && make test` + `make e2e` green; born in its PR. No ADR.

## Notes
Pairs naturally with the deferred **B — cost-anomaly reader** (both make the cost surface *active*).
See [epic 0050](0050-epic-billing-fidelity.md) and the reconciliation epic
[0038](0038-epic-cost-run-reconciliation.md).
