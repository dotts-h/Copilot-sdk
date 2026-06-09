---
id: 0058
title: "Per-model breakdown from the ledger + the missing integration test (roadmap v10, P2-core)"
status: closed
severity: high
group: 0050
depends_on: []
github:
links:
  adr: []
  prs: [96]
  issues: [0050]
  regression: [3]
assets: []
---

## Summary
The Telemetry per-model token table reads `0/0/0` next to real spend: the live in-process meter is
empty until turns replay through it, while the **persisted ledger already carries the token counts**
(epic 0050, finding 2). Compute the per-model breakdown **from the ledger** (populated,
restart-surviving), relabel live = "this session" vs ledger = "all-time", and add the **integration
test that currently does not exist** asserting the table is populated from history.

## Scope / Touches
- `internal/telemetry` — a pure reader: per-model aggregation (`in / cached / out + credits/usd`)
  over `SpendStore.Records()` (a cousin of the `*Shares` readers).
- `internal/web` — `pages.go` per-model table reads the ledger aggregation; relabel the two sources.
- **Test (the point):** an integration test seeding the ledger and asserting the rendered table is
  populated (closes the demo-seeding gap NEXT_FEATURES v10 calls out).

## Dependencies
- `depends_on: []` — **independent of P0/P1**: the ledger records carry today's token counts already,
  so the empty-table fix + its missing test need neither the reframed meter (0057) nor the new priced
  token types (0059). This makes it buildable **now** — a first parallel lane (disjoint seam: the
  ledger reader + the table render, not the `Meter` semantics 0057 touches).

## Acceptance
- The per-model breakdown is populated from history and **guarded by an integration test**.
- Live vs all-time sources are clearly labelled; pure reader, no schema change.
- Failing test first; `make lint && make test` + `make e2e` green; born in its PR. No ADR.

## Notes
P1 (0059) later **extends this same table** with cache-write/reasoning columns — hence 0059
`depends_on` this issue (they share the breakdown seam; serialize, don't parallelize them). See
[epic 0050](0050-epic-billing-fidelity.md).

## Resolution (shipped)
**Shipped 2026-06-09 — PR #96, part of epic 0050.** Branch `feat/billing-per-model-breakdown`
(rebased on the post-0057 main; `make codemap` re-run, `make lint && make test` green under `-race`,
`make e2e` green — 140 passed). Implemented:
- `telemetry.ModelBreakdowns` — a new pure reader (`internal/telemetry/breakdown.go`) aggregating
  per-model token counts (in/cached/out) + USD/credits + turns over `SpendStore.Records()`, sorted
  by spend desc (ties by model name). Unit-tested in `breakdown_test.go`. The `Meter`/`SpendRecord`
  types are untouched (lane 0057 owns those) — it reads the records as they already are.
- `internal/web/telemetry_render.go` — the per-model table now reads the ledger via
  `ModelBreakdowns(s.spend.Records())` instead of the empty live meter; the live token row is
  relabelled "Tokens (this session)" and the table header "Per-model breakdown (all-time, from
  history)".
- **The missing integration test** (`TestTelemetryPerModelTableIsPopulatedFromLedger`, spend_test.go):
  seeds ONLY the ledger (the live meter stays empty — the demo-seeding gap), renders the page, and
  asserts the table is populated with the summed token counts (not 0/0/0) and labelled "all-time".
No schema change, no ADR (pure reader + test). The demo ledger (`bootstrap.seedSpend`) already carries
token-bearing records, so the demo's table is now populated too.
