---
id: 0043
title: "Rerun a recorded run from the Runs page — re-execute its workflow's current definition (roadmap v8, item V18)"
status: open
severity: medium
group: 0042
github:
links:
  adr: [0023]
  prs:
  issues: [0042]
  regression:
---

## Summary

After roadmaps v4–v7 the orchestration surface is **fully observable but entirely
read-only**: the Runs page (ADR-0022) lists every persisted run — its lanes, outcomes
(including skipped branches), duration, and metered cost — and the Telemetry page
reconciles ledger vs. runs (V15–V17). A user can *see* that a run failed or cost too much,
but the only way to **act** on it (re-execute that workflow) is to navigate to the
Workflows page and find the row by hand. The sole run-trigger (`POST /workflows/{id}/run`,
ADR-0013) lives only on the Workflows page; the orchestration history has no action at all.

**V18 makes the orchestration surface interactive**, starting with a **rerun** control on
each recorded run: a `↻ rerun` button that re-executes that run's workflow. It is the
**first child** of epic [0042](0042-epic-interactive-orchestration.md) (roadmap v8 —
interactive orchestration); on its merge the epic is born in this PR. Unlike the v7 reader
children, this is an **action with side effects** (it spawns live orchestration), so it
takes **ADR-0023** for the rerun semantics and the shared trigger seam. Source:
`docs/NEXT_FEATURES.md` "roadmap v8" section (V18 entry).

> A **historical replay** (re-run the exact recorded steps/prompts) was weighed and
> **dropped**: a `RunRecord` doesn't carry the step definitions (it would need a schema
> change to snapshot them), and re-running a stale definition is rarely what a user wants.
> A rerun re-executes the workflow's **current** definition (looked up by `WorkflowID`) —
> a re-execution, not a replay — recorded in ADR-0023.

## Repro
1. Open the Runs page with a recorded run whose workflow still exists.
2. Try to re-execute that workflow from there (you just saw it cost too much / failed and
   want to run it again).
   - **Expected:** a "Rerun" control on the run that re-launches the workflow (watch each
     step stream as a lane on the Chat page), the new run rolling up under the same
     per-workflow totals.
   - **Actual (before V18):** no action on the Runs page at all — you must navigate to the
     Workflows page and find the row by hand.

## Proposed resolution

- **`internal/web` (`workflow.go`):** extract the run-launch body of `handleWorkflowRun`
  into `func (s *Server) launchWorkflow(id string) bool` — compile the workflow under
  `forgeMu`, and (if not `s.busy`) install a fresh run, mark busy, start the lanes, return
  `true`; on a compile error or busy server make **no state change** and return `false`.
  `handleWorkflowRun` becomes a thin caller (chat page on `true`, Workflows page on
  `false`). Add `handleRunRerun` (`POST /runs/rerun/{workflow}`) calling the same
  `launchWorkflow` with the recorded `WorkflowID` — chat page on `true`, Runs page (at the
  request's `?window=`) on `false`.
- **`internal/web` (`runs.go`):** `runRow(r, window)` surfaces `WorkflowID`, a `CanRerun`
  gate (`s.forge.Workflow(r.WorkflowID) != nil`, under the `forgeMu` the render holds), and
  the active `window` so the button's POST preserves the selection.
- **`internal/web/templates/fragments.html` (`runRecord`):** a `↻ rerun` button in the run
  head, rendered only when `CanRerun`, with a **DISJOINT `rerun` marker class** so it can't
  collide with the Workflows-page `button.run` or the `a.export` links (the V16/V17
  strict-mode lesson).
- **`internal/web/static/app.css`:** a small `.run-rec-head .rerun` button style.
- **`internal/web/hub.go`:** wire `POST /runs/rerun/{workflow}`.
- **ADR-0023** (written first, ADR-0004): rerun re-executes the workflow's **current**
  definition under the same `WorkflowID` (re-execution, not replay); reuses the existing
  `launchWorkflow` trigger (no new runtime seam); gated on `CanRerun`; refused while busy.
  CONTRACTS §3 gains the route; CONTEXT gains the **rerun** term.

## Tests (failing-first)

- **`internal/web` `TestRunRerunHandlerStartsRecordedWorkflow`:** a POST to
  `/runs/rerun/ship` opens a backing session, sends the first lane's prompt, lands on the
  chat page, and the active run carries the recorded `WorkflowID` (`s.run.id == "ship"`) —
  the rollup-coherence proof.
- **`internal/web` `TestRunRerunUnknownWorkflowNoLaunch`:** a POST to `/runs/rerun/gone`
  starts no run, changes no state, and re-renders the Runs page (fail-safe gate).
- **`internal/web` `TestRunsPartialShowsRerunForLiveWorkflowHidesForOrphan`:** the Runs
  render shows a `rerun` control (posting to the workflow id, carrying the window) for a
  run whose workflow exists, and **none** for an orphan run.
- **e2e (`e2e.spec.ts`):** a structural assertion that the seeded build-and-harden run
  shows a `button.rerun[hx-post^="/runs/rerun/build-and-harden"]` (DISJOINT from
  `button.run` / `a.export`), verified against the Go-rendered demo HTML.

## Resolution (shipped)

_(to be filled on merge — PR number recorded here + on the epic + INDEX, same branch.)_

## Notes
- **ADR-0023:** the first **action** child of the project since the run-store itself —
  re-executing a workflow from a historical record. The decision is the rerun semantics
  (re-execute current definition, not replay) + reusing the single `launchWorkflow` trigger
  (no new runtime seam). Captured in CONTRACTS §3 (the route) and CONTEXT (the **rerun**
  term).
- **Differentiator:** turns the orchestration surface from observable to **actionable** —
  the v8 theme. **First child** of epic 0042; on merge the epic records the PR and stays
  **open** for the next interactive child (re-ranked from a fresh value×fit pass).
- **Numbering:** issue **0043** (next free after 0041), the first build of epic **0042**.
  **ADR-0023 consumed** (the action seam; highest ADR becomes 0023).
