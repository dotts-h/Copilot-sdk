# 0017. Per-lane tool + permission surface for parallel workflow lanes

- Status: accepted
- Date: 2026-06-06
- Deciders: Horia
- Related: extends [ADR-0013](0013-multi-agent-workflow-run-handoff-surface.md)
  (multi-agent workflow run / handoff surface); `internal/copilot`
  (`mock.go` — `CreateSession` distinct ids), `internal/web` (`workflow.go` —
  `lane` tool/permission state, `handleRunEvent`, `renderLanes`, `streamDemoLane`;
  `server.go` — `handlePerm` lane-awareness; `templates/fragments.html` `laneCard`;
  `static/app.css`), `internal/bootstrap` (`SeedForge` parallel demo workflow),
  `e2e/tests/e2e.spec.ts`; `docs/NEXT_FEATURES.md` item B1,
  [issue 0015](../issues/0015-real-parallel-workflow-lanes.md), TECH_DEBT #12,
  [ADR-0001](0001-render-markdown-server-side-for-committed-agent-turns.md),
  [ADR-0012](0012-diff-review-lane-for-file-write-permissions.md)

## Context

ADR-0013 shipped workflows as **lanes**: a sequential handoff or a parallel
fan-out, each step a sub-run on the seam (`CreateSession`+`Send`), attributed to a
lane by the event's `SessionID` with a sole-running-lane fallback. Two gaps were
explicitly deferred to TECH_DEBT #12:

1. **The parallel path was unobserved offline.** `MockClient.CreateSession`
   returned a single constant id (`"mock-session"`), so two concurrent lanes shared
   one id and `laneFor` could not disambiguate them. The demo/e2e could only drive
   the *sequential* path (one running lane → the empty-`SessionID` fallback routes
   to it); the parallel engine was unit-tested directly but never browser-driven.
2. **A lane was thin.** It surfaced only its streamed message + metered cost — a
   sub-run's tool timeline and inline permissions were dropped, so a parallel run
   (the orchestration payoff) was the least observable part of the product.

B1/issue 0015 is to make parallel lanes *real and observed*. Two questions: **how
to drive concurrent lanes offline**, and **how much of a sub-run to surface per
lane** (and, since a lane can now request a permission, **how permission answering
maps onto lanes**).

## Considered options

- **Driving concurrent lanes offline.**
  - **Distinct ids per `CreateSession` + `SessionID`-tagged demo events
    (chosen).** `MockClient.CreateSession` returns `mock-session-N` (incrementing),
    and `streamDemoLane(sid, …)` tags every emitted event with the lane's backing
    id. The existing `SessionID`-keyed `laneFor` then disambiguates concurrent
    lanes with no engine change — the seam already carries `SessionID` (CONTRACTS
    §2). A seeded **parallel** demo workflow (`parallel-review`) lets the demo/e2e
    finally drive the fan-out. This makes the existing routing *exercised*, not new.
  - *A parallel-aware mock that fakes per-session streams.* Rejected — the seam is
    one stream keyed by `SessionID`; faking more is over-engineering. The only thing
    missing was distinct ids.

- **How much of a sub-run to surface per lane.**
  - **A lane's own tool timeline + inline permissions, reusing the chat
    renderers (chosen).** `handleRunEvent` now reduces `EvToolStart/Progress/End`
    and `EvPermission` for the attributed lane; the lane owns a small
    `[]*convo.ToolView` (mirroring `convo.State`'s tool tracking) and a
    `[]PermissionRequest`. `renderLanes` composes each lane card from the **same**
    `renderToolCard` / `renderPermForm` the chat timeline uses, so a sub-run's tools
    and a file-write diff review render identically inside the lane. Reasoning and
    context events stay un-surfaced per lane (a lane is a compact strip, not a full
    transcript).
  - *Promote a full `convo.State` per lane.* Rejected — it pulls message/reasoning
    buffers a lane doesn't need (it tracks `text` already) and a much larger render;
    the lane only needs interleaved tools + pending permissions.

- **How a lane permission is answered.**
  - **Per-lane ownership over the shared `/perm/{id}` seam (chosen).** Each lane
    owns its pending permissions; there is **no** global FIFO across lanes (the
    question issue 0015 flagged). A lane permission renders an inline form posting to
    the existing `/perm/{id}` route; `handlePerm` is lane-aware — after `Respond`, it
    drops the request from whichever lane holds it and refreshes `#lanes`
    out-of-band (instead of the chat timeline). Concurrent lanes can thus hold and
    resolve independent permissions in any order, which is the natural model for a
    fan-out (each lane gates its own writes).
  - *A single cross-lane permission queue (FIFO).* Rejected — it serializes
    independent lanes' approvals for no benefit and muddles which lane a request
    belongs to; per-lane matches how a parallel run actually reasons about work.

## Decision

Give `MockClient.CreateSession` distinct ids (`mock-session-N`) and tag
`streamDemoLane`'s events with the lane's backing session id, so a **parallel** run
drives concurrent lanes offline through the unchanged `SessionID`-keyed `laneFor`
attribution. Seed a `parallel-review` demo workflow so the demo/e2e cover the
fan-out. Extend `handleRunEvent` to reduce a lane's `EvToolStart/Progress/End` and
`EvPermission` onto per-lane state; `renderLanes` surfaces each lane's own tool
timeline and inline permission form by reusing the chat `renderToolCard` /
`renderPermForm`. Make `handlePerm` lane-aware: a permission answered for a lane is
dropped from that lane and `#lanes` is refreshed out-of-band over the shared
`/perm/{id}` flow. Lane permissions are **per-lane**, not a cross-lane FIFO.

## Consequences

- Positive: the orchestration differentiator's parallel half is finally
  **observed** — a fan-out drives concurrent lanes offline (demo + a new e2e spec),
  each lane showing its own tool activity and answerable inline permissions, not
  just output + cost. No seam method was added; `SessionID` was already on `Event`.
  TECH_DEBT #12 is paid.
- Escaping (ADR-0001 held): lane tool args/results flow through the same `richtext`
  escaping as the chat timeline; a lane permission's diff is escaped by the
  `permReview` renderer (ADR-0012). The composed lane-card HTML is
  `trusted()`-wrapped only after each fragment self-escaped — guarded by
  `TestWorkflowLaneToolTextEscaped` (and the existing `TestWorkflowLanesEscapeModelText`).
- Attribution invariant held (REGRESSIONS "a workflow run owns the turn"):
  `handleRunEvent` keeps keying every event by `SessionID` (sole-running fallback
  only when the id is empty). With distinct ids, parallel lanes no longer rely on
  the fallback.
- Contract note: `MockClient.CreateSession` now returns a distinct id per call
  rather than the constant `"mock-session"` — a test/demo-only behavior change
  (CONTRACTS §1 notes the seam promise is unchanged: "open a session, return its
  id"). The `/perm/{id}` route is unchanged on the wire; it merely also resolves
  lane permissions (CONTRACTS §3).
- Trade-off accepted: a lane still does not surface its reasoning or context-window
  events (kept off the compact lane strip); the full per-sub-run transcript remains
  out of scope. Lane permissions don't block in demo mode (as in the chat demo).
