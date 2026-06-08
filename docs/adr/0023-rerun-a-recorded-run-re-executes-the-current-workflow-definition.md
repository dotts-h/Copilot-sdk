# 0023. Rerun a recorded run — re-execute the current workflow definition

- Status: accepted
- Date: 2026-06-08
- Deciders: Horia
- Related: the first **action** on the orchestration history surface, building on
  [ADR-0022](0022-workflow-run-history-sibling-append-only-run-store.md) (the persisted
  run store the Runs page reads) and reusing the run-trigger of
  [ADR-0013](0013-multi-agent-workflow-run-handoff-surface.md) (multi-agent workflow run /
  handoff) + [ADR-0017](0017-per-lane-tool-and-permission-surface-for-parallel-workflow-lanes.md)
  (per-lane surface); rolls up under the per-workflow attribution of
  [ADR-0018](0018-additive-attribution-tags-on-spend-records.md). Touches
  `internal/web` (`workflow.go` — the extracted `launchWorkflow` trigger shared by
  `handleWorkflowRun` and the new `handleRunRerun`; `runs.go` — `runRow` `CanRerun`;
  `hub.go` — the `POST /runs/rerun/{workflow}` route), `docs/CONTEXT.md` (the **rerun**
  term), CONTRACTS §3, [issue 0043](../issues/0043-rerun-workflow-from-runs-page.md),
  epic [0042](../issues/0042-epic-interactive-orchestration.md).

## Context

After roadmaps v4–v7, the orchestration surface is **fully observable but entirely
read-only**: the Runs page (ADR-0022) lists every persisted run — its lanes, outcomes
(including skipped branches), duration, and metered cost — and the Telemetry page
reconciles ledger vs. runs. A user can *see* that a run failed or cost too much, but the
only way to **act** on it — re-execute that workflow — is to navigate to the Workflows
page and find the row by hand. The orchestration surface has no action on it at all; the
sole run-trigger (`POST /workflows/{id}/run`, ADR-0013) lives only on the Workflows page.

Roadmap v8 (epic 0042) makes the orchestration surface **interactive**, and its first
child is **rerun from the Runs page**: a control on each recorded run that re-executes
that run's workflow. This is the first action ever triggered from a **historical record**
rather than a live forge entity, which raises two questions an ADR must settle: **what
"rerun" re-executes** (the recorded run, or the current workflow), and **what action
seam it goes through**.

## Considered options

- **What a rerun re-executes.**
  - **The workflow's *current* definition, looked up by the recorded run's `WorkflowID`
    (chosen).** A `RunRecord` (ADR-0022) stores each lane's settled *result* (agent,
    status, credits) but **not the step definitions** (the prompts, the `when`
    predicates, the models) — those live only in the forge `Workflow`. So a rerun
    compiles and launches the workflow as it exists *now* (`Forge.CompileWorkflow(id)`),
    exactly as the Workflows-page "Run" does. A rerun is a **re-execution, not a replay**:
    if the workflow was edited since, the new run uses the edited definition. Because the
    new run carries the **same `WorkflowID`**, its spend and run record roll up under the
    *same* per-workflow totals / aggregates / reconciliation (V13/V15) — a rerun is
    indistinguishable from any other run of that workflow, which is the coherent
    accounting.
  - *Replay the exact recorded run (its historical steps/prompts).* Rejected — the run
    record doesn't carry the step definitions (it would need a schema change to snapshot
    them), and replaying a stale definition is rarely what a user wants (they reran *this
    workflow*, expecting its current form). Snapshotting full step definitions on every
    run to enable replay is a large, speculative cost for a feature no one asked for.

- **What action seam the rerun goes through.**
  - **Reuse the existing run-trigger, extracted into one shared `launchWorkflow(id)`
    (chosen).** `handleWorkflowRun` already compiles a workflow by id, guards `s.busy`,
    installs a fresh `workflowRun`, and launches the lanes over the `copilot.Client` seam
    (`CreateSession`/`Send`). The action the orchestration surface needs **already
    exists** — rerun is a *second entry point* to it, not a new mechanism. So the
    launch body is extracted verbatim into `func (s *Server) launchWorkflow(id string)
    bool` (returns whether a run started); `handleWorkflowRun` and the new
    `handleRunRerun` both call it, differing only in which page they re-render when no run
    starts (Workflows vs. Runs). **No new seam to the runtime, no new orchestration
    path** — the same `forgeMu → s.mu` lock order, the same `!s.busy` guard, the same
    "land on the chat page where the lanes stream" on success.
  - *A bespoke rerun path that drives the runtime directly.* Rejected — it would
    duplicate the compile/guard/launch logic and create a second, divergent orchestration
    trigger. There is one way to start a run; rerun reuses it.

- **When the rerun control is offered.**
  - **Only when the workflow still exists (`CanRerun`, chosen).** `runRow` gates the
    button on `s.forge.Workflow(r.WorkflowID) != nil` (under the `forgeMu` the Runs
    render already holds) — a run whose workflow was since **renamed or deleted** (an
    orphan, like the demo's `review-and-fix`) shows no rerun control, because there is
    nothing to re-execute. A click that races a just-deleted workflow still fails safe:
    `launchWorkflow` returns `false` on a compile error and the handler re-renders the
    Runs page unchanged.
  - *Always show it and error on click.* Rejected — offering an action that can't work is
    worse UX than hiding it; the gate is a cheap forge lookup the render already has.

## Decision

Extract the run-launch body of `handleWorkflowRun` into `func (s *Server)
launchWorkflow(id string) bool`: it compiles the workflow under `forgeMu`, and (if not
`s.busy`) installs a fresh `workflowRun`, marks the server busy, starts the lanes, and
returns `true`; on a compile error or a busy server it makes **no state change** and
returns `false`. `handleWorkflowRun` becomes a thin caller that renders the chat page on
`true` and the Workflows page on `false`. A new `handleRunRerun` (route `POST
/runs/rerun/{workflow}`) calls the same `launchWorkflow` with the recorded `WorkflowID`,
rendering the chat page on `true` and the Runs page (at the request's `?window=`) on
`false`. `runRow` surfaces `WorkflowID`, a `CanRerun` gate (`forge.Workflow(id) != nil`),
and the active `window`; the `runRecord` template renders a `↻ rerun` button (a
**disjoint `rerun` marker class**) only when `CanRerun`. A rerun is a re-execution of the
workflow's current definition under the same `WorkflowID`, so it rolls up with every
other run of that workflow.

## Consequences

- Positive: the orchestration history surface gains its **first action** — a recorded run
  can be re-executed in place. Because rerun reuses the single `launchWorkflow` trigger,
  there is exactly one orchestration path (one busy-guard, one lock order, one seam), so
  the new entry point can't drift from the original. Spend/run roll-up stays coherent: a
  rerun is just another run of the same workflow (same `WorkflowID`), so V13/V15
  aggregates and reconciliation need no special case.
- Re-execution, not replay (the key semantic): a rerun runs the workflow's **current**
  definition. If the workflow was edited or deleted since the recorded run, the rerun
  reflects that (edited → new behavior; deleted → no rerun control). No run-record schema
  change — `RunRecord` already carries `WorkflowID`, which is the only key a rerun needs.
- Fail-safe gating: the control is hidden for an orphan run (`CanRerun` false), and a
  racing click on a just-deleted workflow returns `false` from `launchWorkflow` and
  re-renders the Runs page unchanged — no panic, no half-run.
- Busy-coherent: like the Workflows-page run, a rerun is refused (no state change) while a
  turn or another run is in flight (`!s.busy`), and lands the user on the chat page where
  the lanes stream on success.
- Escaping (ADR-0001 held): the rerun button's only dynamic value is the `WorkflowID` in
  the `hx-post` URL, emitted through the same `html/template` auto-escaping as the rest of
  the Runs surface.
- Selector disjointness (the V16/V17 e2e lesson): the new button's `rerun` class token is
  **disjoint** from the Workflows page's `button.run` and the Runs page's `a.export`, so it
  can't collide with an existing strict-mode `locator(...)` assertion.
- Contract change: `POST /runs/rerun/{workflow}` is a new route — recorded in CONTRACTS §3
  beside `POST /workflows/{id}/run`; no new reader/writer (§4 unchanged), no persisted
  schema change.
