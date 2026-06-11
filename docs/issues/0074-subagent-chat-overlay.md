---
id: 0074
title: "Per-subagent chat overlay — popup transcript with live stream, pause form, and steer (S5)"
status: closed
severity: medium
group: 0069
depends_on: [0071]
github: 115
links:
  adr: [0044]
  prs: []
  issues: [0069, 0073]
  regression:
---

## Summary

The drill-down of the sketch: open a sub-agent from the S2 list into a **popup chat
overlay** showing its full live transcript, with interaction when the agent allows it.

- **Opening.** A visible per-row button **and** double-click (double-click alone is
  undiscoverable and has no touch equivalent). htmx GET loads the sub-agent's
  transcript fragment into a `<dialog>` (the command-palette/help-overlay pattern —
  ⎋ closes, focus-trapped, `view-transition` polish optional).
- **Live.** The dialog's transcript region carries its own named `sse-swap` listener on
  the **same** `/events` stream (htmx-ext-sse supports many child listeners per
  connection); deltas append, structural events re-render the fragment — the
  `#timeline` reducer pattern scoped per agent. Tool calls render collapsed
  one-line-per-call with expand (progressive disclosure, the Ctrl+O lesson).
- **Interaction.** When the sub-agent is `input-required`, the S4 pause form renders
  inside the overlay (same partial, capability-flagged buttons). For lane-backed
  sub-agents, a composer **steers**: input is queued and applied after the current
  tool call completes (GitHub mission control's contract) via `Send` on the lane's
  session. Cooperative-cancel and hard-abort buttons mirror S4's verbs.
- **State.** The transcript state per sub-agent lives in the S2 registry (bounded —
  cap retained turns per sub-agent; the full record is the session, not the overlay).

## Acceptance

- [x] Overlay opens from button and dblclick, closes on ⎋/backdrop, returns focus to
      the row (native `<dialog>` `showModal` — focus-trap/restore for free; e2e opens
      from the row button, asserts the modal, ⎋ closes it).
- [x] Transcript streams live inside the open overlay against the demo's synthetic
      sub-agent (per-instance `subagent-{spawnID}` SSE listener on the shared
      connection; e2e sees the demo's delta + grep tool card in the overlay).
- [x] Idempotent re-open: closing and reopening renders the same bounded transcript
      (no duplicate turns — `TestSubagentOverlayIdempotentReopen` + e2e re-open count);
      all model text escaped (`TestSubagentOverlayEscapesContent`).
- [x] `input-required` pause form renders and resolves from inside the overlay; the
      list row and overlay agree on state after resolution (`pausesFor` +
      `MarkInputRequired`/`ClearInputRequired` wired through escalate;
      `TestSubagentOverlayRendersPauseForm`).
- [x] Steer: composer input on a lane-backed sub-agent is delivered via the seam and
      annotated in the overlay timeline (mock-recorded `Send` —
      `TestSubagentSteerDeliversToSession`). **Seam built + gated on `LaneSession`** per
      the agreed scope (ADR-0044): SDK-native sub-agents are read+pause-only, and no
      production path sets `LaneSession` yet — lighting it end-to-end is a follow-up.
- [x] Gates green: `make lint && make test` (floor 65%), `make e2e`.

## Out of scope

Steering SDK-native (in-session) sub-agents — they have no `Send` target; the overlay
is read+pause-only for those (rendered state makes the difference visible).

## Notes

Research: [SUBAGENTS_RESEARCH.md](../SUBAGENTS_RESEARCH.md) §3 (overlay = htmx GET +
own named SSE listener; no new transport) and §5 (Devin session links, Claude Code
Enter-to-view/Escape-to-interrupt, mission-control steer contract). Reuses the dialog
patterns from ADR-0026 (command palette) and the per-lane timeline (ADR-0017).
