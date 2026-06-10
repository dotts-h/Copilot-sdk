---
id: 0061
title: "Live price-book refresh — opt-in, cached, fail-open fetch of per-model multipliers (roadmap v10, P4)"
status: closed
severity: low
group: 0050
depends_on: [0059]
github:
links:
  adr: [0035]
  prs: []
  issues: [0050]
  regression:
assets: []
---

> **Closed 2026-06-10 — refuted by the mandated spike; decision recorded in
> [ADR-0035](../adr/0035-no-live-price-book-refresh-catalog-has-no-pricing.md). No code.**
> The live fetch of `catalog/models` (HTTP 200, 37 entries) found the payload carries
> **no pricing data**: the only rate-ish field, `rate_limit_tier`, is a request-throttling
> tier, not a billing multiplier — there is nothing for the fetch to deliver. Rate
> freshness is covered without network: actual spend self-heals via `ReportedAIU`
> (ADR-0033), staleness is *visible* in the 0060 drift table, and estimates are
> user-correctable offline via the price-override editor (issue 0027). The
> authenticated `/rest/billing/usage` reconciliation remains a possible future child;
> revisit only if GitHub publishes a machine-readable pricing endpoint.

## Summary
Optionally refresh the per-model rate multipliers from `https://models.github.ai/catalog/models` on a
cadence, **cached to disk** and **fail-open** to `DefaultPriceBook`. Keeps the offline-single-binary
doctrine: the fetch is opt-in, cached, and degradable — no CDN dependency at runtime. Strictly additive.

## Scope / Touches
- `internal/telemetry` (or a small fetch seam) — fetch + parse `catalog/models`, cache to disk, merge
  into the book; fail-open on any error. **Spike the payload shape + network policy first.**
- `internal/config` — an opt-in toggle (default off) + cache path.
- **ADR (reserved 0035):** introducing network egress into an offline-first tool — posture, cadence,
  failure mode.

## Dependencies
- `depends_on: [0059]` — refreshes the **price book that P1 reshaped** (the new cache-write/reasoning
  rate fields must exist before a fetch can populate them). Last in the epic; optional.

## Acceptance
- Any live fetch is **opt-in, cached, and fail-open** — the binary still runs fully offline.
- Spike the payload + network policy before building; failing test first (fetch parse + fail-open);
  `make lint && make test` + `make e2e` green; born in its PR; ADR-0035.

## Notes
Lowest priority; behind the dedicated quality audit in the v10 sequence. See
[epic 0050](0050-epic-billing-fidelity.md) and NEXT_FEATURES "Roadmap v10".
