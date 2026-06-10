# 0040. Sub-agent identity through the seam — the envelope `AgentID` is the instance key, `SubagentInfo.ToolCallID` is the spawn key, and they join at the registry

- Status: accepted
- Date: 2026-06-10
- Deciders: Horia
- Related: epic [0069](../issues/0069-epic-first-class-subagents.md) (first-class
  sub-agents), issue [0070](../issues/0070-agentid-attribution-seam.md) (S1, this
  ADR's child), issue [0031](../issues/0031-subagent-description-activity-strip.md)
  (the lifecycle strip this supersedes the "lifecycle-only" limitation of),
  [CONTRACTS §2](../CONTRACTS.md) (the event vocabulary the new field joins),
  [SUBAGENTS_RESEARCH.md](../SUBAGENTS_RESEARCH.md) §0/§3 (the parent-pointer
  convergence: AG-UI `parentRunId`, Claude SDK `parent_tool_use_id`, LangSmith
  `parent_run_id`)

## Context

The SDK already streams sub-agent activity. `rpc.SessionEvent` carries
`AgentID *string` — *"Sub-agent instance identifier. Absent for events from the
root/main agent and session-level events"* — and `IncludeSubAgentStreamingEvents`
defaults to **true**, so a sub-agent's deltas, tool events, and usage arrive on the
same stream as the root agent's. The normalizer (`SDKClient.makeHandler`) ignored
the envelope, so those events were indistinguishable from the root agent's: a
sub-agent's `EvMessageDelta` appended to the user-facing bubble, and its `EvUsage`
was metered as the root agent's spend (the under-attribution this epic exists to
fix).

There are **two distinct identifiers** in play, and conflating them would mis-wire
every later child (registry, per-sub-agent cost, pause/escalate):

1. **The instance id** — `SessionEvent.AgentID`. Stamped on *every streamed event*
   a running sub-agent emits. This is the identity of a *live instance* of a
   sub-agent.
2. **The spawn/invocation id** — `SubagentInfo.ToolCallID` (from
   `SubagentStartedData.ToolCallID`). It identifies the *tool call that spawned* the
   sub-agent, and is the key the lifecycle strip (issue 0031) already tracks for
   start/end.

The `SubagentStarted`/`Completed`/`Failed` lifecycle events are **session-level**
(emitted from the parent's perspective): their envelope `AgentID` is absent. So the
spawn key arrives on the lifecycle events, while the instance key arrives on the
tagged stream — they are never carried by the same event, and the mapping between
them must be reconstructed.

## Decision

**Thread the instance id, keep the spawn id, and treat them as two keys that join.**

1. **`copilot.Event` gains `AgentID string`** (empty = root agent), populated from
   `derefStr(ev.AgentID)` for **every** normalized event type the handler emits.
   The field is **additive**: empty for every pre-existing path, so no existing
   consumer changes behaviour. This is the instance key.
2. **`SubagentInfo.ToolCallID` stays the spawn key**, unchanged. The lifecycle strip
   keeps working off it exactly as before.
3. **The reducer filters on the instance key, not the spawn key.** Until S2 gives
   sub-agents their own surface, an event with a non-empty `AgentID` is **parked**
   (dropped) in both the chat reducer (`web.handleEvent`) and — by placing the guard
   before the lane router — the workflow-lane path, so a sub-agent's stream never
   mutates the root transcript or meters the root agent's spend. Session-level
   lifecycle events (`EvSubagentStart`/`End`, `AgentID` empty) are unaffected.
4. **The join is deferred to the registry (S2).** The instance→spawn mapping is
   reconstructed where both keys are observable: `SubagentStarted` arrives with a
   `ToolCallID`; the first tagged stream event for that sub-agent carries the
   instance `AgentID`. The registry correlates them (chronological first-tag-after-
   start, refined if the SDK later exposes the pairing directly). S1 deliberately
   does **not** build that map — it only makes the instance key *available* on the
   seam so S2 can.

We model the instance id as the **parent-pointer** the research found convergent
across agent runtimes (AG-UI `parentRunId`, Claude SDK `parent_tool_use_id`,
LangSmith `parent_run_id`): a child event points at the agent that produced it. The
root agent is the empty pointer.

## Consequences

- **Attribution is now possible without UI.** Every normalized event knows root vs
  sub-agent; metering, rendering, and pause/escalate (S3/S4) can key off `AgentID`.
  The "lifecycle-only" limitation noted in issue 0031 is superseded — the activity
  strip is no longer the *only* thing we know about a sub-agent.
- **The seam stays additive and stable.** `AgentID` is empty for all current paths,
  so the CONTRACTS §2 vocabulary is extended, not broken; the
  every-event-has-a-test invariant gains two cases (with/without the tag).
- **A two-key model is carried forward.** Consumers must not assume instance id ==
  spawn id. The registry (S2) owns the join; until then, sub-agent stream events are
  parked, which means S1 ships **no new sub-agent rendering** — by design.
- **Offline-developable.** `MockClient`/the demo emit tagged deltas/tool/usage under
  a synthetic instance id distinct from the spawn's `ToolCallID`, so the whole epic
  is exercised end-to-end with no live runtime — and the demo proves the parking
  filter (the strip shows the sub-agent busy while its stream is held back).
- **Callback-path events remain untagged — an SDK limitation, not a choice.**
  `EvPermission`/`EvUserInput`/`EvElicitation`/`EvToolDecision` flow through SDK
  bridge callbacks (`handlers.go`) whose invocation types carry **no** envelope
  `AgentID`, so a sub-agent's permission/input request (if the SDK ever raises one
  through those callbacks) would arrive untagged and hit the root surface. The
  issue charter scopes S1 to "where the SDK provides it"; stamp these the moment
  the SDK exposes the tag on the callback payloads. `EvHookRun` IS tagged: the
  PostToolUse fire inherits the completing tool's envelope tag.
- **Revisit if the SDK exposes the pairing directly.** Should a future SDK event
  carry both keys (or a `parentToolCallId` on the tagged stream), the registry's
  chronological correlation is replaced by the authoritative mapping; this ADR's
  two-key framing still holds.
