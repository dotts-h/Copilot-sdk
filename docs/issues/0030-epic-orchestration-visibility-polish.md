---
id: 0030
title: "Epic: orchestration visibility & polish (roadmap v5)"
status: open
severity: medium
group:
github:
links:
  adr:
  prs:
  issues: [0031]
  regression:
assets: []
---

## Charter

Roadmap **v4** (epic 0024: convergence dashboards & cost-surface completion) is shipped
and closed — the Workflows page, the Telemetry per-bucket forecast, the Settings price
knobs, the Sessions picker, and the spend-over-time trend are all cost-aware /
inspectable. With the **cost-awareness** differentiator now deep *and* surfaced, v5
turns to sharpening the **orchestration** differentiator's *visibility*: the
multi-agent surfaces the app already drives should say more about *what* is happening,
and the keymap/store machinery underneath should stop leaking small rough edges.

This epic carries the **v5 orchestration-visibility + polish** picks:

- **V3 — sub-agent descriptions on the activity strip.** During a parallel run the
  chat shows a strip of one chip per concurrent sub-agent — but only its name + model,
  not *what* it is doing, even though the SDK already populates each sub-agent's
  `Description`. Surfacing it makes the orchestration legible at a glance.
- **V10 — keybinding live-apply** (TECH_DEBT #13). A rebind on the Settings page takes
  effect only on the next full page load; an OOB swap of `<body data-keymap>` + the help
  overlay on the Settings POST closes the gap.
- **H1 — generic `telemetry.AppendOnlyStore[T]`** (debt paydown). The duplicated
  `SpendStore`/`RunStore` machinery (atomic write, migration, round-trip) collapses into
  one generic store, guarded by the existing round-trip/atomic/migration tests; the
  on-disk JSON tags are the stable contract and must not change.

## Tasks

- [ ] **V3 — surface `SubagentInfo.Description` on the sub-agent activity strip** →
      [0031](0031-subagent-description-activity-strip.md) (no ADR — a pure
      presentation-layer surfacing of an already-populated SDK field, escaped through
      `html/template` per ADR-0001). The `renderSubagents` chip gains the sub-agent's
      `Description` as a `title=` tooltip so concurrent sub-agents in a parallel run say
      *what* they are doing; an empty description renders the prior chip shape.
- [ ] **V10 — keybinding live-apply** (S, polish; TECH_DEBT #13) — not yet promoted into
      an issue. An OOB swap of `<body data-keymap>` + the help overlay on the Settings
      POST so a rebind applies without a full page reload.
- [ ] **H1 — generic `telemetry.AppendOnlyStore[T]`** (M, debt) — not yet promoted into an
      issue. Extract the duplicated `SpendStore`/`RunStore` machinery into one generic
      store, guarded by the existing round-trip/atomic/migration tests; the on-disk JSON
      tags must not change.

## Status

**Open.** First child **V3** (sub-agent descriptions on the activity strip — issue 0031)
is the opening build. **V10** (keybinding live-apply) and **H1** (generic
`AppendOnlyStore[T]`) remain unbuilt children; this epic stays open until they ship.

## Notes

Per CONVENTIONS: write the failing test first; keep domain logic pure
(`telemetry`/`ctxforge`/`config` dependency-free); `make lint && make test` (floor 65%)
+ `make e2e` for UI; fold ADR/CONTRACTS/REGRESSIONS into the feature branch that
motivates them (ADR-0004). V3 needs **no ADR** — it is a pure web-layer render change
over an already-populated SDK field (`SubagentInfo.Description`), escaped through
`html/template` like every other chip value (ADR-0001).

## Numbering

Highest on disk before this pass: issues → **0029**, epic → **0024**, ADRs → **0022**.
This epic takes **0030**; its first child **V3** takes issue **0031**. No ADR consumed
(V3 is a presentation-layer surfacing pre-blessed by ADR-0001).
