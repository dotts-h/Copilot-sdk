---
id: 0021
title: Workflow run history (roadmap v2, item B3)
status: closed
severity: medium
group: 0013
github:
links:
  adr: ../adr/0022-workflow-run-history-sibling-append-only-run-store.md
  prs: []
  issues: [0013]
  regression: ../REGRESSIONS.md
assets: []
---

## Summary

A workflow run (ADR-0013/0017/0021) is the product's unit of orchestration — a set of
lanes (sequential handoff, parallel fan-out, or branched with skips) driven to
completion. But a run is **ephemeral**: once it finishes, `runFrags` clears `busy` and
the next run overwrites it. There is no record of *what ran, with which agents, to what
outcome, at what cost*. A2 (ADR-0018) tags spend records with `workflow`/`lane`, but a
ledger row is "one metered turn" — it can't express a run's start/finish/outcome, nor a
**skipped branch that incurred no cost** (no turn → no record). B3 persists each run and
adds a "Runs" view. Source: `docs/NEXT_FEATURES.md` item B3; needs an ADR for the
run-store schema.

## Repro
1. Run a workflow, then restart the app and look for its history.
   - **Expected:** the completed run (its lanes, outcomes incl. skips, per-lane cost,
     timestamps) is listed on a Runs page.
   - **Actual (pre-B3):** runs are in-memory only; nothing survives the run finishing,
     let alone a restart. A skipped branch leaves no trace at all.

## Resolution (shipped — ADR-0022)

- **Sibling run store (`internal/telemetry/runs.go`):** a new append-only
  `telemetry.RunStore` at `<configDir>/runs.json` — versioned envelope
  `{"version":1,"runs":[…]}`, atomic temp-file+rename, missing=empty,
  invalid=error, empty-dir=ephemeral — mirroring `SpendStore`'s discipline. Each
  `RunRecord` is `{id, workflow, name, mode, startedAt, finishedAt, outcome,
  lanes:[{index, agentId, status, credits}]}`; a lane `status` ∈ {done, failed,
  skipped} so a branched run's per-lane outcomes (incl. cost-free skips) are
  first-class. Pure + unit-tested (round-trip, atomic write, missing-file-empty,
  invalid-errors, newer-schema-tolerant, skipped-lane carries no cost,
  stamps-finished-when-zero).
- **Record once on completion (`internal/web/workflow.go`):** the web adapter records
  the run in `runFrags(run, done=true)` — the one terminal point, reached exactly once
  per run (after `run.done` flips, events stop routing to `handleRunEvent`, where
  `busy` also clears). `recordRun` → `runRecord(run)` (a pure mapping; lane status via
  the shared `glyphFor`/`laneStatusName` vocabulary) → `Append`, best-effort (a disk
  error is logged, not surfaced). `workflowRun` gains adapter-stamped `runID`/`started`
  (the pure engine methods stay clock-free).
- **Runs view (`internal/web/runs.go`, `templates/fragments.html`):** a top-level
  **Runs** nav page lists persisted runs most-recent-first — each run's name, mode,
  outcome, when, total cost, and per-lane breakdown (agent, status glyph, credits),
  resolving agent ids to names under `forgeMu` like the Telemetry cost breakdowns.
  Adding the page bumps the `pageNames` / e2e `pages` count (helpers.ts updated).
- **Demo/e2e:** `bootstrap.seedRuns` seeds a couple of completed runs (incl. one with a
  skipped branch) so the Runs page renders offline; an e2e asserts a run-record row
  appears after running a workflow — structure, never figures (the demo run store is
  shared + append-only across the suite).

See ADR-0022 and the guard tests in the REGRESSIONS "a workflow run is persisted to
history exactly once" entry.

## Notes
- **Decision:** a dedicated sibling run store over (a) deriving from spend tags — which
  can't see a cost-free skipped branch nor a run's start/finish/outcome — or (b)
  folding runs into the ledger, which overloads its one-row-per-metered-turn contract.
  Two stores, one proven persistence pattern. Recorded in ADR-0022.
- **Numbering:** issue 0019 / ADR-0020 stay reserved for C1 (MCP secrets / Env editor),
  which had not merged to `main` when this branched; B3 takes issue 0021 / ADR-0022
  (continuing past B2's 0020 / ADR-0021) so the numbers don't collide when C1 merges.
- **Last v2 item:** B3 closes roadmap v2 (epic 0013). Pure run-history aggregations
  (per-workflow run counts, average run cost/duration) are deferred until a surface
  needs them — the records carry enough to compute them later without a schema change.
