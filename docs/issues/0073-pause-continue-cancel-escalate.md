---
id: 0073
title: "Pause / continue / cancel — typed pause records + the orchestrator escalate tool (S4)"
status: closed
severity: high
group: 0069
depends_on: [0070]
github: 114
links:
  adr: [0043]
  prs: []
  issues: [0069, 0071]
  regression:
---

> **Closed 2026-06-11.** Shipped on the `feat/subagent-pause-escalate` branch
> (ADR-0043): a pure `internal/pause` ledger with idempotent resolution + clock-injected
> SLA timeout; the orchestrator's `escalate` back-channel (`Server.escalate`) blocking on
> a one-shot channel; `input-required` as a non-terminal lane state (continue resumes,
> cooperative-cancel settles `failed (cancelled)`, hard abort `CancelAll`s pending
> pauses); inline capability-flagged pause forms (`#pauses`, `POST /pause/{id}`); and a
> demo "Escalation demo" workflow the e2e drives end-to-end. The overlay surface (S5),
> attention badging (S6), and pause persistence (TECH_DEBT #16) stay out of scope.

## Summary

The HITL heart of the epic: a sub-agent that hits an issue or needs input **parks as
`input-required`** (a non-terminal state, A2A's contract) and waits for
continue/cancel — instead of failing or barreling on.

- **Pause record (the generalization of `permBridge`).** A typed
  `Pause{ID, AgentID/LaneID, Kind (input | issue | budget | permission), Message,
  Fields?, Caps (continue | respond | cancel)}` registered with the orchestrator
  (which owns the pause ledger — OpenAI's outer-run invariant), emitted as an event,
  resolved by `POST /pause/{id}` with `continue(payload) | cancel`. Resolution is
  **idempotent** (CAS/`sync.Once`): duplicate POSTs and a concurrent run-abort cannot
  double-resolve — the Temporal signal-handler race.
- **The escalation back-channel.** An orchestrator-registered custom tool
  (`copilot.DefineTool`) injected into lane sessions: `escalate(reason, question?)` /
  `report_status(note)`. `escalate`'s handler emits the pause and **blocks on a
  one-shot channel** until resolution, returning the human's instruction as the tool
  result (or "cancelled — wrap up"). `report_status` is non-blocking telemetry for the
  S2 activity line. Native SDK asks (`EvPermission`/`EvUserInput`/`EvElicitation`) from
  sub-agent sessions route into the same pause surface.
- **Lane state machine.** Lanes gain `input-required` (non-terminal) alongside
  done/failed/skipped (ADR-0021): parked lanes don't settle, the run stays live, and
  the S2 list row flips amber. Structured input reuses the `ElicitRequest`
  flat-primitive + accept/decline/cancel shape (MCP elicitation).
- **Two cancel verbs.** **Cooperative cancel** resolves the pause with "cancel" so the
  sub-agent's turn ends cleanly and the lane records why; **hard abort** stays
  `Client.Abort` (ADR-0024), also force-resolving any pending pause.
- **SLA timeout.** Every pause carries a timer (default off in interactive mode, on in
  autopilot): on expiry, a configurable default (cancel | continue-with-note),
  annotated in the timeline — indefinite waits leak goroutines and budget unattended.
- **ADR-worthy:** the pause-record model + state machine; the custom-tool back-channel
  (vs SDK-native subagents' one-way stream); the idempotency + pre-pause-idempotency
  invariants; timeout semantics per mode.

## Acceptance

- [x] Pure pause store: register → emit → resolve(continue/cancel) table-tested;
      double-resolve is a no-op; abort-while-pending resolves exactly once.
      `internal/pause/pause_test.go` (`TestRegisterResolveContinue`,
      `TestDoubleResolveIsNoOp`, `TestCancelAllThenResolveIsNoOp`).
- [x] `escalate` round-trip against the mock: sub-agent calls tool → pause event →
      `POST /pause/{id}` continue with payload → tool returns the payload to the
      sub-agent; cancel returns the cancel instruction (seam test, no browser).
      `TestEscalateContinueReturnsPayload` / `TestEscalateCancelReturnsDirective`.
- [x] A lane parks as `input-required` (run not settled), resumes on continue, settles
      `failed (cancelled)` on cancel; parallel siblings keep streaming meanwhile.
      `TestLaneParksInputRequiredSiblingsStreamThenResumes`, `TestLaneCancelSettlesFailed`.
- [x] Pause forms render inline in chat (the `EvPermission` pattern) with only the
      capability-flagged buttons (Agent Inbox model: continue/respond/cancel as
      declared). `TestRenderPauseFormCapabilityFlagged`, `TestChatPartialRendersPendingPause`.
- [x] Timeout: an expired pause resolves to its default and annotates the timeline
      (clock injected, table-tested). `TestSweepExpiresToDefault` (clock injected via
      `Ledger.Sweep(now)`).
- [x] Pre-pause idempotency invariant documented (CONVENTIONS or the ADR). ADR-0043
      Consequences + CONTRACTS §3 (`POST /pause/{id}`).
- [x] Gates green: `make lint && make test` (floor 65%), `make e2e` (demo drives a
      scripted escalate — the "Escalation demo" workflow + the e2e that parks/resumes it).

## Out of scope

The overlay surface for pauses (S5 renders the same form in the dialog), attention
badging (S6), pause persistence across restart (accepted gap — record it in
TECH_DEBT when this lands).

## Notes

Research: [SUBAGENTS_RESEARCH.md](../SUBAGENTS_RESEARCH.md) §1–2 — A2A
`input-required`, OpenAI outermost-run interruptions + serialized resume, LangGraph
interrupt/rollback (the two cancel verbs), Temporal SLA pattern, MCP elicitation wire
shape; Claude's own SDK forbids `AskUserQuestion` inside subagents — validation that
the orchestrator routes the ask. Builds on the `permBridge` (ARCHITECTURE §interactive
permissions), the budget gate (ADR-0008), run abort (ADR-0024).
