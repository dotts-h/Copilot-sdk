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

Filed 2026-06-09 with `depends_on` edges (graph below). P2 is split: **P2-core** (empty-table fix +
the missing integration test) is independent of P0/P1 and buildable now; P1 later extends that same
table with the new priced columns (hence P1 `depends_on` P2-core — shared seam).

- [ ] **P0 · Authoritative-cost-first metering** — [0057](0057-authoritative-cost-first-metering.md)
      (M; ADR-0033). `depends_on: []`. Re-frames `Meter`/`SpendRecord` around "estimated vs reported";
      `ReportedAIU` = actual, price book = estimate/fallback.
- [ ] **P2-core · Per-model breakdown from the ledger + missing integration test** —
      [0058](0058-per-model-breakdown-from-ledger.md) (M; no ADR). `depends_on: []`. Computes the table
      from the persisted ledger and adds the integration test that currently doesn't exist. **Buildable
      now** — a first parallel lane.
- [ ] **P1 · Price cache-write + reasoning tokens** —
      [0059](0059-price-cache-write-and-reasoning-tokens.md) (L; ADR-0034). `depends_on: [0057, 0058]`.
      Promotes cache-write (1.25× input) + reasoning (output rate) out of display-only `ExtraTokens`
      into priced `Usage`; extends the per-model breakdown columns.
- [ ] **P3 · Estimate-vs-reported reconciliation + drift** —
      [0060](0060-estimate-vs-reported-reconciliation-drift.md) (M; no ADR). `depends_on: [0057]`.
      Telemetry row joining computed credits to `ReportedAIU`, ambered past an epsilon. Parallel lane
      after 0057.
- [ ] **P4 · Live price-book refresh (optional, opt-in)** —
      [0061](0061-live-price-book-refresh.md) (L; ADR-0035). `depends_on: [0059]`. Opt-in, cached,
      fail-open fetch of per-model multipliers; spike payload + network policy first.

Dependency graph:
```
0057 (P0) ─┬─► 0059 (P1) ──► 0061 (P4)
           └─► 0060 (P3)
0058 (P2-core) ──► 0059 (P1)
```
Unblocked now (parallel lanes if seams are disjoint): **0057, 0058**.

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
