---
id: 0025
title: Workflow list "last run" + cost badges (roadmap v4, item V4)
status: closed
severity: medium
group: 0024
github:
links:
  adr:
  prs: []
  issues: [0024, 0023]
  regression:
assets: []
---

## Summary

The **Workflows page** (`workflow.go` `workflowsPartial`) is the orchestration entry
point, but each row is purely navigational — name + step summary only, with no signal of
whether the workflow has ever run, how it last ended, how often, or what it costs. V1
(epic 0022, issue 0023) shipped `telemetry.RunAggregates` (run count / failures / avg
cost / avg duration per workflow) + `RunRecord.Duration()` and badged the *Runs* page
with a per-workflow summary — but the Workflows page never joined the two stores. V4 is
that same cost ⋈ orchestration convergence **one surface over**: join `RunStore` (run
count + last-run outcome/age) and `SpendStore` (authoritative per-workflow spend) keyed
by workflow id, and badge each Workflows row. Pure readers over existing records, **no
schema change**. Source: `docs/NEXT_FEATURES.md` item F2/V4.

## Repro
1. Open the Workflows page after running a workflow a few times.
   - **Expected:** each row shows its last-run outcome + how long ago, how many times it
     has run, and its total spend — so the page is diagnostic, not just navigational.
   - **Actual (before V4):** each row shows only name + step summary; whether the
     workflow has ever run, how it ended, or what it cost is invisible here (only the
     Runs and Telemetry pages carry it).

## Proposed resolution (pure readers — no schema change)

- **`internal/telemetry` (pure, unit-tested):** extend `RunAggregate` with the
  **last-run** signal it lacks — `LastOutcome string` + `LastStartedAt time.Time`,
  populated in the `RunAggregates` pass by tracking the most recent run per workflow
  (latest by `StartedAt`; a tie broken by the later `FinishedAt`, then stable input
  order → deterministic). Additive: the existing V1 fields/tests are unchanged.
  Per-workflow **spend** stays `WorkflowShares`' job — the two readers are joined in the
  web layer, keeping each pure over its own store.
- **`internal/web` (Workflows view):** `workflowsPartial` joins
  `RunAggregates(s.runs.Records())` and `WorkflowShares(s.spend.Records())` into a
  per-workflow-id map under `forgeMu`; each row gains a last-run outcome glyph + relative
  age (`humanWhen`), a run count, and total spend (`FormatCredits`). The all-absent case
  (no runs and/or no spend store wired) renders today's shape, no badges. All values
  through `html/template` (ADR-0001).
- **Tests:** unit — `RunAggregates` reports the latest run's outcome + start across a
  multi-run workflow (last-by-`StartedAt` wins deterministically; a single run; an
  unfinished latest; a tie broken by `FinishedAt`). web — a row renders the run-count +
  last-run glyph + spend badges when both stores carry the workflow (structure + the
  glyph matches outcome), and renders cleanly with no run store / no spend store / an
  orphan (since-deleted) id (no panic, no badge). e2e — assert the badge structure on a
  seeded row, never figures (the demo run/spend stores are shared + append-only).

## Resolution (shipped)

Pure-reader convergence landed, no schema change. `internal/telemetry`: `RunAggregate`
gained `LastOutcome string` + `LastStartedAt time.Time`, populated in the `RunAggregates`
roll-up by tracking the most recent run per workflow (latest by `StartedAt`; a
same-`StartedAt` tie broken by the later `FinishedAt`, then stable input order, via the
pure `laterRun` helper — deterministic regardless of record order). Additive: the V1
fields/tests are untouched (`Runs > 0` is the guard for the never-produced zero value).
`internal/web`: `workflowsPartial` joins `RunAggregates(s.runs.Records())` and
`WorkflowShares(s.spend.Records())` under `forgeMu` into per-workflow-id maps (each
guarded for a nil store), and each row gains a `wf-lastrun` outcome glyph + `humanWhen`
age, a `wf-runs` count, and a `wf-spend` total (`FormatCredits`) — via the `workflowRow`
template + `wf-badges` CSS. A workflow with no runs/spend renders its prior navigational
shape, no badges; an id present only in a store badges no row. Tests: unit
(`TestRunAggregatesLastRun` — multi-run latest-wins, single, unfinished latest,
`StartedAt`-tie → later `FinishedAt`, order-independent); web
(`TestWorkflowsPageBadgesRunAndSpend`, `TestWorkflowsPageNoStoresNoBadges`,
`TestWorkflowsPagePartialAndOrphanStores` — structure + glyph-matches-outcome + the
absent/partial/orphan-store joins); e2e (`badges the seeded workflow row with last-run +
spend` — structure only, the demo stores are shared + append-only). Docs: CONTRACTS
§3 (Workflows badge surface) + §4 (the new `RunAggregate` fields). No REGRESSIONS entry:
the last-run tie-break and the nil-store join were guarded preemptively by the unit/web
tests; no real bug was found-and-fixed. Shipped on branch
`claude/workflow-last-run-badges`.

## Notes
- **No ADR:** a pure-reader composition over the existing `RunStore` + `SpendStore`
  records, mirroring the V1 (issue 0023) reader pattern and the established `*Shares`
  readers — no schema change, no new IO. Pre-blessed by the same cost ⋈ orchestration
  convergence rationale as ADR-0022 (per ADR-0004 an ADR leads only a *non-obvious*
  decision).
- **Differentiator:** cost ⋈ orchestration — the second surface to join the two stores
  (after V1's Runs page); a pure-reader follow-on, small + compounding.
- **Numbering:** issue **0025** (next free after 0023), first child of epic **0024**
  (roadmap v4). No ADR consumed.
