---
id: 0092
title: "Price the timeline — per-step usage/credits in the run event log (O2)"
status: open
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

- [ ] An `EvUsage` event logs its token counts + credits; all other event types are
      byte-identical to before (the on-disk pin proves it).
- [ ] A pre-O2 log file loads and renders unpriced (zero-valued fields → no cost column
      noise, not `0.00 cr` spam).
- [ ] The detail header shows the summed event-log credits beside `RunRecord.Credits`;
      a non-trivial mismatch ambers (and a matching pair doesn't).
- [ ] Unit tests cover the new case, the additive read-back, and the cross-check;
      `make lint && make test` (floor 65%) green.

## Notes

S/M-sized; no ADR (additive fields pre-blessed by ADR-0048's additive-only rule; the
money math reuses the meter's existing computation — no new pricing logic).
