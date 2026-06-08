---
id: 0050
title: "Epic: Billing fidelity — price every token type GitHub bills, with an authoritative-cost-first source hierarchy (roadmap v10)"
status: open
severity: high
group:
github:
links:
  adr: []
  prs: []
  issues: []
  regression: [3]
---

## Charter

A real-world audit (2026-06-08) found the meter has **drifted from GitHub Copilot's June-2026
usage-based token billing**, under-counting real spend in the product's *core* cost-awareness
differentiator. Two concrete findings:

1. **Two billed token types are unpriced.** Copilot now prices input / output / cached tokens per
   model rate → **AI Credits** (1 cr = $0.01 — our model exactly), **plus** (a) a **cache-write**
   cost (Anthropic ≈ **1.25× input**) on top of cached-input, and (b) **reasoning/thinking** tokens
   billed at the **output** rate. We model neither — both sit in display-only `ExtraTokens`
   (REGRESSIONS #3). So `Meter`/`PriceBook` systematically under-count.
2. **The per-model breakdown reads empty next to real spend.** The Telemetry page has three
   deliberately-separate sources (persisted ledger / month-to-date / **live in-process meter**). The
   demo seeds the ledger but never replays turns through the live meter, so the token table reads
   `0/0/0` while "11.20 cr" shows above it. The **ledger records already carry the token counts**, so
   the table can be computed from history (populated, restart-surviving) instead of the live meter —
   and no integration test currently asserts that table is populated.

### The consistency spine (how we stay correct without re-implementing GitHub's billing)

A **three-tier source hierarchy** — confirmed against the live billing surfaces:

1. **Per-turn truth = the SDK's `ReportedAIU`** (GitHub's authoritative cost, already captured). The
   static price book is demoted to an **estimate** (pre-flight composer + forecast) and the offline
   fallback — never the source of truth for *actual* spend.
2. **Rate freshness (optional) =** poll `https://models.github.ai/catalog/models` for current
   per-model multipliers, **cached + fail-open** to `DefaultPriceBook` (keeps the offline-single-binary
   doctrine — the fetch is opt-in, cached, degradable; no CDN dependency).
3. **Reconciliation (optional) =** pull `/rest/billing/usage` (apiVersion 2026-03-10; `usageItems`
   with `model, unitType, pricePerUnit, quantity`) to reconcile our ledger against GitHub's billed
   records — the cost cousin of the V15/V16 ledger⋈runs reconciliation.

## Children

- [ ] **P0 · Authoritative-cost-first metering** (M; ADR). `ReportedAIU` is the truth for actual turn
      spend when present; price book = estimate/fallback. Surface estimate-vs-reported. Re-frames
      `Meter`/`SpendRecord` around "estimated vs reported."
- [ ] **P1 · Price cache-write + reasoning tokens** (L; ADR — money math + price-book migration).
      Add `CacheWritePerMTok` + reasoning pricing to `ModelRate`; promote the counts out of
      display-only `ExtraTokens` into priced `Usage`; surface in the statusline split + per-model
      breakdown. **Default: cache-write = 1.25× input, reasoning = output rate**, overridable
      per-model via the Settings price editor (G1). Table-tested; deterministic price book.
- [ ] **P2 · Per-model breakdown from the ledger** (M; no ADR). Compute the per-model token table
      (in / cached / cache-write / out / reasoning + credits/usd) from the persisted ledger; relabel
      live ("this session") vs ledger ("all-time"). Closes the empty-table finding; adds the missing
      integration coverage.
- [ ] **P3 · Estimate-vs-reported reconciliation + drift** (M; no ADR). Telemetry row joining computed
      credits to `ReportedAIU` (optionally `/billing/usage`), ambered past an epsilon.
- [ ] **P4 · Live price-book refresh (optional, opt-in)** (L; ADR — network in an offline-first tool).
      Fetch per-model multipliers from `catalog/models` on a cadence, cached to disk, fail-open to the
      static book. Spike the payload shape + network policy first. Strictly additive.

## Acceptance (epic)

- [ ] `ReportedAIU` is the actual-spend source of truth; the price book is explicitly the estimate.
- [ ] Cache-write and reasoning tokens are **priced** (not display-only), with the confirmed defaults,
      overridable, and table-tested; the price book stays deterministic and migrates cleanly.
- [ ] The per-model breakdown is populated from history and guarded by an integration test.
- [ ] Estimate-vs-reported drift is visible on the Telemetry page.
- [ ] Any live fetch is opt-in, cached, and fail-open — the binary still runs fully offline.
- [ ] Each child: failing test first, ADR where it changes money math / a decision, `make lint &&
      make test` (floor 65%) + `make e2e` green, born in its PR, SemVer minor on the epic (`v0.3.0`).

## Sequencing

P0 → P1 → P2 → **(dedicated quality audit)** → P3 / P4, re-ranked on each merge.

## Notes

Research log + sources (Copilot usage-based billing 2026-06-01; cache-write & reasoning pricing;
`catalog/models` + `/billing/usage` endpoints) are recorded in NEXT_FEATURES.md "Roadmap v10". The
deferred **B — cost-anomaly reader** pairs naturally with P3 (both make the cost surface *active*).
