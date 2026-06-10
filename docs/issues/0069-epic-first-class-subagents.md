---
id: 0069
title: "Epic: First-class sub-agents — live view, per-subagent cost, and HITL pause/continue/cancel (roadmap v12, S1–S6)"
status: open
severity: high
group:
depends_on: []
github: 110
links:
  adr: []
  prs: []
  issues: [0070, 0071, 0072, 0073, 0074, 0075]
  regression:
---

## Charter

Sub-agents today are a **lifecycle-only activity strip**: `normalize.go` maps the SDK's
`SubagentStarted/Completed/Failed` to `EvSubagentStart/End`, and the chat shows a
name+detail indicator (`server.go` `subagents`, `renderSubagents`, issue 0031). What they
are *not* is **first-class**: their streamed activity is invisible (worse — interleaved
into the main transcript indistinguishably), their cost is a single end-of-run token
count, and they cannot pause, escalate, or be cancelled individually.

The research deliverable [docs/SUBAGENTS_RESEARCH.md](../SUBAGENTS_RESEARCH.md)
(5 verified web-research passes + a local SDK audit) established:

1. **The SDK already streams everything the live view needs.** The
   `rpc.SessionEvent` envelope carries `AgentID *string` ("Sub-agent instance
   identifier. Absent for events from the root/main agent") on **every** event, and
   `IncludeSubAgentStreamingEvents` defaults to true. Our normalizer drops `AgentID` —
   the single load-bearing gap (S1).
2. **The field converged on the user-sketch UX**: a persistent sub-agent list beside
   the chat + 4-state status vocabulary (working / **input-required** amber / done /
   failed) + click-through overlay (Devin sidebar, Cursor 3 Agents Window, GitHub
   mission control).
3. **Pause/continue/cancel is a generalization of our `permBridge`** (blocking callback
   + one-shot channel resolved by POST — same shape as Claude `canUseTool`, OpenAI
   `interruptions`, Restate awakeables), with the escalation back-channel as an
   orchestrator-registered **custom tool** (`copilot.DefineTool` — the handler runs in
   *our* process and can block until the human answers). A2A's non-terminal
   `input-required` task state is the contract to copy into the lane state machine.
4. **Per-subagent cost is a tag-threading exercise** on the ADR-0018 attribution
   pattern — and the dominant risk control: Anthropic measured multi-agent ≈ **15×**
   chat tokens, and GitHub's June 2026 AI-Credits switch makes sub-agent spend
   genuinely token-proportional. A per-subagent **budget leash** lands at the existing
   pre-`Send` gate.

This epic is the convergence of **both differentiators on one surface**: orchestration
(first-class, controllable sub-agents) × cost-awareness (live per-subagent metering),
governed by the existing third pillar (the pause/permission gates).

## Children

- [ ] **S1 · `AgentID` attribution through the seam** ([0070](0070-agentid-attribution-seam.md), S/M; ADR) —
      the keystone. Envelope `AgentID` → `copilot.Event.AgentID` on every normalized
      event; `MockClient`/demo emit tagged events. Pure seam change, no UI.
- [x] **S2 · Sub-agent registry + live list** ([0071](0071-subagent-registry-live-list.md), M; ADR-0041) —
      pure registry state (id, name, status, current activity, credits) fed by S1 +
      the existing lifecycle events; the chat-side list with the 4-state status
      vocabulary, rendered as idempotent SSE fragment re-renders.
- [ ] **S3 · Per-subagent cost + budget leash** ([0072](0072-per-subagent-cost-budget-leash.md), S/M) —
      `AgentID`-tagged `EvUsage` → per-subagent meter + additive `SpendRecord` tag
      (schema v3, the ADR-0018 pattern); live credits on the list; max-credits /
      max-turns leash at the pre-`Send` gate.
- [ ] **S4 · Pause / continue / cancel — the pause record + escalate tool** ([0073](0073-pause-continue-cancel-escalate.md), M/L; ADR) —
      typed pause records with capability flags; the orchestrator-registered
      `escalate`/`report_status` custom tool blocking on a one-shot channel;
      `input-required` as a non-terminal lane state; idempotent resolution; SLA
      timeout; cooperative-cancel vs hard-abort.
- [ ] **S5 · Per-subagent chat overlay** ([0074](0074-subagent-chat-overlay.md), M) —
      `<dialog>` loaded by htmx GET from the list (button + dblclick); per-agent
      transcript region with its own named SSE listener; pause form inside;
      send-into-subagent for lane-backed agents.
- [ ] **S6 · Attention surface** ([0075](0075-subagent-attention-surface.md), S) —
      badge count on the list header, amber title/favicon dot when any agent is
      input-required; Runs/lane integration (pause durations, input-required rendered
      distinctly).

## Acceptance (epic)

- [ ] Every normalized event is attributable to root vs a specific sub-agent instance;
      sub-agent deltas/tools never bleed into the main transcript.
- [ ] The chat shows a live sub-agent list: status glyph + text label (working /
      input-required / done / failed), current activity, and live credits per agent —
      working offline against the mock/demo.
- [ ] A sub-agent can escalate / request input mid-run; the run parks as
      `input-required` (non-terminal), and the user resolves it with
      continue(payload) / cancel — duplicate answers and a concurrent abort are safe
      (idempotent resolution), and an unattended pause times out to a default action.
- [ ] Per-subagent spend is metered live and persisted attributably in the ledger
      (additive schema; old ledgers read back); a per-subagent budget leash blocks
      before `Send`, not after the spend.
- [ ] Each child: failing test first, ADR where it sets semantics, `make lint && make
      test` (floor 65%) + `make e2e` green, born in its PR, SemVer minor.

## Sequencing

S1 → {S2, S3 in parallel} → S4 → {S5, S6 in parallel}. S1 is the keystone (everything
reads `Event.AgentID`); S2 and S3 share the tag but touch disjoint surfaces (render vs
telemetry); S5 needs the list (S2) to open from; S6 needs the input-required state (S4)
to badge.

## Risks (carry into the ADRs)

- **Don't trust `SubagentCompleted` alone** — claude-code#47936-class bugs (subagents
  dying early yet reporting completed in 14–30% of runs); cross-check tokens/output
  before rendering "done".
- **Unlistened named SSE events drop silently** — render the listening container
  before per-agent tagged events fire; keep idempotent full-fragment re-renders the
  foundation (OOB-in-SSE is empirically working but undocumented).
- **In-memory pauses orphan on crash** — acceptable for the attended UI now; persist
  pause records when unattended runs matter.
- **Pre-pause idempotency invariant** must be documented the day S4 lands.

## Notes

Research + sources: [SUBAGENTS_RESEARCH.md](../SUBAGENTS_RESEARCH.md) (A2A task states,
OpenAI outer-run interruptions, LangGraph supervisor routing, Devin/Cursor/mission-
control UX, OTel GenAI attribution, Claude Code `query_source` schema). Builds on:
ADR-0013/0017/0021 (lanes), ADR-0018 (attribution tags), ADR-0024 (run abort),
ADR-0008 (budget gate), issue 0031 (the activity strip this epic supersedes).
