# 0007. Pre-flight turn cost estimate prices the live context as fresh input

- Status: accepted
- Date: 2026-06-05
- Deciders: Horia
- Related: `internal/telemetry` (`EstimateTurn`), `internal/web` (`renderStatline`),
  `docs/NEXT_FEATURES.md` item 1.2

## Context

The meter only **observes** spend after the fact; the README promises "a coding
session never surprises you on the bill," yet nothing projects the cost of the
turn you are about to send. `NEXT_FEATURES.md` item 1.2 asks for a pre-flight
estimate ("~N credits at current context") in the composer so the decision to
send — or to abort and compact first — is informed.

Two signals are already in hand: the pure `telemetry.PriceBook`/`Price`, and the
live context-window fill (`EvContextWindow` → `s.ctxCurrent`). The open question
is **what to price**, since the real cost of a turn isn't knowable before it runs:
output length is unknown, and how much of the resent context hits the prompt cache
depends on the runtime.

## Considered options

- **Price the context as fresh (uncached) input.** Each turn resends the
  accumulated context as input; pricing `ctxCurrent` at the model's *input* rate
  is the one knowable, dominant component. Simple, pure, deterministic.
- **Model cache hits + a projected output.** More "accurate," but the cache-hit
  ratio and output length are both guesses; it dresses up a guess as precision and
  needs state the estimate function would have to carry.
- **Show a token count only, no credits.** Sidesteps the modelling question but
  fails the actual ask — the differentiator is *credits*, not tokens (the context
  meter already shows tokens).

## Decision

`telemetry.EstimateTurn(pb, model, contextTokens)` prices the current context-
window fill as **fresh input** at the model's rate and returns the usual `Cost`.
It is pure (same inputs → same `Cost`), total (nil price book or non-positive
context → zero, never panics), and a thin wrapper over `Price`, so the existing
pricing fuzz/totality guarantees cover it. `Meter.EstimateTurn` delegates with the
meter's (settings-overridable) price book.

The web statusline (`renderStatline`, in the composer bar) renders it as
`next turn ~<credits>` whenever a context reading has arrived (`ctxCurrent > 0`),
refreshing on the same `stat` SSE fragment already emitted on `EvContextWindow`,
`EvUsage`, and tool starts. No new route, event, or persisted schema.

## Consequences

- Positive: the abort/compact decision is now informed by a live credit figure
  built entirely from existing primitives; zero new dependencies, no SDK surface,
  domain logic stays pure and unit-tested. The estimate tracks price-book edits
  and model switches for free.
- Trade-off we accept: the figure is **deliberately approximate** — it omits the
  prompt-cache discount on resent context (making it a conservative ceiling on
  input) and excludes output (making it a floor on the *total*). It is framed as
  "~" and labelled "next turn" so it reads as a projection, not a bill; the
  authoritative AIU on the Telemetry page remains the ground truth.
- Follow-ups: a tighter estimate could fold the observed cache-hit ratio and a
  rolling average output size once spend history is persisted (item 1.3); the soft
  warn / hard cap (item 1.1) can reuse `EstimateTurn` to project an over-budget
  turn before it runs.
