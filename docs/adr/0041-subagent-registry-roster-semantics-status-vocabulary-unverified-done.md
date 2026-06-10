# 0041. The sub-agent registry — a persistent roster in `convo`, the 4-state status vocabulary, the first-tag-after-start join, and the unverified-done rule

- Status: accepted
- Date: 2026-06-10
- Deciders: Horia
- Related: epic [0069](../issues/0069-epic-first-class-subagents.md) (first-class
  sub-agents), issue [0071](../issues/0071-subagent-registry-live-list.md) (S2, this
  ADR's child), [ADR-0040](0040-subagent-identity-instance-agentid-vs-spawn-toolcallid.md)
  (the two-key identity model this registry joins), issue
  [0031](../issues/0031-subagent-description-activity-strip.md) (the transient strip
  this replaces), [SUBAGENTS_RESEARCH.md](../SUBAGENTS_RESEARCH.md) §3/§5 (htmx
  named-event mechanics; Devin/Cursor/mission-control UX convergence),
  [CONTRACTS §2](../CONTRACTS.md)

## Context

S1 (ADR-0040) made every normalized event attributable — instance `AgentID` on the
tagged stream, spawn `ToolCallID` on the lifecycle events — and deliberately deferred
the join, parking tagged events in the reducer. S2 builds the surface those events
were parked for: a live sub-agent list beside the chat. Four semantics had to be
fixed, and each shapes the later children (S3 credits, S4 pause, S5 overlay, S6
badges):

1. Where the registry lives and what owns the instance↔spawn join.
2. What the status vocabulary is, and whether a finished sub-agent stays visible.
3. Whether a `SubagentCompleted` is trusted at face value — sub-agents are known to
   die early yet report completed (claude-code#47936-class, 14–30% of runs in the
   linked reports).
4. How the list renders over SSE without falling into the named-event drop / OOB
   append traps (research §3).

## Decision

**1. The registry is a pure `convo` sibling (`convo.Subagents`), and it owns the
join.** Like `convo.State` it is a UI-agnostic, HTTP-free reducer target: the web
layer translates events into method calls (`Start`/`Observe`/`AddCredits`/`End`) and
renders from `Entries()`. The ADR-0040 join is implemented here as **chronological
first-tag-after-start**: the first tagged event for an unknown instance binds it to
the oldest still-`working` entry that has no instance yet. A tag that matches nothing
is **ignored gracefully** — dropping an unknown instance's activity is safe; inventing
an entry for it is not.

**2. The 4-state vocabulary, and entries persist.** `working` (accent, pulse) ·
`input-required` (amber — the attention state; the enum and rendering exist now, S4
wires the transition) · `done` (good) · `failed` (bad) — the field's de-facto
standard (Devin / Cursor / GitHub mission control). Status renders as a glyph **plus
a text label** (never icon/color alone — a11y, and Devin's labels beat icon-only).
A finished entry **stays on the roster** with its terminal status: the list is the
session's sub-agent record (what S5 opens an overlay onto), not the transient busy
indicator it replaces. `/clear` resets it.

**3. Don't trust completed blindly — the unverified-done rule.** A successful
completion is rendered plain `done` only when corroborated: the lifecycle event
reported **tokens** (`SubagentInfo.TotalTokens`, a new additive seam field carrying
the raw count beside the display-formatted `Detail`), or the registry **observed the
instance's stream** doing work. A zero-token, unobserved success renders **`done
(unverified)`**. Cheap insurance against the die-early-report-completed failure mode.

**4. Idempotent full-fragment re-render, change-gated.** The list re-renders as one
`subagents` SSE fragment (the existing named event whose listener container exists
from first paint — named events are never dropped, reconnect-safe) — the same
registry state yields byte-identical HTML, no append anywhere. The reducer emits the
fragment **only when the displayed registry changed** (`Observe` reports it), so a
delta storm updates the row at most once ("thinking…") instead of re-rendering per
chunk. Tagged usage performs the join/observation but defers display to S3.

## Consequences

- **The tagged stream now has a surface; the S1 invariant holds.** Tagged events
  reach ONLY the registry — root transcript and meters stay untouched (the S1 pin
  test still passes verbatim). The guard precedes the lane router, so sub-agents
  running during a workflow still feed the list.
- **S3/S4/S5/S6 land on prepared ground.** `Credits` + `AddCredits` exist (rendering
  0.00 until S3 prices tagged usage); `input-required` exists for S4; the roster rows
  are what S5's overlay opens from and S6 badges.
- **The demo must use unique per-turn ids.** Finished rows persist on the shared demo
  session, so a fixed spawn id would be swallowed by the duplicate-`Start` guard on
  the second turn; the demo now mints per-turn spawn/instance ids — matching the real
  SDK's per-invocation ids.
- **The join is heuristic until the SDK pairs the keys.** Two sub-agents spawned
  before either streams could in principle swap instances; per ADR-0040, replace the
  chronological correlation with the authoritative mapping the moment the SDK carries
  both keys on one event.
- **A row is bounded but unbounded in count.** A pathological session could grow the
  roster; acceptable for the attended UI now (same stance as in-memory pauses, epic
  risk list) — cap/collapse when it bites.
