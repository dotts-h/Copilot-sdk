---
id: 0060
title: "Estimate-vs-reported reconciliation + drift on the Telemetry page (roadmap v10, P3)"
status: closed
severity: medium
group: 0050
depends_on: [0057]
github:
links:
  adr: []
  prs: [103]
  issues: [0050]
  regression:
assets: []
---

> **Shipped 2026-06-10 — PR #103, part of epic 0050.** `telemetry.ModelDrifts` (new
> `drift.go`) joins the price-book estimate (via the named `EstimateCredits` seam) to
> GitHub's reported AIU per model over the ledger's **reported turns only** — an
> unreported turn has no authoritative figure to drift from, so it is counted as
> est-only coverage, never compared. The Telemetry page shows the join as an
> "Estimate vs reported" table (estimate · reported · delta · coverage), the delta
> ambered past the shared `reconcileEpsilon` — the cost cousin of the V15/V16
> ledger⋈runs reconciliation, within one store. The demo seeds two reported turns
> (one drifted) so the table renders offline; *drift* is defined in CONTEXT.md.
> The optional `/rest/billing/usage` pull was left out (deferred with P4's posture).

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
