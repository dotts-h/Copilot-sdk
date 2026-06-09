---
id: 0059
title: "Price cache-write + reasoning tokens — promote out of display-only ExtraTokens into priced Usage (roadmap v10, P1)"
status: closed
severity: high
group: 0050
depends_on: [0057, 0058]
github:
links:
  adr: [0034]
  prs: [98]
  issues: [0050]
  regression: [3]
assets: []
---

> **Shipped 2026-06-09 — PR #98, part of epic 0050.** Cache-write tokens are now a
> first-class priced category (`ModelRate.CacheWritePerMTok`, default 1.25× input;
> `Cost.CacheWriteUSD` folded into `USD()`); reasoning tokens are recognised as a
> **subset of `OutputTokens`** (SDK: "output tokens used for reasoning") — already
> priced at the output rate, so they stay a display-only count and are **never charged
> twice** (`TestPriceDoesNotDoubleCountReasoning`). `SpendRecord` gained additive
> `cw`/`reasoning` fields (schema v3, backward-readable); `config.PriceOverrides`
> migrated `[3]float64`→`[]float64` (length 3 or 4) so a legacy override loads unchanged
> and a 4-element override pins cache-write. The Settings price editor and the all-time
> per-model table surface the cache-write column. Decision recorded in
> [ADR-0034](../adr/0034-price-cache-write-additive-reasoning-is-output-subset.md). The
> reasoning-as-subset call was confirmed with the maintainer first. `make lint && make
> test` green under `-race`; `make e2e` green.

## Summary
Two billed token types are unpriced and sit in display-only `ExtraTokens` (REGRESSIONS #3): a
**cache-write** cost (Anthropic ≈ **1.25× input**) on top of cached-input, and **reasoning/thinking**
tokens billed at the **output** rate. Add them to `ModelRate`, promote the counts into priced
`Usage`, and surface them in the statusline split + the per-model breakdown.

## Scope / Touches
- `internal/telemetry` — `ModelRate` gains `CacheWritePerMTok` + reasoning pricing; `PriceBook` math;
  **deterministic** book + a backward-readable price-book migration. Default cache-write = 1.25× input,
  reasoning = output rate, overridable per-model via the Settings price editor (G1/issue 0027).
- `internal/web` — statusline token split + the per-model breakdown gain the two columns.
- **ADR (reserved 0034):** the money math + price-book migration decision.

## Dependencies
- `depends_on: [0057]` — builds on the estimated-vs-reported framing (pricing is the *estimate* tier).
- `depends_on: [0058]` — **extends the same per-model breakdown table**; serialize on that shared seam
  rather than racing 0058's render changes.

## Acceptance
- Cache-write + reasoning are **priced** (not display-only), with the confirmed defaults, overridable,
  and **table-tested**; the price book stays deterministic and migrates cleanly.
- Failing test first; `make lint && make test` (floor 65%) + `make e2e` green; born in its PR; ADR-0034.

## Notes
Largest child (money math). See [epic 0050](0050-epic-billing-fidelity.md), the price editor
[0027](0027-settings-price-override-editor.md), and REGRESSIONS #3.
