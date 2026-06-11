# 0043. The pause ledger (typed records + idempotent resolution), the escalate back-channel custom tool, and `input-required` as a non-terminal lane state

- Status: accepted
- Date: 2026-06-11
- Deciders: Horia
- Related: epic [0069](../issues/0069-epic-first-class-subagents.md) (first-class
  sub-agents), issue [0073](../issues/0073-pause-continue-cancel-escalate.md) (S4, this
  ADR's child), [ADR-0040](0040-subagent-identity-instance-agentid-vs-spawn-toolcallid.md)
  (the instance↔spawn identity model the pause's `AgentID` uses),
  [ADR-0042](0042-per-subagent-cost-attribution-and-the-budget-leash.md) (the budget
  leash that defers its mid-run enforcement to this pause point),
  [ADR-0024](0024-run-abort-cooperative-then-hard.md) (run abort — the hard verb this
  ADR's cooperative cancel complements), [ADR-0008](0008-budget-guardrails-soft-warn-and-hard-cap-gate.md)
  (the blocking-callback-resolved-by-POST gate shape this generalizes),
  the permission bridge (`internal/copilot/bridge.go`),
  [SUBAGENTS_RESEARCH.md](../SUBAGENTS_RESEARCH.md) §1–2, [CONTRACTS §1/§3](../CONTRACTS.md)

## Context

S1–S3 made sub-agents attributable, listed, and metered. They still could not be
**stopped to ask a human**: a sub-agent that hit a blocker or an ambiguity either
failed or barreled on. The research (§1–2) fixed the shape to copy: A2A's
non-terminal `input-required` task state, OpenAI's outermost-run interruption with a
serialized resume, LangGraph's interrupt with two distinct cancel verbs, MCP's
elicitation wire shape, and Temporal's per-pause SLA timer. The mechanism is a
generalization of our existing **permission bridge** (`internal/copilot/bridge.go`):
a synchronous callback blocks on a one-shot channel that the asynchronous web UI
resolves via a POST.

Four things had to be fixed:

1. **What a pause IS, and where its semantics live.** The permission bridge carries a
   bare `bool`; a pause needs a typed kind, a message, capability flags, and an
   optional SLA — and its idempotent-resolution guarantees must be testable without a
   browser or a client.
2. **How a sub-agent reaches the human given it runs *inside* the SDK.** ADR-0042
   left this open: the orchestrator observes a sub-agent but does not drive its
   per-turn `Send`, so there is no native hook to block on.
3. **What a parked lane's lifecycle state is.** A paused lane must NOT settle (the run
   stays live and siblings keep streaming), but it is not "running" either.
4. **The two cancel verbs**, and what an unattended pause does.

## Decision

**1. A pure `internal/pause` ledger owns the records and the idempotency invariant.**
A `Pause{ID, AgentID, Kind (input|issue|budget|permission), Message, Caps
(continue|respond|cancel), Deadline, OnExpiry}` is registered with a `Ledger`, which
hands back a pointer the caller blocks on (`Wait` → a buffered one-shot channel).
**Every** resolution path — `Resolve` (a POST), `CancelAll` (a run abort), `Sweep`
(an SLA timeout) — routes through one `deliver` that, under the mutex, removes the id
from the open map *before* sending on the channel. That single point is the
idempotency invariant: a second `Resolve`, a `Resolve` racing a `CancelAll`, or a
duplicate POST all find the id already gone and return `false` — the duplicate-answer
and abort-while-pending races collapse to a no-op. The package is dependency-free, so
register→emit→resolve, double-resolve-is-noop, abort-resolves-exactly-once, and the
clock-injected timeout are table-tested with no HTTP.

**2. The escalation back-channel is an orchestrator-registered custom tool whose
handler runs in our process and blocks.** `Server.escalate(req)` registers a pause,
parks the caller's lane, broadcasts the form, then **blocks on the pause** until the
human resolves it — returning the human's instruction (continue) or a wrap-up
directive (cancel/timeout) as the tool result handed back to the sub-agent. This is
the SDK's `DefineTool` pattern (the research's `escalate`/`report_status`): the
handler is *ours*, so it can wait. The native SDK asks
(`EvPermission`/`EvUserInput`/`EvElicitation`) are designed to route into the same
ledger later; the seam is the same. Because there is no SDK custom-tool injection
yet, the demo/e2e exercise the back-channel by driving `escalate` from the scripted
demo lane — a faithful seam test, no browser required.

**3. `input-required` is a new, explicitly non-terminal lane state.** `settled()`
returns false for it, so a parked lane keeps the run live, `allSettled()` stays
false, and parallel siblings keep streaming — A2A's contract. It renders amber
(`--warn`, matching the S2 roster's `input-required` glyph). On **continue** the lane
returns to `running` and finishes normally at its next idle; on **cooperative
cancel** the lane is *armed* (a `cancelReason`) but keeps running so the sub-agent
wraps up, then settles `failed (cancelled)` at its next idle — distinct from
**hard abort** (`Client.Abort`, ADR-0024), which force-resolves every pending pause
via `CancelAll` so no tool-handler goroutine leaks.

**4. The SLA timeout is in the pure layer, clock-injected, and off by default in
interactive mode.** A pause carries an optional `Deadline` + `OnExpiry` default;
`Ledger.Sweep(now)` resolves the past-deadline ones to their default and returns the
swept ids. Interactive mode arms no timer (the human is present — an indefinite wait
is fine and a stray timer would steal a turn); autopilot drives `Sweep` from a ticker.

## Consequences

- **The permission bridge is now one instance of a general pattern.** Pauses carry a
  typed kind + capability flags the permission bridge's bare `bool` could not; the UI
  renders only the declared buttons (the Agent Inbox model). The four existing
  bridges are unchanged — this is an additive surface, not a migration.
- **Pre-pause idempotency invariant (carry into CONVENTIONS):** a pause must be
  *registered* (in the ledger, id minted) **before** its event is emitted / the lane
  is parked / the caller blocks. Emitting first opens a window where a resolution
  POST arrives for an id the ledger does not yet hold and is dropped, and the caller
  then blocks forever. `Register` returns the stored pointer precisely so the caller
  cannot block on a pause it has not yet stored.
- **An abort can never strand a parked goroutine.** `abortRun` calls `CancelAll`
  under the same lock that settles the run, so a blocked `escalate` always unblocks
  and the run clears `busy` (the ADR-0024 single-completion-point guarantee holds).
- **In-memory only (accepted gap).** Pauses live in the ledger, not on disk; a crash
  while a sub-agent is parked orphans the pause. Acceptable for the attended UI now —
  recorded in TECH_DEBT; persist when unattended/autopilot runs matter (the same
  boundary the epic's risk list drew).
- **The overlay (S5) and attention surface (S6) build on this unchanged.** S5 renders
  the *same* `pauseForm` partial inside the per-sub-agent dialog; S6 badges off the
  `input-required` lane state this ADR introduced. Neither re-models the ledger.
