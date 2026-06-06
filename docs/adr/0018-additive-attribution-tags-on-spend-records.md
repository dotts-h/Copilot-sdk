# 0018. Additive agent/workflow attribution tags on spend records

- Status: accepted
- Date: 2026-06-06
- Deciders: Horia
- Related: builds on [ADR-0009](0009-persisted-spend-history-append-only-ledger.md)
  (append-only ledger) and [ADR-0016](0016-ledger-is-source-of-truth-for-account-wide-budget-accounting.md)
  (ledger is the account-wide source of truth); pairs with
  [ADR-0013](0013-multi-agent-workflow-run-handoff-surface.md) /
  [ADR-0017](0017-per-lane-tool-and-permission-surface-for-parallel-workflow-lanes.md)
  (workflow runs meter per-lane cost). Touches `internal/telemetry`
  (`history.go` — `SpendRecord` `+agent`/`+workflow`/`+lane`, `AgentShares`,
  `WorkflowShares`, `shareBy`, `WriteCSV` columns, `SpendSchemaVersion`),
  `internal/web` (`session.go` — `spendTag` + `recordUsage`; `workflow.go`
  `handleRunEvent`; `server.go`/`commands.go`/`hub.go` — the active-agent id;
  `pages.go` — `spendShares`; `templates/fragments.html`),
  `internal/bootstrap` (`seedSpend` demo attribution); `docs/NEXT_FEATURES.md`
  item A2, [issue 0016](../issues/0016-cost-attribution-rollups.md)

## Context

The ledger (`SpendStore`) records every assistant turn with `At` + `SessionID`
and survives a restart, and ADR-0016 made it the source of truth for the
account-wide "this month" rows. But a `SpendRecord` answered only *how much* and
*which session* — not *which agent persona* or *which workflow run* burned the
budget. This is where the product's two differentiators meet: **cost** that is
also **orchestration-aware**. Workflow runs already meter per-lane cost in memory
(ADR-0013), but that attribution was never *persisted* — it died with the run.

The question: how to record the attribution without breaking the on-disk contract
(CONTRACTS §4: the `records` array is the stable surface; bumps add fields only,
older readers ignore unknown keys), and where the "active agent id" for a chat
turn comes from.

## Considered options

- **Schema shape for attribution.**
  - **Additive optional fields on `SpendRecord` (chosen):** `agent` (the active
    persona id), `workflow` (the run id when a run owned the turn), and `lane`
    (the lane index within that run), all `omitempty`. The on-disk version bumps
    `1 → 2`, but a v1 reader ignores the new keys and tolerates the higher version
    (already guarded by `TestLoadSpendStoreToleratesNewerSchema`), and a v1 record
    reads back into v2 code with empty/zero tags. No migration, no rename — the
    least-surprise extension of an append-only ledger.
  - *A separate attribution side-table keyed by record.* Rejected — two files to
    keep atomic and in sync for data that is 1:1 with a record; the additive field
    is simpler and keeps the CSV export a single self-describing table.

- **Where the chat turn's agent id comes from.**
  - **A live `Server.agentID`, guarded by `s.mu` (chosen):** seeded from
    `config.DefaultAgent` at session creation and updated in `applyAgentSpec` (the
    single point where an `/agent` switch or the Agents-page select restarts the
    session). `recordUsage` reads it under the `s.mu` it already holds.
  - *Read `config.DefaultAgent` directly in `recordUsage`.* Rejected — `config`
    is shared and mutated under `forgeMu`; `recordUsage` runs under `s.mu`, and the
    established lock order is `forgeMu → s.mu` (e.g. `handleAgentSelect`). Reading
    config there would either race (`-race` would catch it) or invert the lock
    order. A mirrored `s.mu`-guarded field sidesteps both.

- **The aggregation.**
  - **A shared `shareBy` helper behind typed `AgentShares`/`WorkflowShares`
    (chosen):** cousins of `ModelShares`, all three now route through one pure
    grouping function (keyed by a field selector, with an `includeEmpty` flag).
    `AgentShares` includes the empty-agent bucket (every turn has an agent — the
    empty one is the built-in chat, labelled at the UI edge); `WorkflowShares`
    **excludes** non-workflow turns so the per-workflow view shows each workflow's
    share of *orchestrated* spend, not a bucket of chat noise.
  - *Duplicate the `ModelShares` body three times.* Rejected — the only difference
    is the key and the empty-bucket policy; one helper removes the copy-paste.

## Decision

Tag each `SpendRecord` additively with `agent`, `workflow`, and `lane`
(`omitempty`, schema v2; v1 records read back with empty tags, v1 readers ignore
the new keys). The chat reducer attributes a turn to `Server.agentID` (seeded from
`config.DefaultAgent`, kept current in `applyAgentSpec`, read under `s.mu`); the
workflow-lane reducer (`handleRunEvent`) attributes the turn to the run id and the
lane's agent + index — both flow through the one shared `recordUsage` via a
`spendTag`. Add pure `AgentShares` (empty-agent bucket included) and
`WorkflowShares` (non-workflow spend excluded) aggregations alongside
`ModelShares`, factored over a shared `shareBy`. Surface a "Cost by agent" and
"Cost by workflow" breakdown on the Telemetry page (ids resolved to names under
`forgeMu`, falling back to the raw id). Append `agent,workflow,lane` to the CSV
export header (at the end, so pre-v2 column positions are unchanged).

## Consequences

- Positive: the meter now answers *"which agent / which workflow is burning my
  budget?"* across time, and the attribution **survives a restart** — the
  orchestration-aware half of the cost differentiator. The chat and workflow paths
  share one `recordUsage`, so a turn lands in all three sources (account meter,
  session meter, ledger — REGRESSIONS "three sources") *and* carries its tags.
- `telemetry` stays pure and dependency-free; the aggregations are deterministic
  (ties broken by key) and unit-tested, including the v1 backward-read.
- Backward-compatible on disk and in the CSV: older spend files load unchanged
  (empty tags), and the export's pre-existing columns keep their positions.
- Lock-order safe: `Server.agentID` is mirrored under `s.mu`, so `recordUsage`
  never reaches for `forgeMu` while holding `s.mu`. The Telemetry breakdown
  resolves names under `forgeMu` from the page-render path, which (like every
  partial) runs without `s.mu`.
- Trade-off accepted: a chat turn taken with no agent persona records an empty
  `agent` (labelled "chat (built-in)" at the UI), and a `WorkflowShare` fraction is
  relative to workflow-attributed spend, not the grand total — documented so the
  two breakdowns aren't misread as shares of the same denominator. Changing
  `config.DefaultAgent` via Settings without restarting the live session does not
  retag in-flight attribution until the next session restart (consistent with how
  Settings already defers persona changes).
