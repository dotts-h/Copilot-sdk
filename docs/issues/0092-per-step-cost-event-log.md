---
id: 0092
title: "Price the timeline — per-step usage/credits in the run event log (O2)"
status: closed
severity: medium
group: 0090
depends_on: [0091]
github:
links:
  adr:
  prs: []
  issues: [0090]
  regression:
assets: []
---

## Summary

`RunEvent` records `EvUsage` as type-only — `normalizeRunEvent`
(`internal/web/eventlog.go`) has no `EvUsage` case, so the log carries no tokens/credits
and the timeline can't be priced. Add the missing case, stamping the turn's token counts +
**computed credits at time of use** into additive `RunEvent` fields (omitempty; older logs
read back zero — the additive-only rule of ADR-0048). The inspector (0091) prices each turn
row, rolls up per-lane subtotals, and cross-checks the header total against
`RunRecord.Credits` — a per-run mini-reconciliation, ambered on mismatch (the V15
discipline at run grain).

## Why now

The finest cost⋈orchestration attribution grain (run → lane → **step/turn**), one additive
field away. Price-at-time-of-use matters independently: token prices are falling 80%+/yr,
so a later price-book change (or the G1 live repricing) must never silently reprice
history — the log keeps the credits the meter actually computed.

## Touches

- `internal/telemetry` — `eventlog.go` (additive `RunEvent` fields, e.g.
  `tokensIn`/`tokensOut`/`credits`, omitempty; extend the tag-stability pin).
- `internal/web` — `eventlog.go` (`normalizeRunEvent` `EvUsage` case — the credits the
  pump's metering path computed for that event), `runs.go` (per-turn pricing, per-lane
  subtotals, header cross-check vs `RunRecord.Credits`).

## Acceptance

- [x] An `EvUsage` event logs its token counts + credits; all other event types are
      byte-identical to before (the on-disk pin proves it).
- [x] A pre-O2 log file loads and renders unpriced (zero-valued fields → no cost column
      noise, not `0.00 cr` spam).
- [x] The detail header shows the summed event-log credits beside `RunRecord.Credits`;
      a non-trivial mismatch ambers (and a matching pair doesn't).
- [x] Unit tests cover the new case, the additive read-back, and the cross-check;
      `make lint && make test` (floor 65%) green.

## Notes

S/M-sized; no ADR (additive fields pre-blessed by ADR-0048's additive-only rule; the
money math reuses the meter's existing computation — no new pricing logic).

## Close-out

Shipped. Three additive `RunEvent` fields (`tokensIn`/`tokensOut`/`credits`, omitempty —
older logs read back zero, ADR-0048's additive-only rule) carry per-turn usage; the on-disk
tag pin (`TestRunEventLogPricedUsageRoundTrips`) proves every other event stays
byte-identical. `normalizeRunEvent` gained an `EvUsage` case stamping the **price-book estimate**
credits the meter computed **at time of use** (via the new `usageCredits` helper) — the SAME
figure `recordUsage` adds to the run's lane (`run_adapter.go`: `l.credits += cost.Credits()`),
deliberately NOT the reported-AIU authoritative basis, so the summed log credits reconcile
against `RunRecord.Credits` on one basis. Frozen at log time, so a later price-book change can't
silently reprice history. The run inspector (0091) rolls the priced usage into per-lane subtotals
(coalesced away as steps, ADR-0052) and cross-checks the summed event-log credits against
`RunRecord.Credits` in the detail header, ambering a non-trivial mismatch at `reconcileEpsilon`
(the V15 reconcile discipline at run grain) — which catches a genuine log↔record drift, never the
estimate-vs-reported gap the Telemetry ModelDrift table already surfaces (ADR-0033). A pre-O2 log
renders unpriced — no cost-column noise. `make lint && make test` green (telemetry 96.8%, web
89.9%); inspector e2e green.
