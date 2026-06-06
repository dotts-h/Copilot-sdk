---
id: 0016
title: Cost attribution — per-agent / per-workflow / per-session rollups (roadmap v2, item A2)
status: closed
severity: high
group: 0013
github:
links:
  adr: ../adr/0018-additive-attribution-tags-on-spend-records.md
  prs: []
  issues: [0013, 0014]
  regression:
assets: []
---

## Summary

A `telemetry.SpendRecord` carried only *how much* (`USD`/`AIU`) and *which
session* (`SessionID`) — not *which agent persona* or *which workflow run* spent
it. Workflow runs already meter per-lane cost in memory (ADR-0013) but never
**persisted** the attribution. Tag each record additively with the active **agent
id** (and the **workflow id + lane index** when a run owns the turn), add pure
`AgentShares` / `WorkflowShares` aggregations (cousins of `ModelShares`), and
surface a "Cost by agent / workflow" breakdown on the Telemetry page. This is
where the two differentiators meet: **orchestration-aware cost**. Source:
`docs/NEXT_FEATURES.md` item A2; ADR-0018; builds on A1 (issue 0014).

## Repro
1. Run turns under different agents, and run a workflow.
2. Open the Telemetry page.
   - **Expected:** a breakdown of spend by the agent that incurred it and by the
     workflow that owned it, surviving a restart (it reads the persisted ledger).
   - **Actual (before):** only a per-model share existed; the ledger could not say
     which agent/workflow burned the budget, and a workflow run's per-lane cost
     died with the run.

## Resolution (shipped)

Built on `claude/next-features-research-8aBvS`:

- **Schema (`internal/telemetry/history.go`):** `SpendRecord` gains additive
  `agent`/`workflow`/`lane` fields (`omitempty`, schema **v2**). v1 files load
  unchanged (empty tags); v1 readers ignore the new keys and tolerate the higher
  version. CSV export appends `agent,workflow,lane` at the end (pre-v2 columns
  keep their positions).
- **Aggregations:** pure `AgentShares` (empty-agent bucket included — every turn
  has an agent; the empty one is the built-in chat) and `WorkflowShares`
  (non-workflow spend excluded — each workflow's share of *orchestrated* spend),
  both factored over a shared `shareBy`; `ModelShares` now routes through it too.
- **Web (`internal/web`):** `recordUsage` takes a `spendTag` — the chat reducer
  (`session.go` `EvUsage`) tags the active `Server.agentID`; the workflow-lane
  reducer (`workflow.go` `handleRunEvent`) tags the run id + lane agent + index.
  `Server.agentID` is seeded from `config.DefaultAgent` and kept current in
  `applyAgentSpec` (read under `s.mu`). `telemetryPartial` renders the "Cost by
  agent / workflow" breakdown (`spendShares`, ids → names under `forgeMu`).
- **Demo (`internal/bootstrap`):** `seedSpend` now tags its records with agents
  and a couple of workflow-owned turns, so the breakdown (and a new e2e
  assertion) renders offline.

Guarded by `internal/telemetry` `TestSpendRecordRoundTripsAttributionTags` /
`TestSpendStoreReadsV1RecordWithoutTags` / `TestAgentShares*` /
`TestWorkflowShares*` / `TestWriteCSVAppendsAttributionColumns`; `internal/web`
`TestUsageTagsActiveAgent` / `TestWorkflowUsageTagsWorkflowAndLane` /
`TestNewSessionSeedsActiveAgentFromConfig` / `TestTelemetryPageShowsAttributionBreakdown`.

## Notes

- **Decision:** ADR-0018 (additive agent/workflow attribution tags on spend
  records) — lead-with-a-decision (ADR-0004).
- **Watch:** the shared-demo-ledger gotcha — the e2e asserts the breakdown
  *structure* ("Cost by agent" / "Cost by workflow"), never exact figures.
- **Unblocks:** A3 (burn-rate forecast — attribution is now on the time series so
  a forecast can be sliced per agent/workflow) and B3 (workflow run history is now
  mostly a ledger query over the `workflow`/`lane` tags).
