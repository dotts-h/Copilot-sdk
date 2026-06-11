---
id: 0083
title: "Epic: Orchestration robustness — backpressure + replayability (considered-and-rejected event bus)"
status: closed
severity: medium
group:
depends_on: []
github: 139
links:
  adr: [0048]
  prs: [148, 149]
  issues: [0084, 0085]
  regression:
assets: []
---

## Charter

An evaluation of the orchestration core asked whether a **web-style event-driven
architecture** (a pub/sub event bus, a message queue with at-least-once delivery, reactive
streams with backpressure between in-process components) would benefit the project. The
answer was **no** — but the evaluation surfaced two small, local wins worth landing, and
this epic carries them.

Today the project already runs the *right amount* of event-driven for its scale: one
normalized SDK event channel (`copilot.Client.Events()`) pumped through a single fan-in/
fan-out loop (`internal/web/hub.go` `pump`, routed by `SessionID` to the owning per-cookie
`Server`), a **pure, synchronous, deterministic** run engine (`internal/web/workflow.go`
`workflowRun` — `start`/`advance`/`finishLane`/`failLane`/`evalPending`, no IO, unit-testable
with zero client), goroutine-per-lane fan-out (`launchLanes`), and one-shot **channel
bridges** for human-in-the-loop (`internal/copilot/bridge.go` + the pause ledger
`internal/pause/pause.go`).

### Why the event bus was rejected (the "why these two and not a bus")

- **No scale driver.** This is a single local user, one in-memory session per cookie,
  in-process (even the desktop shell is loopback). There is no network boundary, no
  multi-node fan-out, and no throughput problem a queue solves.
- **It would undermine the project's biggest strength.** The run engine's purity and
  determinism are the source of its test rigor; a bus adds eventual consistency, ordering
  concerns, and non-determinism, and would scatter the currently *local and explicit*
  idempotency (`run.recorded`; the ledger's single `deliver`) across queue handlers.
- **The egress is already webhook-shaped** (SSE). Pushing that pattern *inward* between
  in-process components re-solves, with more moving parts, what one Go channel + the pure
  state machine + the channel bridges already solve cleanly.

The full bus/queue is therefore **considered and rejected**; if a future change ever wants
it, an ADR records the rejection rather than a roadmap item reopening it.

## Children (the two honest wins)

- [ ] **Bounded lane worker pool** ([0084](0084-bounded-lane-worker-pool-backpressure-for-wide-parallel-workflows.md), S) —
      `launchLanes` (`workflow.go`) fans out **unbounded** goroutines; bound it with a worker
      pool so wide parallel workflows get backpressure. Local, no new seam.
- [ ] **Per-run event log for replay/audit** ([0085](0085-per-run-event-log-for-replay-audit-on-appendonlystore.md), M) —
      an optional append-only log of normalized run events beside the existing `RunStore`,
      built on the shipped `telemetry.AppendOnlyStore[T]`, so a run can be reconstructed
      step-by-step (not just summarized). Fits the cost⋈run reconciliation differentiator.

## Acceptance (epic)

- [ ] Both children land respecting the existing seams (pure-core/thin-edges), each
      test-first, `make lint && make test` (floor 65%) green, born in its own PR.
- [ ] The bus rejection is recorded as an ADR (considered-and-rejected) the first child
      that touches the orchestration seam references, so the decision has one home.
- [ ] No new runtime dependency, no new transport; the worker pool and the event log are
      additive over the channel + `AppendOnlyStore` machinery already present.

## Sequencing

0084 (worker pool) and 0085 (event log) are **independent** — disjoint seams
(`workflow.go` concurrency adapter vs. a new telemetry store fed from `hub.go`) — so they
can run as parallel lanes. Neither blocks the other.

## Notes

Source: the event-driven-architecture evaluation (parallel research pass, 2026-06-11),
which mapped the current model (single event channel + pure state machine + channel
bridges) and weighed a pub/sub bus against this project's actual single-user, in-process
scale. Builds on `telemetry.AppendOnlyStore[T]` (epic 0030 / issue 0033) and the
orchestration seams from epics 0042 (interactive orchestration) and the v6/v7 cost⋈run work.

## Resolution (closed 2026-06-11)

Both children shipped to `main`; the epic is fully delivered. The event bus stayed rejected —
each win is a small, local guard on an existing seam, leaving the pure run engine's determinism
intact (its zero-client tests pass byte-identically).

- **0084 — bounded lane worker pool** (PR #148): `launchLanes` now drains a lane queue with a
  fixed worker pool, capping concurrent in-flight sessions; backpressure for wide parallel
  workflows. No new seam, no ADR.
- **0085 — per-run event log** (PR #149, ADR-0048): an optional `AppendOnlyStore[RunEvent]`
  fed from `hub.go`'s pump gives step-by-step replay/audit; disabling it leaves the live
  record/SSE path byte-identical.
</content>
