---
id: 0087
title: "Split workflow.go — separate the run engine, seam adapter, renderer, demo, and CRUD"
status: open
severity: low
group: 0086
depends_on: []
github:
links:
  adr: []
  prs: []
  issues: [0086]
  regression:
assets: []
---

## Summary

`internal/web/workflow.go` (1222 LOC) bundles **five** distinct responsibilities, making it
the hardest file in the tree to navigate and the most likely to accrue further coupling:

1. the **pure run state machine** (`workflowRun`, `lane`, `laneStatus`, `start`, `evalWhen`,
   `evalPending`, `advance`, `finishLane`, `failLane`, `abort`, `allSettled`,
   `handoffPrompt`, `laneFor`);
2. the **seam adapter** (`launchWorkflow`, `startLane`, `laneError`, `handleRunEvent`,
   `abortRun`, `launchLanes`, `runFrags`, `recordRun`, `runRecord`);
3. the **demo lane simulator** (`streamDemoLane`);
4. the **lane renderer** (`renderLanes`, `laneToolsHTML`, `lanePermsHTML`, `laneGlyph`,
   `glyphFor`, `laneStatusName`, `costDetail`);
5. the **Workflows CRUD page** (`workflowsPartial`, `renderWorkflowForm`, `stepsFromText`,
   `splitStepLine`, …).

## Why now

Pure cleanup; no trigger beyond "it's the biggest coupling risk." Pick up as parallel filler
or the next time the file is substantially edited. The pure engine already has zero-client
unit tests, so the split is low-risk — those tests pin behavior byte-for-byte.

## Touches

- `internal/web/workflow.go` → split (same package, same `package web`) into:
  - `run_engine.go` — the pure state machine + its types/methods.
  - `run_adapter.go` — `launchWorkflow`/`startLane`/`handleRunEvent`/`abortRun`/`launchLanes`/`runFrags`/`recordRun`.
  - `run_render.go` — `renderLanes`/`laneToolsHTML`/`lanePermsHTML`/`laneGlyph`/`glyphFor`/`laneStatusName`/`costDetail`.
  - `streamDemoLane` may stay with the other demo code (`demo.go`) or in `run_adapter.go`.
  - The CRUD page can move to a `workflow_crud.go` or fold into the existing forge-CRUD file.
- Regenerate `docs/CODEMAP.md` (`make codemap`) — top-level decls move files.

## Acceptance

- [ ] `workflow.go` is split into focused same-package files along the seams above; no symbol
      renamed, no behavior changed.
- [ ] The pure `workflowRun` zero-client tests pass **byte-identically** (no test edits
      beyond none — they don't reference file paths).
- [ ] `make lint && make test` (floor 65%) green; CODEMAP regenerated.
- [ ] No ADR (mechanical split, no decision).

## Notes

Parallel-safe with 0089 (different package). Coordinate with 0088 (same package, different
files) only on the shared import block. SemVer **patch**.
</parameter>
