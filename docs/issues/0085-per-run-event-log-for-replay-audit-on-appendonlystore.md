---
id: 0085
title: "Per-run event log for replay/audit on AppendOnlyStore"
status: closed
severity: medium
group: 0083
depends_on: []
github: 141
links:
  adr: [0048]
  prs: []
  issues: [0083]
  regression:
assets: []
---

## Summary

Today only the **outcome** of a run is persisted (`RunStore` records the summarized
`RunRecord`, ADR-0022). Add an **optional append-only log of the normalized run events** —
the same `Ev*` stream the Hub's `pump` already routes (`internal/web/hub.go`) — beside the
existing store, built on the shipped generic `telemetry.AppendOnlyStore[T]`
(`internal/telemetry/store.go`). With it, a run can be reconstructed **step-by-step** (what
happened, in order), not just summarized — replay/audit for debugging multi-lane runs and
for the cost⋈run reconciliation differentiator.

## Why now

The second of the two "honest wins" from the event-driven-architecture evaluation (epic
0083). It delivers the single genuine benefit a full event log/store would give —
replayability — **without** adopting a bus: it is purely additive `Append` over the
`AppendOnlyStore[T]` discipline already in the codebase, fed from the existing event loop.
No new transport, no eventual consistency, no ordering rework.

## Touches

- `internal/telemetry` — a new `AppendOnlyStore[RunEvent]` (a versioned envelope + the same
  atomic temp-file+rename / missing=empty / invalid=error shape the other two stores get
  for free), keyed by run id.
- `internal/web/hub.go` — the `pump` loop (or the run reducer it feeds) is the **event
  source**: normalized `Ev*` events for an active run are appended as they flow, before/
  alongside the SSE broadcast.

## Acceptance

- [x] An `AppendOnlyStore`-backed per-run event log records the ordered normalized events of
      a run; reading it back yields the run's event sequence (round-trip + atomic-write +
      ephemeral/empty-dir behavior covered by tests, mirroring the existing store tests).
- [x] The log is **optional/additive** — disabling it (or an absent log dir) leaves the
      existing run/spend recording and SSE path byte-identical; no behavior change to the
      live surface.
- [x] Appending is off the hot path's critical section (no new lock-order risk; respects
      `forgeMu → s.mu`); idempotency preserved (a re-recorded run does not double-log).
- [x] On-disk JSON tags are pinned as a stable contract (a `…OnDiskTagsAreStable` test like
      the spend/run stores). `make lint && make test` (floor 65%) green.

## Notes

M-sized. Builds directly on `telemetry.AppendOnlyStore[T]` (epic 0030 / issue 0033) and the
`Ev*` event vocabulary (CONTRACTS §events). An ADR records the replay-vs-summary semantics
and that this is the *accepted* slice of the considered-and-rejected event-bus evaluation
(epic 0083 charter). Sibling: 0084 (worker pool) — independent seam, parallel-safe.

## Resolution (shipped)

Shipped in PR #149 (branch `feat/per-run-event-log`). Key files:
- `internal/telemetry/eventlog.go` — `RunEvent` type + `RunEventLog` store + `LoadRunEventLog` + `RunEventLogPath`
- `internal/telemetry/eventlog_test.go` — round-trip, atomic-write, ephemeral, corrupt, newer-schema, on-disk-tags-stable, per-run-isolation tests
- `internal/web/hub.go` — `EventLogDir` option + seeded to Server + pump wired to `appendRunEvent`
- `internal/web/eventlog.go` — `appendRunEvent` off the critical section, `normalizeRunEvent`, `eventTypeName`
- `internal/web/eventlog_test.go` — disabled-leaves-behaviour-identical, appends-run-events, no-active-run, no-subagent-events tests
- `docs/adr/0048-per-run-event-log-replay-vs-summary.md` — ADR-0048
</content>
