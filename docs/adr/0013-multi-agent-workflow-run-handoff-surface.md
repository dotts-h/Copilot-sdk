# 0013. Multi-agent workflow run / handoff surface

- Status: accepted
- Date: 2026-06-05
- Deciders: Horia
- Related: `internal/ctxforge` (`workflow.go` — `Workflow`/`WorkflowStep`/
  `Validate`/`CompileWorkflow`, forge CRUD + referential integrity), the seam
  (`internal/copilot` `Client.CreateSession`/`Send`/`Events`), `internal/web`
  (`workflow.go` — `workflowRun` state machine, lanes reducer, Workflows page,
  `session.go` run-branch, `templates/fragments.html`, `static/app.css`,
  `bootstrap` seed, `demo.go`), `docs/NEXT_FEATURES.md` item 2.1,
  [ADR-0003](0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md),
  [ADR-0001](0001-render-markdown-server-side-for-committed-agent-turns.md)

## Context

The product is named *my-orchestra* but exposed no orchestration. Sub-agent
events (`EvSubagentStart/End`) were normalized and shown as a background chip
strip, but there was **no control surface** to compose or run multiple agents.
The forge already compiles distinct agent personas deterministically
(`Forge.Compile` → `SessionSpec`, ADR-0003), so the missing piece is a way to
chain or fan those personas out and watch each run.

Item 2.1 asks for a small workflow — pick an agent, hand off to another on
completion, or fan out to parallel agents — with each run watched as its own
**lane** in the timeline. The roadmap is explicit: **lead with this ADR**, and
**start with sequential handoff (lowest risk), then parallel.**

Four questions had to be answered: **where the workflow model lives and how it
Validates**, **how a sub-run maps onto the seam's session lifecycle**, **how an
event is attributed to a lane**, and **how the lanes render**.

## Considered options

- **Where the workflow/handoff model lives.**
  - **A pure `ctxforge.Workflow` (chosen).** A workflow is an ordered list of
    `WorkflowStep{AgentID, Prompt}` plus a `Mode` (sequential | parallel). It is a
    first-class forge entity alongside Skill/Instruction/Agent/MCPServer: file-
    backed JSON, slug id, `Validate()`, whole-forge uniqueness, and **referential
    integrity** (each step's `AgentID` must resolve to a real agent — exactly like
    an agent's pinned-skill references, and the built-in `chat` agent resolves). A
    new `CompileWorkflow(id)` reuses the existing per-agent `Compile` to produce one
    `SessionSpec` per step, deterministically. Domain logic stays pure and is
    unit-tested with no browser and no client.
  - **A web-only/run-time concept.** Rejected: it would split the "what context an
    agent runs with" model across two packages and lose the forge's reproducibility,
    determinism, and validation discipline.

- **How a sub-run maps onto the seam.**
  - **One session per step on the existing `Client` (chosen).** Each lane is a
    sub-run = one `CreateSession(stepSpec) + Send(prompt)` on the same seam a normal
    turn uses; its events arrive on the same `Events()` stream and are bound back to
    the originating `Server` (`Hub.bind`). No new seam method, no parallel client.
    Sequential mode launches the next lane only when the current one goes idle,
    appending the previous lane's output as a handoff; parallel mode launches every
    lane at once.
  - **A new multi-session seam / batch API.** Rejected — the seam already opens and
    drives independent sessions; a batch method would duplicate that for no gain and
    break the "no SDK import outside the seam / one stream" invariants.

- **How an event is attributed to a lane.**
  - **By copilot session id, with a sole-running fallback (chosen).** While lanes
    run concurrently (parallel), the only safe key is the event's `SessionID` →
    `lane.sessionID`. When a `MockClient`/offline event carries no `SessionID` (and
    a sequential run has exactly one running lane), it routes to that lone running
    lane. This keeps the sequential path fully testable offline while staying
    correct for concurrent parallel lanes on the live SDK.
  - **A dedicated per-lane client/stream.** Rejected — over-engineered; the seam's
    single stream already carries `SessionID`.

- **How the lanes render.**
  - **A dedicated `#lanes` panel on the chat page (chosen).** The run renders as a
    panel — one card per lane with a status glyph (pending/running/done/failed), the
    step's agent, the streamed output (collapsible), and the lane's metered cost —
    streamed over a new `lanes` SSE event (an elevated cousin of the sub-agent
    strip). The main chat transcript is left untouched during a run (a system note
    marks start/finish), so lanes don't interleave with an unrelated chat turn.
  - **Interleave sub-run output into the chat timeline.** Rejected — it muddles two
    distinct surfaces and makes a parallel run unreadable.

## Decision

Add `ctxforge.Workflow`/`WorkflowStep` as a pure forge entity with `Validate`,
forge CRUD (add/update/remove, rollback-on-invalid), whole-forge referential
integrity (step → agent), and `CompileWorkflow` (per-step `SessionSpec` via the
existing `Compile`). In the web layer, a pure `workflowRun` state machine owns the
lanes and the sequential cursor — `start`/`handoffPrompt`/`finishLane`/`failLane`/
`laneFor` mutate only the run and return the lane indices to launch next, with no
IO, so the engine is unit-tested for both modes with no client. The `Server` is
the thin adapter: `handleWorkflowRun` compiles the workflow under `forgeMu`, builds
the run, and launches the first lane(s); a run-branch at the top of `handleEvent`
routes a sub-run's events to its lane (output, metered cost, advancement) and emits
the `lanes` SSE fragment; on completion it clears `busy` and notes the outcome. A
new **Workflows** nav page provides CRUD (mirroring Agents) plus a **▶ run**
control. The offline demo seeds a sequential **Build & harden** workflow and
scripts each lane so the surface is exercised end-to-end in the browser suites.
**Sequential handoff ships fully; parallel is implemented in the model, the engine,
and the run wiring** (lane attribution by `SessionID`), with the offline demo
covering sequential and the engine's parallel logic unit-tested directly.

## Consequences

- Positive: the product finally exposes orchestration — chain agents with a handoff
  or fan them out — on top of the existing forge/seam with **no new seam method**
  and **no SDK import** on the consumer side. The workflow model is pure,
  deterministic, validated, and reproducible (it round-trips through `forge.json`);
  the run engine is a pure state machine unit-tested without a browser. Per-lane
  cost folds into the same account-wide + per-session meters and the spend ledger,
  so multi-run spend is accounted exactly like a normal turn.
- Escaping: agent names and lane output are model/forge-originated, so they are
  HTML-escaped before the browser like all model text (committed lane output goes
  through the same server-side markdown renderer as a chat turn; names/detail
  through `richtext`/auto-escaping) — ADR-0001.
- Interaction: a workflow run and a normal chat turn are mutually exclusive (both
  gated by `busy`); a prompt typed during a run is refused with a note rather than
  queued, so the run owns the turn cleanly. `/clear` resets the run.
- New nav page: **Workflows** is added to `pageNames` and the e2e `pages` array in
  nav order (the documented `pages.length`/nav-count coupling).
- Trade-off we accept: lane events other than message/usage/idle/error from a
  sub-run (its reasoning, tools, permissions) are **not** surfaced per-lane in this
  first cut — a lane shows its output and cost. Surfacing a sub-run's tool timeline
  per lane, and a richer parallel-lane demo, are tracked follow-ups (TECH_DEBT).
- Contract change: `ctxforge.Workflow`/`WorkflowStep` is a new persisted schema
  (additive `workflows` key on `forge.json`, omitempty so older files read clean);
  the `POST /workflows…` route group + `POST /workflows/{id}/run` and the `lanes`
  SSE event are new — recorded in CONTRACTS.
