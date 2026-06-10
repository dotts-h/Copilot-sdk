# Sub-agents as first-class agents — deep research (2026-06-10)

> Research deliverable, not a commitment. Companion to [NEXT_FEATURES.md](NEXT_FEATURES.md)
> (roadmap v3) and the workflow-lane stack (ADR-0013/0017/0021/0022/0024). The question:
> **make sub-agents first-class agents that communicate back to the orchestrator —
> escalating on issues, pausing for HITL continue/cancel — with a live chat-side view
> (bubble/list with live cost + current activity, status icons, double-click popup chat
> overlay).** Method: 5 parallel web-research passes (communication protocols, HITL
> mechanics, UI event streaming, cost metering, product UX prior art) + a local audit of
> the vendored SDK and our seam, then 3-vote adversarial verification of the 14
> contested claims (**13/14 unanimously confirmed; 1 amended for currency** — noted
> inline).

## TL;DR — the verdict

The user's sketch is exactly what the field converged on in 2025–2026: a **persistent
sub-agent list beside the chat + status badges + click-through overlay** (Devin's session
sidebar, Cursor 3's Agents Window, GitHub mission control all landed there). The
mechanics decompose into four findings:

1. **Communication = a task state machine, not bidirectional chat.** A2A's task states
   (`working / input-required / completed / failed / canceled`) with resume-by-same-id is
   the industry contract; interruptions surface at the *outer* run (OpenAI), and control
   always returns to the supervisor (LangGraph). The escalation back-channel is built
   app-side — the SDK's native sub-agents stream one-way lifecycle only.
2. **Pause/continue/cancel is a generalization of our permission bridge, not a new
   mechanism.** Every major SDK (Claude `canUseTool`, OpenAI `interruptions`, Restate
   awakeables) is "blocking callback resolved by an out-of-band action over a one-shot
   channel" — which `permBridge` already implements. We add a typed **pause record** +
   idempotent resolution + an SLA timeout, and the back-channel itself is an
   orchestrator-registered **custom tool** (`copilot.DefineTool` — handler runs in *our*
   process and can block until the human answers).
3. **The SDK already streams everything the live view needs.** Vendored
   `copilot-sdk/go v1.0.0` has `SubagentStarted/Completed/Failed` lifecycle events
   (tokens, tool-calls, duration, model) **and an `AgentID` on every event envelope** —
   our normalizer currently drops it, which is the single load-bearing gap.
4. **Live per-subagent cost = thread `AgentID` into the existing meter/ledger.** OTel
   GenAI standardizes per-agent *token* attribution but no cost metric; estimate-live +
   reconcile-authoritative (our exact two-tier design) is what Claude Code ships too.
   GitHub's June 1 2026 switch to token-based AI Credits makes per-subagent cost
   genuinely proportional — and Anthropic's "multi-agent ≈ 15× chat tokens" makes the
   per-subagent budget leash the highest-value control.

---

## 0. Ground truth — what the SDK + our code already give (local audit, highest confidence)

Verified directly in the vendored `github.com/github/copilot-sdk/go@v1.0.0` and our tree:

- **Subagent lifecycle events exist and are normalized.** The SDK emits
  `SubagentStarted/Completed/Failed` (also `Selected/Deselected`); `Started` carries
  `ToolCallID` (parent invocation), agent name/display-name/description, model;
  `Completed`/`Failed` add `DurationMs`, `TotalTokens`, `TotalToolCalls`, error
  (`rpc/zsession_events.go:1309-1367`). Our `normalize.go:144-159` already maps these to
  `EvSubagentStart/End`, and the chat shows a lifecycle-only activity indicator
  (`server.go:74` `subagents []copilot.SubagentInfo`, `renderSubagents`).
- **Every event is attributable to a subagent instance.** The `rpc.SessionEvent`
  envelope has `AgentID *string` — "Sub-agent instance identifier. Absent for events
  from the root/main agent" (`rpc/zsession_events.go:19-20`). **Our normalizer ignores
  it**, so subagent deltas/tool-calls/usage interleave into the main transcript
  indistinguishably. Threading `AgentID` through `copilot.Event` is the keystone change.
- **Subagent streaming is on by default.** `IncludeSubAgentStreamingEvents` on
  create/resume defaults to `true` (`client.go:681-684`); off = only lifecycle events.
- **The escalation back-channel is natively buildable.** `copilot.DefineTool` registers
  app-process tool handlers (`definetool.go:31`) — an orchestrator-provided
  `escalate` / `report_status` / `ask_orchestrator` tool can emit a UI event and block
  on a one-shot channel (the `permBridge` pattern) until the human/orchestrator answers,
  returning the instruction as the tool result. Zero SDK changes needed.
- **The sync↔async bridges already exist for the SDK-native asks.**
  `OnPermissionRequest` / `OnUserInputRequest` / `OnElicitationRequest` are bridged in
  `handlers.go` (perm/input/elicit bridges) — these fire for lane sessions today.
- Not relevant despite the name: `SessionHandoffData` is remote-session transfer
  (host/repo metadata), not subagent steering.

Two architectures are therefore on the table, and they compose rather than compete:

| | **(a) SDK-native custom agents** (`CustomAgentConfig` per session) | **(b) lane-as-subagent** (one SDK session per sub-agent — today's workflow lane) |
|---|---|---|
| Spawning | runtime auto-delegates (`infer`), or the model picks | orchestrator decides (workflow step / explicit spawn) |
| Visibility | lifecycle events + `AgentID`-tagged stream within the parent session | full session: own timeline, permissions, abort — already built |
| Control | none mid-run (one-way stream) | `Send`/`Abort` per session; custom tools for escalation |
| Cost | `TotalTokens` at end; `AgentID`-tagged `EvUsage` live | per-session usage events → meter/ledger (ADR-0018 tags exist) |

**Recommendation:** (b) stays the chassis for *first-class* sub-agents (control,
pause, per-session cost); (a) is rendered as in-chat child activity using the same new
`AgentID`-aware vocabulary — the live view must handle both.

---

## 1. Communication & escalation — model it as a task state machine

Verified claims (sources at end of section):

- **A2A (v1.0.0)** defines eight task states — `submitted, working, completed, failed,
  canceled, rejected, input-required, auth-required` — where `input-required`/
  `auth-required` are explicitly **non-terminal "interrupted" states**; the remote agent
  signals blockage by transitioning to `input-required` with a message, and the client
  resumes by sending a new message with the **same taskId+contextId**. Updates stream as
  `TaskStatusUpdateEvent` over SSE (plus polling and webhooks). [A2A spec]
- **OpenAI Agents SDK:** a tool marked `needs_approval` pauses the run;
  `RunResult.interruptions` surfaces approval items, resumed via
  `state.approve(...)/reject(...)` + re-run. Interruptions **surface uniformly on the
  outermost run regardless of nesting depth** (through handoffs and nested
  agents-as-tools); `RunState` serializes (`to_json`/`fromString`) for resume in another
  process. [OpenAI HITL docs]
- **LangGraph supervisor:** handoffs are tools returning
  `Command(goto=<target>, graph=Command.PARENT)`; **control returns to the supervisor
  after every worker finishes** — escalation is a routing event, not peer chat.
  [langgraph-supervisor]
- **Claude Code Task-tool subagents can only report results to the parent — they cannot
  message each other** (the newer *agent teams* feature adds peer mailboxes; Task-tool
  subagents remain parent-report-only). Anthropic's production multi-agent research
  system is **synchronous** orchestrator-worker — "the lead agent can't steer subagents"
  — with async steering named as future work, alongside its headline results
  (multi-agent beat single-agent Opus 4 by 90.2% on their internal eval). [Anthropic
  multi-agent post; Claude docs]
- **The Copilot SDK's native custom agents stream one-way lifecycle events only** — the
  docs define no subagent-initiated pause/escalate channel (community request #301
  closed under "Public Preview"). [GitHub custom-agents docs]
- Cautionary tale (verified verbatim): Claude Code issue **#47936** — background
  Task-tool subagents terminate early in ~14–30% of runs **yet report `completed` to
  the parent**. Don't trust a lifecycle event alone; cross-check (e.g. tokens > 0,
  output non-empty) before rendering "done".

**Design takeaway.** Add an interrupted-but-not-terminal state to the lane state machine:
`running → input-required | escalated → running | canceled | failed | done` (today lanes
settle only `done/failed/skipped`, ADR-0021). The orchestrator — not the lane — owns the
pause ledger (OpenAI's outer-run invariant). The back-channel is an orchestrator-injected
custom tool whose handler blocks until resolution. Ship worker→orchestrator (escalate,
report-status) before orchestrator→worker mid-run steering; keep "control returns to the
supervisor" as the invariant so the branching-predicate model survives intact.

Sources: [A2A specification](https://a2a-protocol.org/latest/specification/) ·
[OpenAI Agents HITL (py)](https://openai.github.io/openai-agents-python/human_in_the_loop/) /
[(js)](https://openai.github.io/openai-agents-js/guides/human-in-the-loop/) ·
[langgraph-supervisor-py](https://github.com/langchain-ai/langgraph-supervisor-py) ·
[Anthropic multi-agent research system](https://www.anthropic.com/engineering/multi-agent-research-system) ·
[Claude Code subagents](https://code.claude.com/docs/en/sub-agents) /
[agent teams](https://code.claude.com/docs/en/agent-teams) ·
[Copilot SDK custom agents](https://docs.github.com/en/copilot/how-tos/copilot-sdk/use-copilot-sdk/custom-agents) ·
[claude-code#47936](https://github.com/anthropics/claude-code/issues/47936)

## 2. HITL pause / continue / cancel — generalize the permission bridge

- **The universal shape** is the one we already run: a blocking callback resolved by an
  out-of-band action over a one-shot channel — Claude SDK `canUseTool` ("pauses
  execution until you return a response", can stay pending indefinitely), OpenAI
  `interruptions` + serialized `RunState`, Restate awakeables (suspend until
  `resolveAwakeable(id)`), Temporal signal + `wait_condition(timeout)`. [respective docs]
- **Resume semantics are the trap.** LangGraph's `interrupt()` **re-executes the whole
  interrupted node from its start** on resume (everything before the pause must be
  idempotent), has **no built-in timeout** ("waits indefinitely"), matches multiple
  interrupts by index, and cancelling a run can lose streamed state not yet
  checkpointed (issue #5672). Our design — pause *between* turns / inside a blocking
  tool handler — avoids replay entirely, but the idempotency invariant should be
  documented now. [LangGraph interrupts docs]
- **Cancel needs two verbs.** LangGraph Platform distinguishes `interrupt` (stop,
  preserve checkpoints, resumable) from `rollback` (stop + delete). For us: **cooperative
  cancel** = resolve the pending pause with "cancel" so the subagent's turn ends cleanly
  and the lane records why; **hard abort** = existing `Client.Abort` (ADR-0024) stays the
  escalation path.
- **Timeouts:** Temporal's pattern — SLA timer on each pause, escalate then default —
  is the production norm; indefinite waits are fine for an attended UI but leak
  goroutines/budget in unattended (autopilot-mode) runs.
- **Structured input has a ready wire contract:** MCP elicitation (spec 2025-06-18) —
  flat-primitive schema + three-action `accept/decline/cancel` — which our
  `ElicitRequest` already models. (Amended claim: client adoption was uneven through
  2025, but Claude Code ≥2.1.74 now advertises elicitation; VS Code/Copilot were the
  early complete implementations.)
- **Idempotent resolution is mandatory:** duplicate POSTs and a concurrent run-abort
  must not double-resolve a pause (CAS/`sync.Once` on the pause record) — the exact race
  Temporal guards with idempotent signal handlers.
- **Crash-orphaned pauses** are the only real gap vs. durable-execution engines: our
  channels are in-memory, so a restart orphans every pause. Persisting pause records +
  SDK session-resume reconstructs the wait without adopting Temporal/Inngest.
- Validation of the orchestrator-routed design: even Claude's SDK **does not allow
  `AskUserQuestion` inside subagents** — subagent-initiated human questions must route
  through the parent. [Claude user-input docs]

Sources: [Claude Agent SDK user input](https://code.claude.com/docs/en/agent-sdk/user-input) ·
[LangGraph interrupts](https://docs.langchain.com/oss/python/langgraph/interrupts) /
[cancel run](https://docs.langchain.com/langsmith/cancel-run) /
[#5672](https://github.com/langchain-ai/langgraph/issues/5672) ·
[Temporal HITL](https://temporal.io/blog/human-in-the-loop-approvals) ·
[Restate awakeables](https://docs.restate.dev/develop/ts/awakeables/) ·
[Inngest waitForEvent](https://www.inngest.com/docs/features/inngest-functions/steps-workflows/wait-for-event) ·
[MCP elicitation](https://modelcontextprotocol.io/specification/2025-06-18/client/elicitation) ·
[OpenAI guardrails & approvals](https://developers.openai.com/api/docs/guides/agents/guardrails-approvals)

## 3. Event vocabulary & SSE wiring — three additions, no new transport

- **No single standard won, but all three converge** (AG-UI, Vercel AI SDK UI streams,
  A2A): run lifecycle boundaries, start/delta/end tool streaming, **parent/thread IDs
  for attribution**, and an explicit "input required" state. AG-UI attributes via
  `runId`/`threadId`/`parentRunId`; the Claude SDK tags every streamed event with
  `parent_tool_use_id` when from a subagent; LangSmith's run tree uses `parent_run_id`.
  Our SDK's envelope `AgentID` is the same parent-pointer pattern.
- **SSE remains the right transport** (2025–2026 practice: SSE for one-way token
  fan-out; WebSockets only for true bidirectionality — and our bidirectional needs are
  already htmx POSTs). AG-UI is explicitly transport-agnostic with SSE first-class.
- **htmx mechanics confirmed:** one `sse-connect` stream supports many child listeners,
  each `sse-swap` taking comma-separated named events; **unlistened named events are
  silently dropped** (so the subagent-list container must exist and listen before
  per-agent events fire); `hx-swap-oob` inside SSE payloads is *empirically working but
  undocumented* (PR #94 closed unmerged) — we already rely on OOB swaps, so keep
  full-fragment idempotent re-renders as the foundation (reconnect-safe; SSE replays
  nothing) and append-mode only for transcripts.

**Minimal vocabulary extension** for `copilot.Event` (additive, mock-expressible):

1. `AgentID string` on the envelope — attribution for *existing*
   `EvMessageDelta/EvToolStart/EvUsage/EvPermission/...` (empty = root agent).
2. Lifecycle already exists (`EvSubagentStart/End`); extend `SubagentInfo` with a
   status enum borrowed from A2A: `working | input-required | done | failed | canceled`.
3. A coalesced low-frequency `EvAgentStatus{AgentID, Activity, Credits}` — emit on
   tool-start and usage ticks (AG-UI ActivityDelta analogue), never per token.

The double-click overlay needs no new transport: htmx GET loads the subagent transcript
fragment into a dialog whose inner region carries its own named `sse-swap` listener on
the same `/events` stream; pause forms inside it reuse the inline permission-form POST
pattern.

Sources: [AG-UI events](https://docs.ag-ui.com/concepts/events) /
[architecture](https://docs.ag-ui.com/concepts/architecture) ·
[Vercel AI SDK stream protocol](https://ai-sdk.dev/docs/ai-sdk-ui/stream-protocol) ·
[Claude SDK streaming output](https://code.claude.com/docs/en/agent-sdk/streaming-output) ·
[htmx SSE extension](https://htmx.org/extensions/sse/) ·
[htmx-extensions PR#94](https://github.com/bigskysoftware/htmx-extensions/pull/94)

## 4. Live per-subagent cost — thread the tag, keep the two-tier meter

- **OTel GenAI conventions** standardize per-agent *token* attribution
  (`gen_ai.agent.id/name`, `invoke_agent` spans, `gen_ai.client.token.usage` histogram)
  but **no cost/USD metric** — cost stays our own ledger field. All gen_ai conventions
  are still "Development" stability.
- **Claude Code's schema is the one to copy:** metrics carry
  `query_source ∈ {main, subagent, auxiliary}` + `agent.name`; traces carry
  `agent_id`/`parent_agent_id` so a delegation chain rolls up like a LangSmith trace
  tree (per-child rows, aggregated parents, trace total). And its docs explicitly label
  live cost "approximations… refer to your API provider" — i.e. even Anthropic ships
  estimate-live + reconcile-authoritative, our exact ADR-0033 split.
- **Billing regime shift (verified against the primary announcement):** from
  **June 1, 2026** GitHub replaces premium request units with **AI Credits consumed by
  token usage** ($10 Pro / $39 Pro+ / $19 Business / $39 Enterprise included monthly;
  transition nuances for annual plans). Under the legacy model a coding-agent session
  cost a flat 1 PR — now per-subagent spend is genuinely token-proportional, which makes
  live per-subagent metering both more accurate and more necessary.
- **Anthropic's token economics:** agents ≈ 4× chat tokens, **multi-agent ≈ 15× chat
  tokens**; "token usage by itself explains 80% of the variance" in their eval. The
  runaway subagent is the dominant cost-risk mode → per-subagent **budget leash**
  (max credits + max turns, LiteLLM-style `max_budget`+window shape — noting LiteLLM's
  own enforcement bugs argue for enforcing at *our* gate, pre-`Send`) is the
  highest-value control. Counterpart finding: OpenAI's SDK has **no per-agent usage
  breakdown** (request closed *wontfix*) — our ledger tags are ahead of the field here.
- **Closest UX prior art:** the `copilot-cli-cost` extension renders a per-session
  statusline (`💸 Cost ~$0.31 (30.6 cr, 2% pro) · last 42K in/3K out`) from the same
  Copilot CLI session RPC (`session.rpc.usage.getMetrics()`) — confirming a live
  per-session pull path exists in our substrate, complementing the push-side
  `AgentID`-tagged `EvUsage`.

Implementation here is small because ADR-0018 did the heavy lifting: add a `SubagentID`
(or reuse the lane tag) on `SpendRecord` (additive schema v3), route `AgentID`-tagged
`EvUsage` through `recordUsage` into a per-subagent `sessionMeter` clone, and surface
`AgentShares`-style rollups. Demo mode: `MockClient` emits tagged usage so the whole
surface works offline.

Sources: [OTel GenAI agent spans](https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-agent-spans/) /
[metrics](https://opentelemetry.io/docs/specs/semconv/gen-ai/gen-ai-metrics/) ·
[Claude Code monitoring](https://code.claude.com/docs/en/monitoring-usage) ·
[GitHub usage-based billing](https://github.blog/news-insights/company-news/github-copilot-is-moving-to-usage-based-billing/) ·
[coding agent 1 PR/session](https://github.blog/changelog/2025-07-10-github-copilot-coding-agent-now-uses-one-premium-request-per-session/) ·
[Anthropic multi-agent](https://www.anthropic.com/engineering/multi-agent-research-system) ·
[copilot-cli-cost](https://github.com/DamianEdwards/copilot-cli-cost) ·
[LiteLLM budgets](https://docs.litellm.ai/docs/proxy/users) ·
[openai-agents-python#2100 (wontfix)](https://github.com/openai/openai-agents-python/issues/2100) ·
[LangSmith cost tracking](https://docs.langchain.com/langsmith/cost-tracking)

## 5. UX — the field converged on the user's sketch

Product prior art (each verified against docs/changelogs):

- **Devin:** session sidebar with persistent per-session text labels ("PR created",
  "Awaiting instructions", "Approve session") + timestamps; **orange favicon dot when
  waiting on you**, green when working. MultiDevin: each worker gets its own session
  link to open/inspect/message; the coordinator monitors **per-child ACU (cost)** and
  can sleep/terminate children — the direct precedent for per-subagent live cost.
- **Cursor 3 (Apr 2, 2026):** the Agents Window — every agent session (local, cloud,
  worktree, SSH) in one control surface, tabs side-by-side or grid.
- **GitHub mission control (Oct 28, 2025):** task list with at-a-glance status, "jump in
  when Copilot needs your input"; **steering input applies after the current tool call
  completes** — a clean, copyable resume contract.
- **Claude Code agent teams:** teammates listed with what they're working on;
  Enter to view a teammate's session, **Escape to interrupt it**, Ctrl+T shared task
  list; transcript viewer collapses tool spam to "Called X 3 times" with expand
  (progressive disclosure).
- **Manus / Jules:** a side activity pane streaming live actions with mid-task
  intervention (Manus's Computer); per-session activity feeds from both user and agent
  (Jules API).
- **LangChain Agent Inbox:** an inbox of interrupted threads where each interrupt
  carries capability flags (`allow_accept/edit/respond/ignore`) that determine which
  action buttons render — adopt this instead of a bare "continue" button.
- UX research (HatchWorks): chat-only agent UX fails on invisible actions/unclear
  state/no pause control; prescribe a taskboard + collapsible activity timeline —
  i.e. don't bury subagents in the transcript.

**Status vocabulary (de facto standard, four states):** running/working (accent or
spinner) · **waiting-on-human (amber — the attention state)** · done · failed/stopped.
Map to our tokens: `--accent` pulse / `--warn` / `--good` / `--bad`, with a text label
next to the icon (Devin's labels beat icon-only for legibility). Out-of-band attention:
badge count on the list header + amber favicon/title dot once >2 agents run.

**For our htmx shell:** list + badges + overlay wins over tabs (Cursor) or split panes
(Claude Code tmux) — those pay off only with IDE/terminal real estate and would fight
the partial-swap model. Rows: status glyph + name + current activity (current tool) +
live credits. Overlay: `<dialog>` loaded by htmx GET (double-click *and* a visible
button — double-click alone is undiscoverable and has no touch equivalent), containing
the subagent's transcript region (own named `sse-swap`), the pause form when
`input-required` (capability-flagged buttons), and cooperative-cancel / hard-abort.
Collapsed one-line-per-tool-call with expand, à la Ctrl+O.

Sources: [Devin release notes](https://docs.devin.ai/release-notes) ·
[Devin manages Devins](https://cognition.ai/blog/devin-can-now-manage-devins) ·
[Cursor 3](https://forum.cursor.com/t/cursor-3-agents-window/156509) /
[Cursor 2.0](https://cursor.com/changelog/2-0) ·
[GitHub mission control](https://github.blog/changelog/2025-10-28-a-mission-control-to-assign-steer-and-track-copilot-coding-agent-tasks/) ·
[Claude Code agent teams](https://code.claude.com/docs/en/agent-teams) /
[interactive mode](https://code.claude.com/docs/en/interactive-mode) ·
[agent-inbox](https://github.com/langchain-ai/agent-inbox) ·
[Manus review](https://www.technologyreview.com/2025/03/11/1113133/manus-ai-review/) ·
[Jules API](https://developers.google.com/jules/api) ·
[HatchWorks agent UX patterns](https://hatchworks.com/blog/ai-agents/agent-ux-patterns/)

---

## Proposed epic shape (build order, each child shippable)

1. **S1 — `AgentID` attribution through the seam** (S/M). Envelope `AgentID` →
   `copilot.Event.AgentID`; normalizer threads it on every event; `MockClient`/demo emit
   tagged events. Pure seam change, no UI. *The keystone — everything below reads it.*
2. **S2 — sub-agent registry + live list** (M). Pure `convo`-style sub-agent state
   (id, name, status, current activity, credits) fed by S1 + existing
   `EvSubagentStart/End`; chat-side list partial with status glyph + label + activity +
   live credits, full-fragment SSE re-render (idempotent). Status icons per the 4-state
   vocabulary.
3. **S3 — per-subagent cost** (S/M). `AgentID`-tagged `EvUsage` → per-subagent meter +
   additive `SpendRecord` tag (schema v3, v2 reads back empty — the ADR-0018 pattern);
   rollup on Telemetry. Budget leash: max-credits/max-turns per sub-agent checked at the
   existing pre-`Send` gate.
4. **S4 — pause/continue/cancel (the pause record)** (M/L). Typed pause record
   {id, agentID, kind: input|issue|budget|permission, capability flags}; orchestrator-
   registered `escalate`/`report_status` custom tool (DefineTool) blocking on a one-shot
   channel; `input-required` lane state; POST continue(payload)/cancel; idempotent
   resolution (CAS); SLA timeout with default action; cooperative-cancel vs hard-abort.
   MCP-elicitation-shaped forms for structured input (reuse `ElicitRequest`).
5. **S5 — popup chat overlay** (M). `<dialog>` via htmx GET; per-agent transcript region
   with own named SSE listener; pause form inside; send-into-subagent for lane-backed
   agents (steering applies after current tool call — the GitHub contract).
6. **S6 — attention surface** (S). Badge count on the list, amber title/favicon dot when
   any agent is `input-required`; Runs/lane integration (`input-required` rendered
   distinctly, pause durations on the run record).

Risks to carry into ADRs: don't trust `SubagentCompleted` alone (claude-code#47936-class
bugs — cross-check tokens/output); unlistened named SSE events drop silently (render the
listening container before tagged events fire); OOB-in-SSE is empirically-working-but-
undocumented (keep idempotent full-fragment re-renders as the foundation); in-memory
pauses orphan on crash (persist pause records if/when unattended runs matter); pre-pause
idempotency invariant must be documented the day S4 lands.

## Verification appendix

14 contested claims → 3 independent adversarial verifiers (web): 13/14 unanimous
CONFIRMED; C8 (MCP elicitation — "Anthropic clients lagging") amended: true through
2025, but Claude Code ≥2.1.74 now advertises elicitation (the cited feature requests are
closed). Strengthening corrections: openai-agents-python#2100 closed *wontfix*;
Cursor "Composer" survives as a model name (the pane was replaced); GitHub AI-Credits
plan figures match the primary announcement but plan lineups have since shifted —
re-verify allowances before building budget UX against them. High-confidence claims not
re-verified were fetched directly from primary sources (specs, official docs,
changelogs); Anthropic's 90.2%/15× figures were independently fetched by two research
passes.
