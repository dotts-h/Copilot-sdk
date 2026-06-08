# 0024. Abort an in-flight run — settle it as failed and abort its lane sessions

- Status: accepted
- Date: 2026-06-08
- Deciders: Horia
- Related: the **second action** on the orchestration surface and the **dual of rerun**
  ([ADR-0023](0023-rerun-a-recorded-run-re-executes-the-current-workflow-definition.md)),
  completing the basic interactive control set (start → rerun → stop). Reuses the
  `copilot.Client` abort seam already used by the chat-turn abort
  ([handleAbort], ADR-0013's lifecycle) and the per-lane run state machine of
  [ADR-0017](0017-per-lane-tool-and-permission-surface-for-parallel-workflow-lanes.md) /
  [ADR-0013](0013-multi-agent-workflow-run-handoff-surface.md); the settled run records
  through the existing run store of
  [ADR-0022](0022-workflow-run-history-sibling-append-only-run-store.md). Touches
  `internal/web` (`workflow.go` — `workflowRun.abort`, `handleRunAbort`/`abortRun`, and an
  idempotency guard on `runFrags`; `hub.go` — the `POST /run/abort` route;
  `templates/fragments.html` — the `stop-run` control; `static/app.css`),
  `docs/CONTEXT.md` (the **abort** term), CONTRACTS §3,
  [issue 0044](../issues/0044-abort-in-flight-run.md), epic
  [0042](../issues/0042-epic-interactive-orchestration.md).

## Context

Roadmap v8 (epic 0042) makes the orchestration surface **interactive**. Its first child,
**rerun** (V18/ADR-0023), added the first action — *start again* — from a recorded run. A
run can also go wrong **while it is in flight**: a cost runaway, a lane heading down the
wrong path, a workflow the user wants to stop. Before this child the only stop control was
the chat-turn abort (`POST /abort`, `handleAbort`), which aborts the **chat session**
(`s.sessionID`) — it does **not** touch a multi-lane workflow run (`s.run`). So a running
workflow could not be stopped at all; it had to be ridden to completion.

This child adds the **dual of rerun**: a **stop** control on the Chat lanes panel that
aborts the in-flight run, completing the basic interactive control set (start → rerun →
stop). Aborting a *multi-lane* run raises three questions an ADR must settle: **what an
aborted run becomes** (its lanes' and the run's terminal state), **what seam stops it**,
and **how the settle stays single** given that lane goroutines may still be in flight when
the user clicks stop.

## Considered options

- **What an aborted run becomes.**
  - **A failed run; its not-yet-settled lanes settle as `failed` with an "aborted" detail
    (chosen).** A `RunLane` carries a status string ∈ {done, failed, skipped} and the run
    an outcome ∈ {finished, failed} (CONTEXT). An aborted run is, for every reader
    (the Runs page, the aggregates, the ledger⋈runs reconciliation), a run that **did not
    finish** — i.e. a **failed** run. Reusing `failed` means **no new terminal status, no
    schema change, no special case** in any downstream reader: an aborted run rolls up
    under the same failure-rate / reconciliation math as any other failed run. The
    user-initiated nuance is carried where it belongs — in the lane's **detail** (`⏹
    aborted`) and the `⏹ stop run` affordance — not in a new enum value the whole pipeline
    would have to learn.
  - *A new `aborted` lane status / run outcome.* Rejected — it is a schema + glyph +
    status-name + reconciliation-semantics change (does an abort count toward failure
    rate? almost certainly yes) for a distinction that is presentational. Reversing the
    "reuse failed" decision later is cheap; introducing a fourth terminal status now is
    speculative surface no reader asked for.

- **What seam stops the run.**
  - **The existing `copilot.Client.Abort(ctx, sessionID)`, per running lane (chosen).**
    Each lane runs on its own backing session (`l.sessionID`); the seam already exposes
    `Abort` (used by `handleAbort` for the chat turn, and recorded by `MockClient`). So
    aborting a run is *aborting each running lane's session* over the **same seam** — no
    new runtime entry point. `abortRun` collects the running lanes' session ids under
    `s.mu`, settles the run, then calls `client.Abort` for each **outside** the lock (the
    seam call may block), exactly as `handleAbort` does for the single chat session.
  - *A bespoke run-cancellation path into the runtime.* Rejected — it would duplicate the
    lifecycle the chat abort already drives and add a second way to stop work. There is one
    abort seam; the run abort fans it out per lane.

- **How the settle stays single (the concurrency decision).**
  - **An idempotency guard (`run.recorded`) on the one completion path (chosen).** The run
    completion path — `runFrags(run, done=true)`, which clears `s.busy`, records the run,
    and notes the outcome — was previously "reached exactly once" only because the event
    reducer stops routing to a `done` run (`session.go`'s `!s.run.done` guard). But
    **`laneError` bypasses that guard**: it is called *directly* from a `startLane`
    goroutine on a `CreateSession`/`Send` error — including an error **caused by** the
    abort itself (we just aborted that lane's session). So after `abortRun` settles and
    records the run, a still-in-flight lane that errors would re-enter `runFrags` and
    **record the run a second time** (and re-note the outcome, re-clear busy). The fix is
    a `run.recorded` flag set on the first terminal pass; a second `runFrags(done=true)`
    is then a no-op that only re-renders the (already-settled) lanes. This makes the single
    completion point genuinely single regardless of which path (abort, the reducer, or a
    late lane error) reaches it.
  - *Cancel the lane goroutines' context so they can't re-enter.* Rejected as the primary
    mechanism — it is a larger change (threading a per-run cancel context through
    `startLane`) and still races (a goroutine past its cancel check). The idempotency guard
    is the smaller, total fix; context cancellation is unnecessary once the completion path
    is idempotent.

## Decision

Add `func (r *workflowRun) abort() []string`: it marks every not-yet-settled lane (running
or pending) `failed` with an `⏹ aborted` detail, flips the run `done` + `failed`, and
returns the backing session ids of the lanes that were still running. Add `handleRunAbort`
(route `POST /run/abort`) → `abortRun(ctx)`: under `s.mu` it no-ops when no run is in flight
(`s.run == nil || s.run.done`), else calls `run.abort()`, then the shared `runFrags(run,
true)` completion path (records the run once, clears `s.busy`), broadcasts the terminal
fragments, and — **outside** the lock — calls `client.Abort` for each running lane's
session; the handler re-renders the chat page. Guard `runFrags` with a `run.recorded` flag
so the completion path (record + busy-clear + outcome note) runs **exactly once** even if an
abort and a late lane error both reach it. The Chat lanes panel renders a `⏹ stop run`
button (a **disjoint `stop-run` marker class**) only while the run is `Running`. An aborted
run is a **failed** run — no new lane status, no schema change.

## Consequences

- Positive: the interactive control set is complete — a run can be **started** (Workflows
  page), **rerun** (Runs page, V18), and now **stopped** in flight (Chat lanes panel). The
  stop reuses the existing abort seam (no new runtime path) and the existing run-record
  path (no schema change), so an aborted run is just a failed run everywhere downstream
  (aggregates, reconciliation, CSV export) with zero special-casing.
- Reuse-failed (the key semantic): a stopped run records `outcome=failed` with its
  in-flight lanes `status=failed` (`detail=⏹ aborted`). The user-initiated nuance lives in
  the detail + the affordance, not a new enum. If a first-class "aborted" distinction is
  ever wanted, it is an additive change to the status/outcome vocabulary — this ADR
  deliberately defers it as speculative.
- Single completion (the concurrency fix): `runFrags` is now **idempotent** on the
  terminal pass (`run.recorded`), closing a real double-record/double-outcome race that
  abort introduces because `laneError` is called from goroutines that bypass the reducer's
  `!s.run.done` guard. The benefit also hardens the pre-existing completion path against
  any future second caller.
- Busy-coherent: aborting clears `s.busy` (via `runFrags`), so the server is immediately
  free for the next turn/run, like a normal completion. A racing double-click is a no-op
  (the second `abortRun` sees `run.done`).
- Fail-safe: a stop with no run in flight aborts no session and changes no state (the chat
  page re-renders); the `stop-run` control is shown only while `Running`.
- Escaping (ADR-0001 held): the `stop-run` button is static markup (a fixed `hx-post`
  URL); no dynamic value is interpolated.
- Selector disjointness (the V16/V17/V18 e2e lesson): the `stop-run` class token is
  **disjoint** from the chat-turn `.abort` (the budget hard-cap / type-ahead selector),
  the Workflows-page `button.run`, the Runs-page `button.rerun`, and the `a.export` links,
  so it can't collide with an existing strict-mode `locator(...)` assertion.
- Contract change: `POST /run/abort` is a new route — recorded in CONTRACTS §3 beside
  `POST /runs/rerun/{workflow}`; no new reader/writer (§4 unchanged), no persisted schema
  change. The `copilot.Client.Abort` seam is unchanged — it is now also driven per-lane for
  a run, not only for the chat session.
