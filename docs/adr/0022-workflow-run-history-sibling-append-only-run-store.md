# 0022. Workflow run history — a sibling append-only run store

- Status: accepted
- Date: 2026-06-06
- Deciders: Horia
- Related: extends [ADR-0013](0013-multi-agent-workflow-run-handoff-surface.md)
  (multi-agent workflow run / handoff surface),
  [ADR-0017](0017-per-lane-tool-and-permission-surface-for-parallel-workflow-lanes.md)
  (per-lane surface), and
  [ADR-0021](0021-conditional-branching-workflow-steps-declarative-predicate.md)
  (branching — a run now includes `laneSkipped` outcomes); mirrors the persistence
  pattern of [ADR-0009](0009-persisted-spend-history-append-only-ledger.md)
  (append-only ledger) and [ADR-0016](0016-ledger-is-source-of-truth-for-account-wide-budget-accounting.md)
  (the ledger as source of truth); `internal/telemetry` (`runs.go` — `RunStore`,
  `RunRecord`, `RunLane`), `internal/web` (`workflow.go` — `runRecord`/`recordRun`
  at run completion, `workflowRun.started`; `pages.go` — the `runsPartial`/`runsPage`
  Runs view, nav page), `internal/bootstrap` (`Build`, `seedRuns`),
  `e2e/tests/e2e.spec.ts`; `docs/NEXT_FEATURES.md` item B3,
  [issue 0021](../issues/0021-workflow-run-history.md),
  [ADR-0018](0018-additive-attribution-tags-on-spend-records.md)

## Context

A workflow run (ADR-0013) is the product's unit of orchestration: a set of lanes —
sequential handoff or parallel fan-out (and, since ADR-0021, with branched lanes that
may be **skipped**) — driven to completion. But a run is **ephemeral**: once it
finishes, `runFrags` clears `s.busy` and the `workflowRun` is overwritten by the next
run. There is no record of *what ran, with which agents, to what outcome, at what
cost*. A2 (ADR-0018) tagged each **spend** record additively with `workflow`/`lane`,
so per-workflow *spend* rolls up — but a ledger row is "one metered turn," which
cannot express a run's **start/finish/outcome**, nor a **skipped lane that incurred no
cost** (a branch that didn't run leaves no spend record at all). B3
(`docs/NEXT_FEATURES.md` Tier B) asks to **persist each run** and add a "Runs" view.

The roadmap is explicit: lead with an ADR for the run-store schema, keep the store
pure + unit-tested, and make the web recording a thin adapter at run completion. Two
questions had to be answered: **what the run-store schema is and where it lives**, and
**where the web layer records a run**.

## Considered options

- **Where run history lives / its schema.**
  - **A new sibling append-only store `telemetry.RunStore` at
    `<configDir>/runs.json` (chosen).** A versioned envelope
    `{"version":1,"runs":[…]}` written **atomically** (temp-file + rename,
    validate-on-load), missing-file = empty, present-but-invalid = error, empty dir =
    ephemeral — the exact discipline of `SpendStore` (ADR-0009). Each `RunRecord` is
    `{id, workflow, name, mode, startedAt, finishedAt, outcome, lanes:[{index,
    agentId, status, credits}]}`, where a lane's `status` ∈ {`done`, `failed`,
    `skipped`} — so a **branched run's per-lane outcomes, including the skips, are
    first-class**. The store is the one IO edge; the record types and any future pure
    aggregations stay dependency-free next to the cost types, like the ledger.
  - *Derive run history from the existing `SpendStore` records' workflow/lane tags
    (no new store).* Rejected — spend tags **cannot represent a skipped lane that
    incurred no cost** (no turn → no record), nor a run's start/finish/outcome, nor
    distinguish two runs of the same workflow on the same day. It loses exactly the
    branching outcomes ADR-0021 made first-class.
  - *Fold runs into `SpendStore` as a new record kind.* Rejected — it overloads the
    ledger's "one row per metered turn" contract (ADR-0009) with a different grain and
    forces every spend reader (`DailyTotals`, `MonthToDate`, the shares) to filter a
    foreign row kind. A run and a metered turn are different nouns; they get different
    files.

- **Where the web layer records a run.**
  - **Once, on completion, in `runFrags(run, done=true)` (chosen).** That is the
    single point where a run terminates: `s.busy` is cleared and the outcome note is
    added there, and it is reached **exactly once** per run (after `run.done` flips,
    `handleEvent` no longer routes events to `handleRunEvent`). A thin `recordRun`
    builds a `RunRecord` from the finished `workflowRun` (a pure `runRecord` mapping —
    lane status int → string, lane credits, agent id, index; outcome from
    `run.failed`) and appends best-effort (a disk error is logged, never surfaced —
    like the ledger append).
  - *Record incrementally as lanes settle.* Rejected — a run is the unit; a
    half-written run is not a useful history row, and "record once at the terminal
    state" is simpler and matches where `busy` already clears.

## Decision

Add `telemetry.RunStore` (`internal/telemetry/runs.go`): `LoadRunStore(dir)` (missing
= empty, invalid = error, `""` dir = ephemeral in-memory), `Append(RunRecord)` (stamps
`FinishedAt` when zero, persists the whole document atomically), `Records()` (snapshot
copy), `Count()`. On-disk shape is the versioned envelope `{"version":1,"runs":[…]}`;
the `runs` array is the **stable** contract and `version` gates migrations (additive
fields only, or a converting migration — CONTRACTS §4), mirroring `SpendStore`.
`RunRecord` is `{ID, WorkflowID, Name, Mode, StartedAt, FinishedAt, Outcome, Lanes}`
and `RunLane` is `{Index, AgentID, Status, Credits}`; JSON tags are the contract.

In the web layer, `workflowRun` gains a `started time.Time` field (stamped by the
Server adapter in `handleWorkflowRun`, keeping the pure engine methods clock-free).
`recordRun` is called from `runFrags` when the run finishes: it builds the record via
the pure `runRecord(run)` mapping and appends to `s.runs` best-effort. A new top-level
**Runs** nav page (`runsPartial` → `runsPage`) lists the persisted runs most-recent
first — each run's name, mode, outcome, when, total cost, and a per-lane breakdown
(agent, status glyph reusing the lane vocabulary, credits) — resolving agent/workflow
ids to display names under `forgeMu` like the Telemetry cost breakdowns.
`bootstrap.Build` loads `runs.json` from the config dir for the real app and seeds a
deterministic ephemeral store (`seedRuns`, including a branched run with a skipped
lane) for demo, exactly as it does for the spend ledger.

## Consequences

- Positive: a run is now a **persisted, first-class history row** that survives
  restart — including a branched run's **skipped** lanes, which the spend-tag-derived
  alternative could not see. The store reuses the atomic-write discipline proven for
  config/forge/spend; the record types are pure and the web recording is a thin
  adapter at the one completion point, so the engine stays a pure, unit-tested state
  machine. Demo/tests never touch disk (ephemeral store).
- Backward/forward-compatible: `runs.json` is a fresh file (no migration of existing
  data); the envelope is versioned so later additive fields stay readable by older
  code and a higher `version` is tolerated (CONTRACTS §4).
- Two stores, one pattern: spend (`spend.json`, one row per metered turn) and runs
  (`runs.json`, one row per orchestrated run) are **siblings**, not a merged file —
  each keeps its own grain and readers. Per-workflow *spend* (ADR-0018) and per-run
  *history* (this ADR) answer different questions and compose on the Telemetry/Runs
  surfaces.
- Record-once invariant (REGRESSIONS): `recordRun` runs in `runFrags(done=true)`,
  reached exactly once per run; a skipped lane persists with `status:"skipped"` and a
  zero `credits`. Drop the call and run history goes empty; move it earlier and a run
  could be written twice or half-finished.
- Escaping (ADR-0001 held): a run's name, agent names, and lane status reach the
  browser through the same `richtext` / `html/template` auto-escaping as the rest of
  the workflow surface.
- Nav coupling (REGRESSIONS): adding the **Runs** top-level page touches the
  `pageNames` / e2e `pages` count coupling — the `e2e/tests/helpers.ts` `pages` array
  is updated in nav order so the nav-count assertion stays green.
- Trade-off accepted: the full-file rewrite is O(n) per run and the append is
  synchronous under `s.mu` — bounded by the same low single-user volume as the ledger
  (one record per *run*, far rarer than per turn); batching/async is a noted follow-up
  if volume ever grows. Pure run-history *aggregations* (e.g. per-workflow run counts,
  average run cost) are deferred until a surface needs them — the records carry enough
  to compute them later without a schema change.
- Contract change: `telemetry.RunStore` / `RunRecord` is a new persisted schema and
  `/page/runs` is a new route/view — recorded in CONTRACTS §3/§4 and ARCHITECTURE;
  guarded in REGRESSIONS.
