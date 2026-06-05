---
id: 0010
title: Multi-agent run / handoff surface (Tier 2, item 2.1)
status: closed
severity: high
group: 0005
github:
links:
  adr: ../adr/0013-multi-agent-workflow-run-handoff-surface.md
  prs: []
  issues: [0005]
  regression:
assets: []
---

## Summary

The product is named *my-orchestra* but exposed no orchestration: sub-agent
events were rendered as a background chip strip, but there was no control surface
to compose or run multiple agents. Add the big-bet surface — a small workflow
(pick an agent, hand off to another on completion, or fan out to parallel agents)
watched as lanes in the timeline. Source: `docs/NEXT_FEATURES.md` item 2.1.

## Repro
1. Want to run the Builder agent, then hand its output to the SDET agent.
2. Expected: define the chain once and run it, watching each step as a lane.
3. Actual (before): no such surface — only one agent persona per session.

## Resolution

- **Pure domain (`internal/ctxforge/workflow.go`):** `Workflow{id, name,
  description, mode, steps}` + `WorkflowStep{agentId, prompt}`, `mode` ∈
  {sequential, parallel} (`""` = sequential). `Validate` covers shape; whole-forge
  `Validate` enforces step→agent referential integrity (like agent→skill, built-in
  `chat` resolves). Forge CRUD with rollback-on-invalid. `CompileWorkflow` reuses
  `Compile` to produce a `SessionSpec` per step — deterministic, unit-tested with
  no browser.
- **Run engine (`internal/web/workflow.go`):** a pure `workflowRun` state machine
  (`start`/`handoffPrompt`/`finishLane`/`failLane`/`laneFor`) — no IO, unit-tested
  for both modes. Each lane is a sub-run on the seam's `CreateSession`/`Send`;
  sequential hands each lane's output to the next, parallel fans all out. Events
  route to a lane by `SessionID` (sole-running fallback for the mock).
- **Surface:** a Workflows CRUD nav page (mirrors Agents) with a **▶ run** control;
  the run lands on the chat page where a `#lanes` panel streams each step (status
  glyph, agent, collapsible output, metered cost) over a new `lanes` SSE event. A
  run is mutually exclusive with a chat turn (both gated by `busy`); `/clear` resets
  it. Per-lane usage folds into the account-wide + per-session meters and the spend
  ledger.
- **Demo/e2e:** a seeded sequential **Build & harden** workflow (builder → sdet)
  with scripted lanes, so the surface runs end-to-end in the browser suites.
- Design recorded in **ADR-0013** (where the model lives & Validates; one session
  per step on the seam; lane attribution by `SessionID`; a dedicated lanes panel).

## Notes

Scope honesty: **sequential handoff ships fully** (model + engine + run wiring +
demo + e2e); **parallel** is implemented in the model, engine, and run wiring
(lane attribution by `SessionID`) and unit-tested, but the offline demo/e2e drive
only sequential because the `MockClient` returns a single session id — tracked as
TECH_DEBT #12 (also: per-lane tool/permission surfacing).

Guarding tests: `internal/ctxforge` `TestWorkflowValidate`,
`TestForgeValidateWorkflowAgentReference`, `TestForgeWorkflowCRUD`,
`TestCompileWorkflow`; `internal/web` `TestRunSequentialHandoff`,
`TestRunParallelStartsAllNoHandoff`, `TestRunSequentialFailAborts`,
`TestRunParallelFailLetsOthersFinish`, `TestLaneForRouting`,
`TestWorkflowRunReducerSequential`, `TestWorkflowLanesEscapeModelText`,
`TestSendBlockedDuringWorkflowRun`, `TestWorkflowsPageLists`,
`TestWorkflowRunHandlerStartsLanes`, `TestWorkflowCreateValidationError`,
`TestWorkflowCreateAndParseSteps`; browser `e2e/tests/e2e.spec.ts`
"multi-agent workflows". New route group + `lanes` SSE event + `Workflow` schema
recorded in CONTRACTS; lane-routing + escaping gotchas in REGRESSIONS. Closes the
2.1 item of epic 0005 — the Tier-2 orchestration epic is now complete.
