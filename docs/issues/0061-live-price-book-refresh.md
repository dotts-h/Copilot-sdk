---
id: 0061
title: "Live price-book refresh — opt-in, cached, fail-open fetch of per-model multipliers (roadmap v10, P4)"
status: open
severity: low
group: 0050
depends_on: [0059]
github:
links:
  adr: []          # reserves ADR-0035 (network in an offline-first tool)
  prs: []
  issues: [0050]
  regression:
assets: []
---

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
