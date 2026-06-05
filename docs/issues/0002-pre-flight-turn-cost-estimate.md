---
id: 0002
title: Pre-flight turn cost estimate (Tier 1, item 1.2)
status: closed
severity: high
group: 0001
github:
links:
  adr: ../adr/0007-pre-flight-turn-cost-estimate-prices-context-as-fresh-input.md
  prs: []
  issues: [0001]
  regression:
assets: []
---

## Summary

Before a turn runs there was no signal of what it would cost, so the decision to
send vs. abort-and-compact was uninformed. Surface a live estimate in the composer
("next turn ~N cr") derived from the price book × the current context-window fill.
Source: `docs/NEXT_FEATURES.md` item 1.2.

## Repro
1. Open the chat; send a prompt so a context-window reading arrives.
2. Expected: the statusline projects the next turn's credit cost at the current context.
3. Actual (before): the statusline showed tokens and *spent* cost only — no projection.

## Evidence

Verified end-to-end against the offline demo: after a scripted turn (18.4k context
tokens, gpt-5) the statusline renders `next turn ~2.30 cr` (18,400 × $1.25/Mtok =
$0.023 = 2.30 credits).

## Resolution

- `telemetry.EstimateTurn(pb, model, contextTokens)` — pure, total (nil price book
  or non-positive context → zero), a thin wrapper over `Price` that prices the live
  context as fresh input; `Meter.EstimateTurn` delegates with the meter's price book.
- `renderStatline` shows `next turn ~<credits>` once a context reading exists
  (`ctxCurrent > 0`), refreshed by the existing `stat` SSE fragment on
  `EvContextWindow` / `EvUsage` / tool starts. No new route, event, or schema.
- Design recorded in **ADR-0007** (why price context as fresh input).

## Notes

Guarding tests: `internal/telemetry` `TestEstimateTurn`,
`TestEstimateTurnUnknownModelUsesFallback`, `TestEstimateTurnNonPositiveContextIsZero`,
`TestEstimateTurnNilPriceBookIsZero`, `TestMeterEstimateTurnUsesItsPriceBook`;
`internal/web` `TestStatlineShowsPreflightEstimateAtCurrentContext`; browser:
`e2e/tests/e2e.spec.ts` "the statusline shows a pre-flight cost estimate once context
is known". Follow-ups tracked in epic 0001 (1.1 can reuse `EstimateTurn`).
