---
id: 0023
title: Workflow run-history aggregations + Runs duration (roadmap v3, item V1)
status: open
severity: medium
group: 0022
github:
links:
  adr:
  prs: []
  issues: [0022]
  regression:
assets: []
---

## Summary

B3 (ADR-0022) persists each workflow run to `telemetry.RunStore` and the Runs page
lists them most-recent-first — but the page is a flat log: it shows no **duration**
(both `StartedAt` and `FinishedAt` are stored, only `StartedAt` is rendered via
`runRow`) and no **roll-up** (total runs, average cost, average duration, failure rate
per workflow). The two persisted stores hold complementary data that is **never
joined** in any view: `SpendStore.WorkflowShares` shows *spend* per workflow, but never
cross-links to run count / avg duration / failure rate from `RunStore`. This is the
pure-reader follow-on the B3 ADR explicitly deferred ("aggregations … deferred until a
surface needs them — the records carry enough to compute them later without a schema
change"). The convergence of the two differentiators — **cost ⋈ orchestration** — lands
here. Source: `docs/NEXT_FEATURES.md` item V1.

## Repro
1. Run several workflows, some failing/branching, then open Runs and Telemetry.
   - **Expected:** each run shows how long it took; a per-workflow summary shows run
     count, avg cost, avg duration, and failure rate.
   - **Actual:** Runs shows only name/mode/outcome/when/total-cost per run; there is no
     duration column and no aggregate anywhere. `WorkflowShares` shows spend but can't
     answer "how often does this workflow run, and how often does it fail?"

## Proposed resolution (pure readers — no schema change)

- **`internal/telemetry` (pure, unit-tested):** add a `RunAggregate`
  (`{workflowID, name, runs, failures, totalCredits, avgCredits, totalDuration,
  avgDuration}`) and a `RunAggregates(records []RunRecord) []RunAggregate` roll-up
  (a cousin of `ModelShares`/`AgentShares`/`WorkflowShares` — deterministic ordering),
  plus a `RunRecord.Duration() time.Duration` helper (`FinishedAt.Sub(StartedAt)`,
  guarding a zero/negative). No `RunStore`/`RunRecord` schema change — the records
  already carry start/finish/outcome/per-lane credits.
- **`internal/web` (Runs view):** `runRow` (`internal/web/runs.go`) gains a duration
  cell; `runsPartial` renders a per-workflow summary table above the run list,
  resolving workflow ids to names under `forgeMu` like the existing breakdowns. Pairs
  with the Telemetry `WorkflowShares` section (optionally cross-link run count there).
- **Tests:** unit — `RunAggregates` over a mixed history (a failed run, a branched run
  with a skipped lane that adds zero cost, two runs of one workflow) computes counts/
  averages/failure-rate correctly and deterministically; `Duration` guards a
  zero/unfinished record. web — the Runs page renders the duration cell and the summary
  rows (structure). e2e — assert the summary section/structure, never figures (the demo
  run store is shared + append-only across the suite — same gotcha family as the spend
  trend).

## Notes
- **No ADR:** the decision is obvious and already pre-blessed by ADR-0022 — pure
  aggregations over the existing `RunStore` records, mirroring the established
  `*Shares` reader pattern, no schema change, no new IO. (Per ADR-0004 an ADR leads only
  a *non-obvious* decision.)
- **Differentiator:** cost ⋈ orchestration (the two stores finally joined); a
  pure-reader follow-on to B3 — small, compounding.
- **Numbering:** issue **0023** (next free after 0021; 0019/ADR-0020 went to C1, 0022 is
  the v3 epic). No ADR consumed.
