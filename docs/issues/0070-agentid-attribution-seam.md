---
id: 0070
title: "AgentID attribution through the seam — every normalized event knows root vs sub-agent (S1)"
status: open
severity: high
group: 0069
depends_on: []
github: 111
links:
  adr: [0040]
  prs: []
  issues: [0069]
  regression:
---

## Summary

The keystone child of epic [0069](0069-epic-first-class-subagents.md). The SDK's
`rpc.SessionEvent` envelope carries `AgentID *string` — "Sub-agent instance identifier.
Absent for events from the root/main agent and session-level events" — and
`IncludeSubAgentStreamingEvents` defaults to **true**, so sub-agent deltas, tool events,
and usage are *already* arriving on the stream. `SDKClient.makeHandler`
(`internal/copilot/normalize.go`) ignores the envelope, so sub-agent activity
interleaves into the main transcript indistinguishably (a sub-agent's `EvMessageDelta`
appends to the user-facing bubble; its `EvUsage` is metered as the root agent's spend).

S1 threads the tag through the seam, UI-free:

- `copilot.Event` gains `AgentID string` (empty = root agent), populated from the
  envelope for **every** normalized event type — deltas, reasoning, tool start/
  progress/end, usage, permission/input/elicitation where the SDK provides it.
- The chat reducer (`web.handleEvent`) and the lane reducer **filter**: agent-tagged
  events no longer mutate the root transcript (parked until S2 renders them; the
  lifecycle strip from issue 0031 keeps working unchanged).
- `MockClient` + the demo script emit `AgentID`-tagged events (deltas, tool, usage
  under a synthetic sub-agent) so the whole epic is developable and e2e-testable
  offline.
- Decision to record (ADR): the identity model — envelope `AgentID` (instance) vs
  `SubagentInfo.ToolCallID` (spawn invocation) — and how the two join (`SubagentStarted`
  arrives with a `toolCallId`; tagged stream events carry the instance id; the registry
  needs the mapping).

## Acceptance

- [x] `copilot.Event.AgentID` is set from the envelope on every event type the
      normalizer emits; table-test: each SDK event type with/without envelope AgentID →
      expected normalized event (extends the existing translation-correctness suite).
      `TestHandlerStampsAgentID` + `TestHandlerLeavesAgentIDEmptyForRootAgent`.
- [x] Sub-agent-tagged `EvMessageDelta`/`EvToolStart`/`EvUsage` no longer mutate the
      root transcript or the session meter (reducer test against the mock).
      `TestAgentTaggedEventsDoNotMutateRootTranscript` (+ `TestRootEventsStillMutateTranscript`).
- [x] `MockClient`/demo emit tagged events; the demo lane runs a synthetic sub-agent
      end-to-end offline. `streamDemoReply` streams a tagged delta/tool/usage under
      `sub-explorer-1`; e2e asserts the stream is parked, not leaked.
- [x] ADR records the identity model (AgentID vs ToolCallID join). ADR-0040.
- [x] CONTRACTS §2 (event vocabulary) updated additively; no existing event consumer
      breaks (the seam is additive — `AgentID` empty for all current paths).
- [x] Gates green: `make lint && make test` (floor 65%), `make e2e`.

## Out of scope (later children)

Rendering the parked sub-agent stream (S2), metering it (S3), pause/escalate (S4).

## Notes

Research: [SUBAGENTS_RESEARCH.md](../SUBAGENTS_RESEARCH.md) §0 (local audit) + §3
(parent-pointer attribution is the convergent pattern: AG-UI `parentRunId`, Claude SDK
`parent_tool_use_id`, LangSmith `parent_run_id`). Supersedes the "lifecycle-only"
limitation noted in issue [0031](0031-subagent-description-activity-strip.md).
