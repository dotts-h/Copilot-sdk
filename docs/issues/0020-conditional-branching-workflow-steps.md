---
id: 0020
title: Conditional / branching workflow steps (roadmap v2, item B2)
status: closed
severity: medium
group: 0013
github:
links:
  adr: ../adr/0021-conditional-branching-workflow-steps-declarative-predicate.md
  prs: []
  issues: [0013]
  regression: ../REGRESSIONS.md
assets: []
---

## Summary

Workflows (ADR-0013/0017) ship sequential handoff and parallel fan-out, but every
step **always runs** — that is fan-out / hand-off, not control flow. There is no way
to gate a step on a prior step's outcome (e.g. *"if the review lane flags issues, run
the fix agent — otherwise skip it"*). Add a `When` predicate to `WorkflowStep` so a
step runs only when a prior lane's settled outcome matches, moving `Workflow` from a
fixed pipe to real control flow — the first genuinely orchestration-shaped capability
beyond fan-out/handoff. Source: `docs/NEXT_FEATURES.md` item B2; needs an ADR for the
predicate model.

## Repro
1. Define a workflow whose step 2 should run only when step 1's output mentions
   "issues".
   - **Expected:** step 2 runs only on that condition; otherwise it is skipped (and
     visibly so), and the run still completes.
   - **Actual (pre-B2):** every step always runs; there is no predicate on a
     `WorkflowStep` and the `workflowRun` engine has no skip state.

## Resolution (shipped — ADR-0021)

- **Declarative predicate model (`internal/ctxforge/workflow.go`):** a nullable
  `WorkflowStep.When *StepCondition` where `StepCondition{Step, Condition, Value}` —
  `Step` is the **1-based index of a strictly-prior step**, `Condition` ∈
  {`succeeded`, `failed`, `output-contains`, `always`}, `Value` is the
  case-insensitive substring for `output-contains`. Pure data, no expression engine.
  `Workflow.Validate` checks it (known condition; strictly-prior step → no
  self/forward reference → acyclic by construction; a value for `output-contains`).
  `omitempty` + nil-means-always, so every pre-B2 workflow loads and behaves
  identically. `CompileWorkflow` carries `When` into `CompiledStep`.
- **Pure branching engine (`internal/web/workflow.go`):** a fifth terminal lane
  status `laneSkipped`; `evalWhen` evaluates the predicate over settled lanes;
  sequential `advance` walks forward, running the first satisfied step and skipping
  unsatisfied ones; parallel `evalPending` launches every lane whose dependency has
  settled (to a fixpoint), run-or-skip. A skip counts as settled so the run still
  terminates; `failLane` now returns `[]int` so a parallel failure can unblock a
  `when failed` lane. All unit-tested with no client.
- **Surface:** `renderLanes`/`laneGlyph` render a skipped lane distinctly (`⊘`,
  `lane-skipped`) with its reason; the steps textarea round-trips a predicate as an
  optional `[step N condition value]` prefix (`splitStepLine` splits on the colon
  after the bracket, so an output-contains value may contain a colon).
- **Demo/e2e:** a seeded sequential branching workflow (`review-then-fix`) whose
  review output gates a fix step (RUNS) and a celebrate step (SKIPS), so the browser
  suite drives a real branch and asserts the skipped lane — structure, never timing.

See ADR-0021 and the guard tests in the REGRESSIONS "a branching step is skipped"
entry.

## Notes
- **Decision:** declarative enum predicate over a free-form/CEL expression (keeps
  `ctxforge` pure + dependency-free; statically validateable; deterministic). The
  index reference makes cycles structurally impossible — acyclicity is an arithmetic
  `Validate` check, not a graph walk. Recorded in ADR-0021.
- **Numbering:** issue 0019 / ADR-0020 are reserved for C1 (MCP secrets / Env editor),
  which had not yet merged to `main` when this branched; B2 takes 0020 / ADR-0021 per
  the roadmap so the numbers don't collide when C1 merges.
- **Pairs with / next:** B3 (workflow run history) — a run is the natural unit of
  orchestrated spend, and a branched run's per-lane outcomes (incl. skips) are what a
  run-history view records.
</content>
