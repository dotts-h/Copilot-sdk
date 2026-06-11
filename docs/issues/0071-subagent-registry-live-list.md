---
id: 0071
title: "Sub-agent registry + live list — status, current activity, live credits beside the chat (S2)"
status: closed
severity: high
group: 0069
depends_on: [0070]
github: 112
links:
  adr: [0041]
  prs: []
  issues: [0069, 0031]
  regression:
---

## Summary

The visible half of the epic's live view. Replaces the lifecycle-only activity strip
(issue 0031) with a **sub-agent registry**: a pure, `convo`-style state holding one
entry per sub-agent instance — id, name/display-name, **status**, current activity
(latest tool or "thinking"), and live credits (fed by S3; renders 0 until then) — and a
chat-side **list partial** rendering it.

- **Registry (pure).** Fed by `EvSubagentStart/End` (lifecycle) + `AgentID`-tagged
  `EvToolStart/End` (current activity) + tagged `EvUsage` (credits). Lives beside
  `convo.State`; unit-tested with no HTTP. Join: `SubagentStarted.ToolCallID` ↔ the
  envelope instance id (the S1 ADR's identity model).
- **Status vocabulary (the field's de-facto standard, 4 states):** `working` (accent,
  pulse) · `input-required` (amber — the attention state, wired by S4; the enum exists
  now) · `done` (good) · `failed` (bad). Rendered as a status glyph **plus a text
  label** (Devin's labels beat icon-only) next to the list index — the "small icons
  around their list index" from the sketch.
- **Rendering.** A `#subagents` region in the chat layout, full-fragment idempotent
  re-render over the existing SSE stream on registry change (reconnect-safe; the
  container exists from first paint so named events are never dropped). Each row:
  glyph + label + name + current activity + credits. Collapsed by default to a compact
  strip when empty.
- **Don't trust completed blindly:** a `SubagentCompleted` with zero tokens/empty
  output renders `done (unverified)` — the claude-code#47936 lesson.

## Acceptance

- [ ] Pure registry: table-tested transitions (start → working; tool start → activity
      updates; end success → done; end failure → failed; unknown-instance events
      ignored gracefully).
- [ ] The chat shows one live row per sub-agent with status glyph + text label +
      current activity, updating over SSE against the mock/demo (Go reducer test +
      Playwright e2e on the demo's synthetic sub-agent).
- [ ] Idempotent re-render: replaying the same registry state produces identical HTML
      (no append-leak); all model-originated text escaped.
- [ ] Zero-token completion renders as unverified-done (regression-style table case).
- [ ] a11y: the list is a labelled region; status conveyed by text, not color alone
      (axe scan stays green in both themes).
- [ ] Gates green: `make lint && make test` (floor 65%), `make e2e`.

## Out of scope

Live credits (S3 wires the value), the overlay (S5), input-required wiring + badges
(S4/S6).

## Notes

UX prior art: Devin session sidebar ("Awaiting instructions" labels, per-child cost),
Claude Code teammate list ("what they're working on"), GitHub mission control —
[SUBAGENTS_RESEARCH.md](../SUBAGENTS_RESEARCH.md) §5. htmx mechanics (named-event drop,
OOB caveat): §3.
