---
id: 0084
title: "Bounded lane worker pool — backpressure for wide parallel workflows"
status: open
severity: medium
group: 0083
depends_on: []
github: 140
links:
  adr:
  prs: []
  issues: [0083]
  regression:
assets: []
---

## Summary

`launchLanes` (`internal/web/workflow.go`, ~L543) spins **one goroutine per launchable
lane** with no upper bound — `startLane` does a blocking `client.CreateSession` + `Send`
over the seam for each. A wide workflow (many lanes becoming runnable at once) fans out an
unbounded number of concurrent live sessions. Bound it with a **worker pool**: a fixed
number of workers drain a lane queue, so concurrency is capped and excess lanes wait their
turn (backpressure) instead of all launching at once.

## Why now

This is one of the two "honest wins" from the event-driven-architecture evaluation (epic
0083): the *only* place the current design fans out unbounded work. It is a small, local
guard that respects the existing seam — the pure run engine still decides *which* lane
indices to launch (`evalPending`); only the concurrency **adapter** changes, so the state
machine and its zero-client tests are untouched.

## Touches

- `internal/web/workflow.go` — `launchLanes` (the goroutine fan-out) → a bounded worker
  pool draining a lane queue; `startLane` unchanged. The pure `workflowRun`/`evalPending`
  state machine is **not** touched.

## Acceptance

- [ ] `launchLanes` caps concurrent in-flight lanes at a fixed worker count; lanes beyond
      the cap queue and start as workers free up (no lane dropped, ordering of *results*
      still driven by `finishLane`/`evalPending`).
- [ ] The pure run engine (`workflowRun` and friends) is unchanged — its existing
      zero-client unit tests pass byte-identically.
- [ ] A test drives a workflow with more launchable lanes than the worker count and asserts
      concurrency never exceeds the cap (e.g. a counting/stub client), and that all lanes
      still complete. `make lint && make test` (floor 65%) green.
- [ ] No new seam/dependency; the lock order (`forgeMu → s.mu`) and idempotency
      (`run.recorded`) invariants are preserved.

## Notes

S-sized; likely no ADR (no new seam — a concurrency bound on an existing adapter). The
considered-and-rejected event-bus rationale lives in the epic 0083 charter. Sibling: 0085
(per-run event log) — independent seam, parallel-safe.
</content>
