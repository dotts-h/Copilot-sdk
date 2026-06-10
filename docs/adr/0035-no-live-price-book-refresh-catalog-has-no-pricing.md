# 0035. No live price-book refresh — the models catalog carries no pricing; the static book + ReportedAIU already self-heal

- Status: accepted
- Date: 2026-06-10
- Deciders: Horia
- Related: [ADR-0033](0033-reportedaiu-is-source-of-truth-for-actual-spend-price-book-is-the-estimate.md)
  (the source hierarchy this slots into), [ADR-0034](0034-price-cache-write-additive-reasoning-is-output-subset.md)
  (the rate fields a fetch would have populated), the Settings price-override editor
  (issue [0027](../issues/0027-settings-price-override-editor.md), `config.PriceOverrides`),
  epic [0050](../issues/0050-epic-billing-fidelity.md),
  issue [0061](../issues/0061-live-price-book-refresh.md) (closed by this decision),
  NEXT_FEATURES "Roadmap v10" research log

## Context

Issue 0061 (P4, the epic's last child) proposed an opt-in, cached, fail-open fetch
of per-model rate multipliers from `https://models.github.ai/catalog/models`, so a
stale hard-coded rate could self-heal without breaking the offline-single-binary
doctrine. This ADR number was reserved for the decision it would force: introducing
network egress into an offline-first tool (posture, cadence, failure mode).

The issue mandated a spike of the payload shape before building. The spike
(2026-06-10, live fetch, HTTP 200, 37 entries) found the catalog carries **no
pricing data at all**. Every entry's full key set is: `id`, `name`, `publisher`,
`summary`, `rate_limit_tier`, `supported_input_modalities`,
`supported_output_modalities`, `tags`, `registry`, `version`, `capabilities`,
`limits`, `html_url`. The only rate-ish field, `rate_limit_tier`
(`"low"`/`"high"`), is a request-throttling tier, **not** a billing multiplier.
GitHub's per-model billing multipliers are published only as documentation prose;
there is no stable, unauthenticated pricing API to poll.

## Decision

**Do not build the live price-book refresh.** The fetch cannot deliver the one
thing it existed to deliver. Rate freshness is covered by the two mechanisms the
epic already shipped:

1. **Actual spend self-heals with no network** — `ReportedAIU` is the per-turn
   source of truth (ADR-0033); a stale estimate rate can never mis-state what a
   turn really cost, and the estimate-vs-reported drift table (issue 0060) makes
   any estimate staleness *visible* per model, ambered past epsilon.
2. **Estimates are user-correctable offline** — `config.PriceOverrides` (the
   Settings price-override editor, issue 0027; 4-element form pins cache-write
   per ADR-0034) lets a user pin any model's rates the moment GitHub's published
   multipliers change.

The tier-3 option (authenticated `/rest/billing/usage` reconciliation) remains a
possible future child — it is a *reconciliation* concern, not rate freshness, and
was already deferred out of 0060 with the same opt-in/fail-open posture this ADR
would have required.

## Consequences

- The binary keeps zero runtime network dependencies; the offline-single-binary
  doctrine holds without an opt-in carve-out, its cache, or its failure modes.
- The static `DefaultPriceBook` remains the estimate's only built-in source; when
  GitHub's published multipliers move, the drift table is the detector and the
  override editor is the correction. No code change.
- Issue 0061 closes as **refuted by spike** (not "shipped"); epic 0050 closes with
  rate freshness consciously resolved this way. Revisit only if GitHub publishes a
  machine-readable pricing endpoint — that future ADR supersedes this one.
