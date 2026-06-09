# 0034. Price cache-write tokens (additive, 1.25× input); reasoning is a subset of output, not a second charge

- Status: accepted
- Date: 2026-06-09
- Deciders: Horia
- Related: `internal/telemetry` (`ModelRate` — new `CacheWritePerMTok`;
  `pricing.go` `DefaultPriceBook`/`BuildPriceBook` — per-model cache-write default
  + override; `credits.go` `Cost` — new `CacheWriteUSD`, `Price`/`Meter.Record`/
  `ModelTotals`; `history.go` `SpendRecord` — additive `cw`/`reasoning` token
  fields, schema v3; `breakdown.go` `ModelBreakdown` — cache-write + reasoning
  columns), `internal/config` (`PriceOverrides` migrates `[3]float64`→`[]float64`,
  backward-readable), `internal/web` (`settings.go` price editor cache-write
  column; `session.go` persists the two counts; `telemetry_render.go` + the
  per-model table fragment), [ADR-0033](0033-reportedaiu-is-source-of-truth-for-actual-spend-price-book-is-the-estimate.md),
  [ADR-0027 price-override editor] (issue [0027](../issues/0027-settings-price-override-editor.md)),
  REGRESSIONS #3 (display-only `ExtraTokens`),
  epic [0050](../issues/0050-epic-billing-fidelity.md),
  issue [0059](../issues/0059-price-cache-write-and-reasoning-tokens.md)

## Context

The billing-fidelity audit (epic 0050, finding 1) flagged two billed token types
that the estimate price book did not model — both sat in display-only
`ExtraTokens` (REGRESSIONS #3): a **cache-write** cost (Anthropic ≈ **1.25×
input**) and **reasoning/thinking** tokens "billed at the output rate." 0057
(ADR-0033) reframed the meter so `ReportedAIU` is the truth and the price book is
the *estimate*; this child makes the estimate model the two types so it tracks
GitHub's bill instead of under-counting.

Building it surfaced a correctness fork in **how the SDK reports the two counts**
(`copilot-sdk/go` `AssistantUsageData`):

- **Cache-write** — `CacheWriteTokens`: *"Number of tokens written to prompt
  cache."* A category **distinct** from `InputTokens` and `CacheReadTokens`, with
  its own count, **additive** to the bill and **currently unpriced**. This is a
  real under-count.
- **Reasoning** — `ReasoningTokens`: *"Number of **output tokens** used for
  reasoning (e.g., chain-of-thought)."* It is a **subset of `OutputTokens`**
  ("output tokens produced"), not a separate quantity. The estimate already prices
  every output token at `OutputPerMTok`, so reasoning is **already priced** — at
  the output rate, exactly as the audit wanted.

Pricing reasoning as a *second*, additive line at the output rate (a literal
reading of "bill reasoning at the output rate") would **double-count** every
thinking token: once inside `OutputTokens` and again as a reasoning charge. That
inflates the estimate — the very drift this epic exists to remove.

## Decision

1. **Cache-write becomes a first-class priced category.** `ModelRate` gains
   `CacheWritePerMTok`; `DefaultPriceBook` seeds it at **1.25 × `InputPerMTok`**
   for every model (the confirmed Anthropic cache-creation multiplier).
   `Cost.CacheWriteUSD = CacheWriteTokens × CacheWritePerMTok / 1e6` is **added**
   to `Cost.USD()`. The meter, per-model totals, the ledger, and the per-model
   breakdown all fold cache-write in.

2. **Reasoning is NOT a second charge.** It stays a **display-only** count
   (`ExtraTokens`, and a new breakdown column) because it is a subset of
   `OutputTokens`, which the estimate already prices at the output rate. The
   estimate is correct without a reasoning line; adding one would double-count.
   The column makes "how much of output was thinking" visible without touching the
   credit math.

3. **Cache-write is overridable, with a backward-readable migration.** The
   persisted per-model override (`config.PriceOverrides`) migrates from
   `[3]float64` (input, cached, output) to `[]float64` of length **3 or 4** — the
   optional 4th element is the cache-write rate. A pre-0059 config (3-element
   arrays) loads unchanged and prices cache-write at the **1.25× default**; a
   4-element row overrides it. `BuildPriceBook` stays **rebuild-not-incremental**
   and deterministic (ADR-0027): same overrides → same book, a removed override
   reverts to the default. Reasoning has no override (it is not a price, it is a
   slice of output).

## Consequences

- The estimate now models every token type GitHub bills, so it tracks
  `ReportedAIU` far closer (validated by the estimate-vs-reported drift row, 0060,
  next).
- `SpendRecord` grows additive `cw` + `reasoning` token fields (schema **v3**,
  backward-readable: older records read back `0`, like the v2 attribution tags) so
  the all-time per-model breakdown shows both from history.
- The price math stays **pure and deterministic**; cache-write rides the same
  `*PriceBook` reference, so a live Settings reprice (ADR-0027 `Replace`) updates it
  atomically alongside the other rates.
- Anyone tempted to "also charge reasoning" must re-read finding 1 against the SDK
  field doc: reasoning ⊆ output. The guard is `TestPriceDoesNotDoubleCountReasoning`.

## Alternatives considered

- **Charge reasoning additively at the output rate.** Matches the epic's prose but
  contradicts the SDK contract (`ReasoningTokens` ⊆ `OutputTokens`) — double-counts
  and re-introduces drift. Rejected.
- **Expose cache-write as a free, independent per-model price with no default
  link to input.** Cache-write is structurally a fixed multiple of the base input
  rate; a free knob invites a value inconsistent with the billing model. Kept the
  1.25× default and made the 4th override optional, so the common case needs no
  input and an advanced user can still pin a rate.
- **A new struct value for `PriceOverrides` instead of `[]float64`.** A struct
  would not unmarshal an old 3-element JSON array without a custom decoder; a
  variable-length slice reads both the 3- and 4-element forms natively — the
  cleaner backward-readable migration.
