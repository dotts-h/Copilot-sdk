---
id: 0030
title: "Epic: orchestration visibility & polish (roadmap v5)"
status: closed
severity: medium
group:
github:
links:
  adr:
  prs: [56, 57, 58]
  issues: [0031, 0032, 0033]
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

- [x] **V3 — surface `SubagentInfo.Description` on the sub-agent activity strip** →
      [0031](0031-subagent-description-activity-strip.md) (**shipped**, PR #56; no ADR — a
      pure presentation-layer surfacing of an already-populated SDK field, escaped through
      `html/template` per ADR-0001). The `renderSubagents` chip gained the sub-agent's
      `Description` as a `title=` tooltip so concurrent sub-agents in a parallel run say
      *what* they are doing; an empty description renders the prior chip shape.
- [x] **V10 — keybinding live-apply** (S, polish; TECH_DEBT #13) → **shipped** as issue
      [0032](0032-keybinding-live-apply.md). The Settings keybinding POST now appends an
      `hx-swap-oob` re-render of the help overlay + an `applyKeymap(…)` script that updates
      `<body data-keymap>` and rebuilds the JS dispatcher's map, so a rebind applies without
      a full page reload. Reads back the persisted keymap so a no-op/rolled-back save can't
      desync live from disk; escaped per ADR-0001. Completes the ADR-0014 mechanism (no new
      ADR); resolves TECH_DEBT #13, guards REGRESSIONS #18.
- [x] **H1 — generic `telemetry.AppendOnlyStore[T]`** (M, debt) → **shipped** as issue
      [0033](0033-generic-append-only-store.md) (PR #58; no ADR — a refactor-only paydown
      that keeps the ADR-0009 spend + ADR-0022 run envelopes byte-identical). The duplicated
      `SpendStore`/`RunStore` machinery (atomic temp+rename, missing=empty / corrupt=error /
      newer-version-tolerant / ephemeral, `Append`/`Records`/`Count`) collapsed into one
      generic `AppendOnlyStore[T]` (`store.go`); the two stores are now thin typed embeddings
      that preserve their exact public API, and the on-disk JSON tags (`version` +
      `records`/`runs`) are unchanged — verified byte-identical and guarded by the existing
      round-trip/atomic/migration/ephemeral tests plus a direct generic-store test and the
      on-disk-tag pins `TestSpendStoreOnDiskTagsAreStable` / `TestRunStoreOnDiskTagsAreStable`.
      Resolves the AppendOnlyStore duplicated-machinery debt (TECH_DEBT, paid). **Third and
      final child — its merge closes this epic.**

## Status

**Closed.** All three children shipped: **V3** (sub-agent descriptions on the activity
strip — issue 0031, PR #56), **V10** (keybinding live-apply — issue 0032, PR #57), and
**H1** (generic `telemetry.AppendOnlyStore[T]` — issue 0033, PR #58). The epic closed on
H1's merge.

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
