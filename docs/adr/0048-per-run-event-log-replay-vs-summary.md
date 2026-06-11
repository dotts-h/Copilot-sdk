# 0048. Per-run event log — replay semantics over an additive AppendOnlyStore[RunEvent]

- Status: accepted
- Date: 2026-06-11
- Deciders: Horia
- Related: issue 0085 (epic 0083), [ADR-0022](0022-workflow-run-history-sibling-append-only-run-store.md), [ADR-0009](0009-persisted-spend-history-append-only-ledger.md), `internal/telemetry/eventlog.go`, `internal/web/hub.go`, `internal/web/eventlog.go`

## Context

ADR-0022 added the **RunStore** (`runs.json`) — one record per completed run, capturing the
final outcome, per-lane status, and metered credits. That answers *"what was the
outcome?"* but not *"what happened, step by step?"* — the event sequence that led there.

Epic 0083 evaluated a full event-bus architecture for the orchestration layer. The
evaluation concluded that a bus is unnecessary and harmful: it introduces a new transport,
eventual consistency, ordering rework, and backpressure complexity for a system that already
has a reliable, sequential event stream (the Hub's `pump`). But the evaluation identified
**one genuine win** a bus would deliver: **replayability** — the ability to reconstruct a
run step-by-step for debugging and audit. This ADR captures the minimal, additive
slice that delivers that win without adopting a bus.

## Considered options

1. **Full event-bus architecture** — an in-process or distributed event bus, every `Ev*`
   event published to topics, subscribers replay from an offset.
   Rejected (epic 0083 charter): new transport, eventual consistency, ordering rework,
   backpressure complexity — the cost far outweighs the one genuine benefit (replayability).

2. **Extend RunRecord with a per-lane event list** — add `[]RunEvent` to the existing
   `RunRecord` shape on `runs.json`.
   Rejected: `runs.json` is a summary store (one record per run, immutable on completion);
   appending a growing event list into it violates the grain and breaks the stable
   `TestRunStoreOnDiskTagsAreStable` contract (the array key and version are the stable
   surface, and per-event records would bloat the file for the summary readers).

3. **Separate per-run event log on `AppendOnlyStore[RunEvent]`** — one file per run id,
   under `<configDir>/eventlogs/<runID>.json`, built on the same generic store that backs
   SpendStore and RunStore (H1, issue 0033). Fully additive: absent or disabled → the
   existing run/spend recording and SSE path are byte-identical.

## Decision

We chose **option 3**: a per-run event log built on `telemetry.AppendOnlyStore[RunEvent]`,
keyed by run id, fed from the Hub's `pump` alongside the existing SSE broadcast.

### Why additive, not in-band

The existing RunStore records the run **once on completion** (ADR-0022). The event log
records each normalized `Ev*` event **as it flows** — these are different grains and must
stay separate. Conflating them would violate the "one fact, one home" principle and bloat
the summary store. The event log is explicitly **optional**: an empty `EventLogDir` in
`web.Options` is the default; disabling it leaves every existing code path byte-identical.

### Why AppendOnlyStore[T], not a bus

`AppendOnlyStore[RunEvent]` gives us:
- Atomic temp-file + rename on every append — no corrupt partial files.
- Missing file = empty (no run yet), present-but-invalid = error, ephemeral when dir="".
- The same forward-compatibility envelope (`{"version":1,"events":[…]}`) every other store
  uses.
- Goroutine-safe `Append`/`Records`/`Count` — the same discipline as SpendStore/RunStore.

A bus would give us the same replayability at 10× the complexity. We take the simple path.

### Lock order

`appendRunEvent` is called from the Hub's `pump` **after** `sv.handleEvent(e)` and
`sv.broadcast(frags)` return — i.e., after `s.mu` has been released by the reducer.
It takes `s.mu` briefly to snapshot the run ID and get/create the log pointer, then
**releases `s.mu` before doing any IO**. The actual disk append runs in a background
goroutine — so the pump never blocks on IO and the critical section is never held during
disk writes. The `forgeMu → s.mu` lock order is respected: `appendRunEvent` never
acquires `forgeMu`.

### Idempotency

The event log is a pure append stream. A re-recorded run (via `rerun`) gets a new
`runID` (allocated by `launchWorkflow`), so its events flow into a new per-run file. A
double-call to `appendRunEvent` for the same event cannot happen: the pump's goroutine
is sequential (one event at a time). A background goroutine per Append is a
concurrent-append race mitigated by `AppendOnlyStore`'s internal mutex — the records
arrive in whichever order the goroutines serialize, not necessarily the pump order.
This is documented as an accepted trade-off: the log is for replay/audit, not for
strict sequencing (strict ordering would require synchronous IO on the pump's hot path).

### On-disk contract

The envelope is `{"version":1,"events":[…]}`; each `RunEvent` carries:
`at` (RFC3339 UTC), `runId`, `type` (stable string name, e.g. `"EvMessage"`),
`laneIndex` (omitted for lane 0), and type-specific payload fields (`text`, `tool`,
`args`, `result`, `success`, `err`). These tags are the stable contract, pinned by
`TestRunEventLogOnDiskTagsAreStable`.

## Consequences

- **Positive**: replayability delivered additively — no new transport, no ordering rework,
  no backpressure complexity. A run can be reconstructed step-by-step for debugging
  multi-lane workflows and for the cost⋈run reconciliation differentiator. The existing
  run/spend recording and SSE path are byte-identical when the log is disabled.
- **Negative / cost we accept**: goroutine-per-Append means the on-disk order may differ
  from the pump order (concurrent goroutines race to the store's internal mutex). For
  audit purposes this is acceptable — all events are present; strict temporal order can be
  recovered from the `at` timestamp. A future slice could use a per-run serialized append
  queue if strict ordering is required (TECH_DEBT candidate).
- **Relation to epic 0083**: this ADR records the *accepted* slice of the
  considered-and-rejected full event-bus evaluation. The bus provides ordering, fan-out,
  and back-pressure at the cost of a new transport layer. This slice takes the one genuine
  benefit — replayability — and delivers it purely additively over the existing pump.
