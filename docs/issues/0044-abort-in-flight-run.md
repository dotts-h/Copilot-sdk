---
id: 0044
title: "Abort an in-flight run from the Chat lanes panel — the dual of rerun (roadmap v8, item V19)"
status: closed
severity: medium
group: 0042
github:
links:
  adr: [0024]
  prs: [77]
  issues: [0042]
  regression:
---

## Summary

After V18 (rerun, 0043, ADR-0023) the orchestration surface gained its **first action** —
*start again* from a recorded run. But a run can also go wrong **while it is in flight**: a
cost runaway, a lane heading the wrong way, a workflow the user wants to stop. The only stop
control was the **chat-turn** abort (`POST /abort`, `handleAbort`), which aborts the chat
session — it does **not** touch a multi-lane workflow run (`s.run`). So a running workflow
could not be stopped at all.

**V19 adds the dual of rerun**: a `⏹ stop run` control on the **Chat lanes panel** that
**stops the in-flight run**, completing the basic interactive control set (start → rerun →
stop). It is the **second child** of epic [0042](0042-epic-interactive-orchestration.md)
(roadmap v8 — interactive orchestration). Unlike the v7 reader children, this is an
**action with side effects**, so it takes **ADR-0024** for the abort semantics, the seam
reuse, and the completion-idempotency decision. Source: `docs/NEXT_FEATURES.md` "roadmap v8"
section ("v8 update (after V19)").

> A **new `aborted` terminal status** was weighed and **dropped**: for every reader (Runs
> page, aggregates, ledger⋈runs reconciliation) an aborted run is a run that **did not
> finish** — i.e. a **failed** run. Reusing `failed` means no schema change and no special
> case anywhere downstream; the user-initiated nuance lives in the lane **detail** (`⏹
> aborted`) and the affordance, recorded in ADR-0024.

## Repro
1. Start a workflow run (Workflows page) and watch the lanes stream on the Chat page.
2. Realize it's going wrong (cost runaway / wrong path) and try to stop it from there.
   - **Expected:** a "stop run" control on the lanes panel that aborts the in-flight run
     (its running lanes' sessions aborted, the run settled and recorded, the server freed).
   - **Actual (before V19):** no stop control for a run — the chat-turn abort (`/abort`)
     stops the chat session, not the workflow run; the run must be ridden to completion.

## Proposed resolution

- **`internal/web` (`workflow.go`):** add `func (r *workflowRun) abort() []string` — mark
  every not-yet-settled lane (running or pending) `failed` with an `⏹ aborted` detail, flip
  the run `done`+`failed`, and return the still-running lanes' backing session ids. Add
  `handleRunAbort` (`POST /run/abort`) → `abortRun(ctx)`: under `s.mu` no-op when no run is
  in flight (`s.run == nil || s.run.done`), else `run.abort()`, then the shared
  `runFrags(run, true)` (records once, clears `s.busy`), broadcast the terminal fragments,
  and — **outside** the lock — `client.Abort` for each running lane's session. Guard
  `runFrags` with a `run.recorded` flag so the completion path runs **exactly once** even
  when a late `laneError` (which bypasses the reducer's `!s.run.done` guard) re-enters it.
- **`internal/web/templates/fragments.html` (`workflowLanes`):** a `⏹ stop run` button in
  the run head, rendered only while `Running`, with a **DISJOINT `stop-run` marker class**
  (≠ the chat `.abort`, the Workflows `button.run`, the Runs `button.rerun`, the
  `a.export` links).
- **`internal/web/static/app.css`:** a small `.run-head .stop-run` button style.
- **`internal/web/hub.go`:** wire `POST /run/abort`.
- **ADR-0024** (written first, ADR-0004): an aborted run is a **failed** run (reuse, no new
  status); the abort fans the existing `copilot.Client.Abort` seam per running lane (no new
  runtime path); the single completion path is made **idempotent** (`run.recorded`).
  CONTRACTS §3 gains the route + the seam note; CONTEXT gains the **abort** term.

## Tests (failing-first)

- **`internal/web` `TestRunAbortStopsInFlightRun`:** a POST to `/run/abort` on an in-flight
  parallel run settles it (`!s.busy`, `run.done && run.failed`, both lanes `laneFailed`),
  aborts both lanes' sessions (`len(mock.Aborted) == 2`), records the run **once** with a
  `failed` outcome, and re-renders the chat page.
- **`internal/web` `TestRunAbortThenLateLaneErrorRecordsOnce`:** after an abort settles +
  records the run, a still-in-flight lane that errors (via `laneError`, bypassing the
  reducer guard) must **not** double-record — the idempotency proof (`run.recorded`).
- **`internal/web` `TestRunAbortNoRunIsNoop`:** a POST with no run in flight aborts no
  session and changes no state (fail-safe gate).
- **`internal/web` `TestLanesPanelShowsStopForRunningRunHidesWhenDone`:** the lanes panel
  shows a `stop-run` control (posting to `/run/abort`) while running, and **none** when the
  run is done.
- **e2e (`e2e.spec.ts`):** the `stop-run` marker class is verified **DISJOINT** from
  `.abort` / `button.run` / `button.rerun` / `a.export` (no strict-mode collision). A
  mid-flight `button.stop-run` *visibility* assertion was **tried and dropped**: the parallel
  **demo** run settles quickly (it does not block on the surfaced permission), so the
  `Running`-only control is not reliably observable mid-flight — the deterministic Go render
  test below is the spec for the button's render logic instead.

## Resolution (shipped)

Shipped in **PR #77**. Built as specified. The orchestration surface gained its second
action and the **dual of rerun**: a `⏹ stop run` control on the Chat lanes panel stops an
in-flight run. `workflowRun.abort()` marks every not-yet-settled lane `failed` (detail `⏹
aborted`), flips the run done+failed, and returns the running lanes' session ids;
`handleRunAbort` → `abortRun` runs the shared `runFrags(run, true)` completion path (records
the run once, clears `s.busy`), broadcasts the terminal fragments, and aborts each running
lane's session over the existing `copilot.Client.Abort` seam (outside the lock, like
`handleAbort`). An aborted run is a **failed** run — no new lane status, no schema change —
so it rolls up under the same aggregates / reconciliation as any failed run. A stop with no
run in flight is a no-op (the chat page re-renders), so a racing double-click can't
double-settle.

The self-review (`/code-review`, high effort) + a concurrency finder caught a **real
double-record race**: `laneError` is called directly from a `startLane` goroutine on a
`Send`/`CreateSession` error — including an error **caused by** the abort aborting that
lane's session — and it **bypasses** the reducer's `!s.run.done` guard, so it re-enters
`runFrags(run, true)` after the abort already recorded the run, recording it **twice** (and
re-noting the outcome, re-clearing busy). Fixed by making the single completion path
idempotent: a `run.recorded` flag set on the first terminal pass, with a deterministic
regression test (`TestRunAbortThenLateLaneErrorRecordsOnce`).

Tests (failing-first): `TestRunAbortStopsInFlightRun`, `TestRunAbortThenLateLaneErrorRecordsOnce`,
`TestRunAbortNoRunIsNoop`, `TestLanesPanelShowsStopForRunningRunHidesWhenDone` (the
render-logic spec — shows the control while running, hides it when done). The `stop-run`
class is kept disjoint from `.abort` / `button.run` / `button.rerun` / `a.export`; a
mid-flight e2e *visibility* assertion was tried and dropped (the parallel demo run settles
too quickly to observe the `Running`-only control reliably — CI flagged it on first run).
The existing telemetry + web + bootstrap + e2e tests stayed green unchanged. Gates green
(`make lint && make test` with `-race`; web 89.6%, telemetry 96.0%).

Docs: NEXT_FEATURES "roadmap v8" section ("v8 update (after V19)"), CONTRACTS §3 (the new
route) + §1 (the `Abort` seam note), CONTEXT (the **abort** term), ADR-0024 (the abort
semantics + seam reuse + completion-idempotency). No REGRESSIONS entry — the double-record
race was caught and fixed in-branch, never shipped. On merge, epic 0042 records the PR and
is re-evaluated.

## Notes
- **ADR-0024:** the second **action** child of epic 0042 — the dual of rerun. The decision
  is threefold: reuse `failed` (no new terminal status), reuse the `Abort` seam per-lane (no
  new runtime path), and make `runFrags` idempotent (`run.recorded`) against the
  guard-bypassing `laneError` path. Captured in CONTRACTS §3 (the route) + §1 (the seam) and
  CONTEXT (the **abort** term).
- **Differentiator:** completes the basic interactive control set (start → rerun → stop) —
  the v8 theme. **Second child** of epic 0042; on merge the epic records the PR and is
  re-ranked from a fresh value×fit pass (V20 per-lane rerun, or the pivot to B's
  cost-anomaly surface if the interactive surface is judged exhausted).
- **Numbering:** issue **0044** (next free after 0043), the second build of epic **0042**.
  **ADR-0024 consumed** (the second action seam; highest ADR becomes 0024).
