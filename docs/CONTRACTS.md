# CONTRACTS.md — stable promises between components

> A contract is a promise one component makes to another: an interface signature, an
> event name and shape, a route, a persisted schema, an invariant. This registry makes
> changing them **deliberate** — see who depends on a promise before breaking it.
> Distinct from [ARCHITECTURE.md](ARCHITECTURE.md) (where code *is*); this is what code
> *guarantees*. Stability: `stable` (needs an ADR to change) · `internal` (move freely) ·
> `experimental`.

Extracted from code 2026-06-04 (`internal/copilot/copilot.go`, `internal/web/hub.go`,
`internal/ctxforge/`). Re-run `registering-contracts` after any seam/route/schema change.

## 1. The `copilot.Client` seam — `internal/copilot/copilot.go:219`

**Producer:** `SDKClient` (live runtime) and `MockClient` (tests/demo). **Consumer:**
`internal/web` (Hub/Server) and `internal/tui`. **Stability: stable** — the single seam;
no SDK import is allowed on the consumer side (see CONVENTIONS architecture rules).

| Method | Promise |
|--------|---------|
| `CreateSession(ctx, SessionSpec) (id, error)` | Open a session, return its id. |
| `Send(ctx, sessionID, prompt, attachments, agentMode) error` | Submit a turn; output arrives as `Events()`. `agentMode` ∈ {plan, autopilot, interactive, shell, ""}. |
| `Abort(ctx, sessionID) error` | Cancel the in-flight turn. Driven by the chat-turn abort (`handleAbort`) and, per running lane, by the run abort (`handleRunAbort`, V19/ADR-0024). |
| `Respond(id, approve) error` | Answer a pending `EvPermission`. |
| `RespondInput(id, answer) error` | Answer a pending `EvUserInput` (ask_user). |
| `RespondPlan(id, approved, action, feedback) error` | Answer a pending `EvPlanReview`. |
| `RespondElicit(id, action, content) error` | Answer a pending `EvElicitation`; `action` ∈ {accept, decline, cancel}. |
| `ListModels(ctx) ([]ModelInfo, error)` | Models available to the account. |
| `AuthStatus(ctx) (AuthStatus, error)` | The runtime's live credential (`auth.getStatus`): authenticated?, opaque method label, login, host. Read-only — the runtime exposes no auth write surface; changing the method is a config edit applied at the next dial. `MockClient` returns a settable canned status. — see [ADR-0039](adr/0039-connection-page-auth-method-via-config-status-via-auth-getstatus.md) |
| `ListSessions(ctx) ([]SessionMeta, error)` | Persisted sessions, most-recent first. |
| `ResumeSession(ctx, sessionID, SessionSpec) (id, error)` | Reattach to a persisted session, wiring the same handlers as `CreateSession`; runtime restores full history. — see [ADR-0002](adr/0002-restore-sdk-session-resume-for-session-pick-start-continue.md) |
| `SessionHistory(ctx, sessionID) ([]Event, error)` | A session's conversation as normalized events, for rebuilding the transcript. |
| `DeleteSession(ctx, sessionID) error` | Permanently remove a persisted session. |
| `Events() <-chan Event` | Single normalized-event stream until `Close`. |
| `Close() error` | Release all resources. |

**`SessionSpec`** (`copilot.go:198`): `Model, ReasoningEffort, SystemMessage, Streaming,
AutoApproveTools, MCPServers, AllowedTools, Hooks, Workspace`. `AllowedTools` empty = all tools;
otherwise maps to the SDK session's `AvailableTools`. — see [ADR-0003](adr/0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md)
`Hooks` (`[]ctxforge.Hook`) carries the session's compiled **governance policy** — the built-in
safe-by-default hooks (`ctxforge.DefaultHooks`, auto-approve reads), the built-in **mandatory**
dangerous-action ruleset (`ctxforge.DangerousHooks`), then the forge's **enabled** user hooks,
compiled in by `Forge.Compile` and threaded via `web.SeamSpec` (mirroring `MCPServers`). `Workspace`
is the session's **workspace root** (absolute; the process cwd, set by `bootstrap` and carried on
every `SeamSpec` path), threaded into `Evaluate` so the built-in fence can gate a write whose target
resolves outside it (empty = fence inert). The `SDKClient` records both per `SessionID` (a
`sessionPolicy{hooks, autoApprove, workspace}`); **`permissionHandler` is the only permission handler**
(the SDK's blanket `ApproveAll` is never wired) and consults `ctxforge.Evaluate` before the interactive
gate — a PreToolUse decision of `allow` returns `PermissionDecisionApproveOnce` (no `EvPermission`
emitted), `deny` returns `PermissionDecisionReject{Feedback: reason}`, and `ask` falls through to the
emit-and-block gate. Under `AutoApproveTools` the handler blanket-approves only the **non-mandatory**
remainder: a **mandatory** deny still rejects and a mandatory ask (e.g. `sudo`, an out-of-workspace
write) still gates **even with auto-approve set** — `Decision.Mandatory` drives this — so the dangerous
policy is **enforced in the bridge, unbypassable by config**. This generalizes the flat
`AutoApproveTools` into a per-tool ruleset; the override now sits **above the non-mandatory policy and
below every deny**. The seam imports `ctxforge` (the pure domain package) for the single shared `Hook`
type + evaluator — no SDK type crosses into `ctxforge`. The handler additionally records the
turn's **active agent mode** on the `sessionPolicy` (updated at `Send`) and threads it into
`Evaluate` (**mode binding**): a mode-scoped user hook participates only in its modes, while the
mandatory ruleset (unscoped) holds in every mode; `ctxforge.EffectiveAutoApprove(mode, config)`
resolves the non-mandatory auto-approve baseline (autopilot on / interactive off / else config).
Every non-gated decision is surfaced for the timeline "why": a `deny` and a **user** allow emit an
`EvToolDecision`, and a gated **ask** carries the hook `Reason` on its `EvPermission`. — see
[ADR-0029](adr/0029-hooks-forge-entity-bridge-enforced-allow-deny-ask-safe-read-defaults.md),
[ADR-0030](adr/0030-dangerous-action-deny-and-mandatory-hitl-unbypassable-by-config.md),
[ADR-0031](adr/0031-hooks-management-ui-mode-binding-and-timeline-why.md)
On the **PostToolUse** path the seam runs the **executor** (ADR-0032): when a tool call completes,
`ctxforge.PostToolUseCommands` selects the enabled PostToolUse hooks that carry a `Command` and match
the completed call, and the `SDKClient` runs each — `${VAR}` resolved via the `lookupEnv` seam
(default `os.Getenv`, an unset ref → empty, never the literal), the program exec'd **directly**
(`runCmd`, no shell) with the **workspace** as cwd, a 5s timeout, and ~2KB of combined output
captured. The command's stdout/stderr is **UNTRUSTED**: it is emitted only as an `EvHookRun`
annotation, never fed back to the agent and never consulted on the permission path — a post-tool
command can **never** flip a decision. — see
[ADR-0032](adr/0032-posttooluse-hook-command-execution-untrusted-output.md)
`MCPServers` carries the forge's **enabled** servers (compiled in, translated via
`web.MCPServerSpecs`); each `copilot.MCPServer` registers under its unique `Key()`
(its `ID`, or `Name` for legacy callers) so a non-unique `Name` can't collide in the
SDK config map. `MCPServerSpecs` is also where each server's `Env` is resolved:
a value of the reference shape `${VAR_NAME}` is expanded via a lookup seam
(default `os.Getenv`) and a reference that resolves empty is left **unset** (never
forwarded as the literal `${VAR_NAME}`). — see
[ADR-0010](adr/0010-mcp-server-management-page-curated-defaults-disabled-with-preflight.md),
[ADR-0020](adr/0020-mcp-secrets-via-env-var-reference-indirection.md)

## 2. Normalized event vocabulary — `EventType` (`Ev*`)

**Producer:** `SDKClient` normalizes SDK events into these; `MockClient` emits them directly.
**Consumer:** the Hub's `pump` routes each by `SessionID`; the Server's reducer renders SSE
fragments. **Stability: stable** — the wire vocabulary between runtime and UI.

`EvMessage`, `EvMessageDelta`, `EvReasoning`, `EvReasoningDelta`, `EvToolStart`,
`EvToolProgress`, `EvToolEnd`, `EvUsage`, `EvContextWindow`, `EvPermission`, `EvUserInput`,
`EvUserMessage`, `EvPlanChanged`, `EvPlanReview`, `EvElicitation`, `EvCompactionStart`,
`EvCompactionEnd`, `EvSubagentStart`, `EvSubagentEnd`, `EvToolDecision`, `EvHookRun`, `EvError`,
`EvIdle`, `EvUnknown`. `EvToolDecision` (ADR-0031) is the **timeline "why"** annotation: a governance
decision the bridge made *without* a gate (a `deny` or a user `allow`), reduced into a compact,
muted `convo.RoleDecision` turn — not a control. `EvHookRun` (ADR-0032) is the **PostToolUse
command** annotation: a hook ran an external command after a matching tool completed, reduced into a
compact `convo.RoleHookRun` turn carrying the hook id + a bounded, **escaped**, UNTRUSTED output
snippet — display-only telemetry, never fed back to the agent and never a gate.

**`Event` shape** (`copilot.go`): `Type, SessionID, AgentID, Text, Tool, ToolCall*, Usage, Context,
Permission*, Input*, Plan*, Elicit*, Subagent*, Decision*, HookRun*, Err`. Pointer fields are set only for
the matching event type (e.g. `Permission` for `EvPermission`, `Decision` for `EvToolDecision`,
`HookRun` for `EvHookRun`). **`AgentID`** (epic 0069 S1, [ADR-0040](adr/0040-subagent-identity-instance-agentid-vs-spawn-toolcallid.md))
is the **sub-agent instance** tag, threaded from the SDK envelope (`sdk.SessionEvent.AgentID`) onto
**every** normalized event type — **empty for the root/main agent and session-level events, i.e. every
pre-existing path (additive, no consumer breaks)**. The reducer **routes** on it: an event with a
non-empty `AgentID` reaches ONLY the **sub-agent registry** (`convo.Subagents`, issue 0071 S2) —
its live list is the sub-agent's surface — and never mutates the root transcript or meters the root
agent's spend (the S1 invariant). It is the **instance** key; `SubagentInfo.ToolCallID` is the
**spawn** key; the two join at the registry by chronological first-tag-after-start (ADR-0040). The
lifecycle events (`EvSubagentStart`/`End`, `AgentID` empty) register/settle the entry. **`HookRun`** (`copilot.go`): `HookID, Command, Output, ExitCode,
TimedOut, Failed` — the resolved command line + its bounded untrusted output; `Output` is rendered
through `html/template` auto-escaping (ADR-0001), never `trusted()` raw.
`PermissionRequest` gained a `Reason` (the gating hook's message, shown on the gate). `SessionID` is empty for
a bare `MockClient.Emit` (the chat demo); a single-session consumer may ignore it.
**Exception:** the workflow-lane demo (`web.streamDemoLane`) tags every emitted
event with its lane's backing session id so a **parallel** run's concurrent lanes
are disambiguated by `SessionID` (`web.workflowRun.laneFor`) — paired with
`MockClient.CreateSession` now returning a **distinct** id per call. — see
[ADR-0017](adr/0017-per-lane-tool-and-permission-surface-for-parallel-workflow-lanes.md)

**`PermissionRequest`** (`copilot.go:50`): `ID, Kind, Detail` plus the write-only
`FileName, Intention, Diff` — set from `sdk.PermissionRequestWrite` for file-write
requests, empty for every other kind. The `Diff` (a unified diff) feeds the diff
review lane: `web.renderPermForm` renders the richer `permReview` form when the
diff parses, falling back to the compact form otherwise. Additive/backward-compatible.
— see [ADR-0012](adr/0012-diff-review-lane-for-file-write-permissions.md)

**`SubagentInfo`** (`copilot.go:79`): `ToolCallID, Name, DisplayName, Description, Model,
Success, Detail, TotalTokens` — carried by `EvSubagentStart`/`EvSubagentEnd`. `TotalTokens` is the
completion's raw reported token count beside the display-formatted `Detail`: the registry
cross-checks it (with the observed stream) before trusting a "completed", so a zero-token,
unobserved success renders **done (unverified)** (claude-code#47936). The web layer's sub-agent
**live list** (`web.renderSubagents` → `subagentRow`, issue 0071) shows one row per registry entry —
textual status label (working / input required / done / failed) + glyph, `DisplayName` + `Model`,
current activity, and credits — and surfaces `Description` as the row's `title=` tooltip; an empty
`Description` renders no `title`. Finished entries STAY listed with their terminal status. All
row values are model/SDK-originated text and flow through `html/template` auto-escaping
(ADR-0001), never `trusted()` raw.

**Invariant:** every SDK-event → normalized-`Event` mapping has a test; `EvUnknown` is the
total fallback (no SDK event is dropped silently).

## 3. HTTP routes — `internal/web/hub.go` `Handler()`

**Producer:** `Server` handlers. **Consumer:** the htmx frontend. Routing is **per-cookie**:
each route resolves the requesting browser's `Server` before dispatch. **Stability: stable**
for the streaming/turn routes (`/events`, `/send`, the `/perm|ask|plan|elicit/{id}` answers);
`internal` for CRUD pages.

| Group | Routes |
|-------|--------|
| Core | `GET /` · `GET /events` (SSE) · `POST /send` · `POST /abort` |
| Turn answers | `POST /perm/{id}` · `POST /ask/{id}` · `POST /plan/{id}` · `POST /elicit/{id}` · `POST /budget/{action}` |
| Navigation | `GET /page/{name}` · `GET /commands` · `GET /static/…` |
| Telemetry | `GET /telemetry/export.csv` · `GET /runs/export.csv` |
| Runs | `POST /runs/rerun/{workflow}` · `POST /run/abort` |
| Skills | `GET /skills/new` · `GET /skills/{id}/edit` · `POST /skills` · `POST /skills/{id}` · `POST /skills/{id}/toggle` · `POST /skills/{id}/delete` |
| Instructions | `POST /instructions/import` · `GET /instructions/new` · `GET /instructions/{id}/edit` · `POST /instructions` · `POST /instructions/{id}` · `POST /instructions/{id}/toggle` · `POST /instructions/{id}/delete` |
| Agents | `GET /agents/new` · `GET /agents/{id}/edit` · `POST /agents` · `POST /agents/{id}` · `POST /agents/{id}/select` · `POST /agents/{id}/delete` |
| MCP servers | `GET /mcp/new` · `GET /mcp/{id}/edit` · `POST /mcp` · `POST /mcp/{id}` · `POST /mcp/{id}/toggle` · `POST /mcp/{id}/delete` |
| Workflows | `GET /workflows/new` · `GET /workflows/{id}/edit` · `POST /workflows` · `POST /workflows/{id}` · `POST /workflows/{id}/run` · `POST /workflows/{id}/delete` |
| Snippets | `GET /snippets/new` · `GET /snippets/{id}/edit` · `POST /snippets` · `POST /snippets/{id}` · `POST /snippets/{id}/delete` |
| Hooks | `GET /hooks/new` · `GET /hooks/{id}/edit` · `POST /hooks` · `POST /hooks/preflight` · `POST /hooks/command-preflight` · `POST /hooks/{id}` · `POST /hooks/{id}/toggle` · `POST /hooks/{id}/delete` |
| Sessions | `POST /sessions/new` · `POST /sessions/{id}/resume` · `POST /sessions/{id}/delete` |
| Settings | `POST /settings` · `POST /models/{id}/select` · `POST /effort/{value}/select` |
| Connection | `POST /connection` |

`/instructions/import`, `/agents/{id}/select`, and the `/sessions/*` routes are the
phase-2–4 additions. — see [ADR-0002](adr/0002-restore-sdk-session-resume-for-session-pick-start-continue.md), [ADR-0003](adr/0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md)

The `/mcp…` group (item 2.2) closes MCP-server CRUD, the last forge entity without
a UI; it mirrors the skills/agents routes. — see [ADR-0010](adr/0010-mcp-server-management-page-curated-defaults-disabled-with-preflight.md)

The `/hooks…` group (G4) is governance-hook CRUD (mirroring MCP): the page lists the
**read-only** built-in policy (`DefaultHooks` + the mandatory `DangerousHooks`) plus full user
add/edit/toggle/delete, with a **mode-binding** checkbox set and a pattern **preflight**
(`POST /hooks/preflight` calls `ctxforge.MatchPattern`/`PatternIsGlob` against a sample command,
mutating nothing). `POST /hooks/command-preflight` (G5) is its sibling for the **PostToolUse
command** field: it reports the resolved command line and which `${VAR}` references resolve empty
(behind the `s.lookupEnv` seam, ADR-0020), **never executing** the command. — see
[ADR-0031](adr/0031-hooks-management-ui-mode-binding-and-timeline-why.md),
[ADR-0032](adr/0032-posttooluse-hook-command-execution-untrusted-output.md)

The `/workflows…` group (item 2.1) is workflow CRUD (mirroring agents) plus
`POST /workflows/{id}/run`, which **starts a multi-agent run**: it compiles the
workflow's steps into one `SessionSpec` each and launches them as **lanes** —
sequential handoff (each lane's output feeds the next) or parallel fan-out — each a
sub-run on the seam's `CreateSession`/`Send` lifecycle. Run progress streams over
the `lanes` SSE event; the run response lands the user on the chat page where the
`#lanes` panel renders. Each lane card surfaces its **own** tool timeline and
inline file-write permissions (reusing the chat `renderToolCard`/`renderPermForm`),
not just output + cost; a parallel run drives concurrent lanes offline (distinct
mock session ids + `SessionID`-tagged demo events). — see
[ADR-0013](adr/0013-multi-agent-workflow-run-handoff-surface.md),
[ADR-0017](adr/0017-per-lane-tool-and-permission-surface-for-parallel-workflow-lanes.md)

`POST /runs/rerun/{workflow}` (item V18) is the **first action on the orchestration
history surface**: a recorded run's `↻ rerun` control re-executes its workflow's
**current** definition (looked up by `WorkflowID`) under the **same** `WorkflowID`, so the
new run rolls up under the same per-workflow totals / aggregates / reconciliation — a
re-execution, **not** a historical replay. It reuses the **same** run-trigger as
`POST /workflows/{id}/run`: both call one shared `launchWorkflow(id)` (extracted from
`handleWorkflowRun`), so there is exactly one orchestration path (one `!s.busy` guard, one
`forgeMu → s.mu` lock order, one `copilot.Client` lifecycle — **no new seam to the
runtime**). The control is gated on the workflow still existing (`CanRerun` — an orphan run
shows none) and refused while the server is busy; on success the user lands on the chat
page where the lanes stream. — see [ADR-0023](adr/0023-rerun-a-recorded-run-re-executes-the-current-workflow-definition.md)

`POST /run/abort` (item V19) is the **dual of rerun** and the second action on the
orchestration surface: a `⏹ stop run` control on the Chat lanes panel **stops the
in-flight run**. `handleRunAbort` → `abortRun` marks every not-yet-settled lane **failed**
(detail `⏹ aborted`), flips the run done+failed so the **same** `runFrags` completion path
records it once and clears `s.busy`, and aborts each still-running lane's backing session
over the existing `copilot.Client.Abort` seam (per lane) — **no new runtime seam, no new
lane status** (a stopped run is a **failed** run, so it rolls up under the same
aggregates/reconciliation as any failed run). A no-op when no run is in flight (the chat
page re-renders), so a racing double-click can't double-settle; the completion path is
guarded idempotent (`run.recorded`) because `laneError` — called from a lane goroutine that
bypasses the reducer's `!s.run.done` guard — can re-enter `runFrags` after the abort already
settled the run. The `stop-run` marker class is **disjoint** from the chat-turn `.abort`,
the Workflows `button.run`, the Runs `button.rerun`, and the `a.export` links. — see
[ADR-0024](adr/0024-abort-an-in-flight-run-settles-it-as-failed-and-aborts-its-lane-sessions.md)

The **Workflows page** (`GET /page/workflows`) lists each workflow (name + step
summary) with a run control — and, when history exists, **badges** each row (V4): the
**last-run** outcome glyph + relative age, a **run count**, and **total spend**.
`workflowsPartial` joins the two pure readers keyed by workflow id under `forgeMu` —
`RunAggregates(s.runs.Records())` (last-run signal + count) and
`WorkflowShares(s.spend.Records())` (per-workflow credits) — resolving nothing new (the
row already knows its own id/name). It is a **pure-reader composition** over the two
existing stores (no schema change, no new IO): a workflow with no runs and/or no spend
store wired renders its prior navigational shape, no badges; an id present only in a
store (since-deleted/renamed workflow) badges no row. Values flow through
`html/template` (ADR-0001). The decision is pre-blessed by the same cost ⋈ orchestration
convergence rationale as ADR-0022 / V1. — see
[ADR-0022](adr/0022-workflow-run-history-sibling-append-only-run-store.md),
[ADR-0018](adr/0018-additive-attribution-tags-on-spend-records.md)

The **Runs** view (`GET /page/runs`, item B3) is a read-only history of completed
workflow runs (most recent first) — each run's name, mode, outcome, when it ran, how
long it took (**duration**, V1), total metered cost, and a per-lane breakdown (agent,
settled status incl. **skipped**, credits) — with a **per-workflow summary table**
(run count, failure rate, **total & average cost** (V13), average duration, V1) above
the history. It carries a **time-window selector** (V12): an optional **`?window=` query
param** on `GET /page/runs` (threaded `handlePage` → `renderPage` → `runsPartial`, clamped
via the shared `clampWindow` to the allowed `{14,30,90}` set, default 14) slices the run
history to the records started within `window` days of the **most recent run** (a pure
`windowRuns`, tail-relative like the Telemetry trend) **before** both the summary roll-up
and the history list — so an out-of-window run is dropped from both. The selector renders
the same 14/30/90-day buttons (active one marked) that re-fetch `GET /page/runs?window=N`
into `#main`. A presentation-layer slice over the existing v1 records — no schema change,
no new reader in telemetry. It is a
query over the persisted `telemetry.RunStore` (§4) and its pure `RunAggregates`
roll-up; adding it as a top-level nav page bumps the `pageNames` / e2e `pages` count.
Below the per-workflow summary the page renders a **"Cost by lane"** share list (V14)
from `telemetry.LaneShares` (§4) — the per-lane cousin of `RunAggregates`, keyed by
(workflow, lane) and sorted by credits descending — resolving each lane's workflow/agent
ids to display names under `forgeMu` (like the run rows), so *which lane in a workflow
costs / fails most* reads at the finest grain. A pure reader + UI composition over the
existing v1 records — no schema change, no new store, no ADR.
A run is recorded **once on completion** by the web adapter (`workflow.go`
`recordRun`, where `runFrags` already clears `busy`). — see
[ADR-0022](adr/0022-workflow-run-history-sibling-append-only-run-store.md)

`POST /budget/{action}` (`action` ∈ {proceed, raise, cancel}) resolves a turn the
hard cap paused before `Send`: **proceed** dispatches the held prompt and keeps the
cap, **raise** lifts (disables) and persists the cap then dispatches, **cancel**
drops the turn. It is an **app-level** gate, not an SDK permission — distinct from
`/perm/{id}` despite reusing the inline-form look. — see [ADR-0008](adr/0008-budget-guardrails-soft-warn-and-hard-cap-gate.md)
The **same route** also resolves a **per-agent budget leash** gate (issue 0072): when
the paused turn's gate is tagged with an agent (`leashAgent`), **raise** lifts only
**that** persona's leash for the session (a transient override, not a forge edit; the
account cap is untouched), while proceed/cancel behave as above. One pause-resolve
shape, two ceilings (the account cap and the per-agent `telemetry.Leash`). — see
[ADR-0042](adr/0042-per-subagent-cost-attribution-and-the-budget-leash.md)

`POST /perm/{id}` answers a file-write permission identically whether it renders
as the compact form or the **diff review lane** (item 3.1): both post `approve=1|0`
to the same route. The review lane is a richer affordance on the existing seam,
**not** a new gate — distinct from `/budget/{action}` above, which is app-level
because it pauses before `Send`. — see [ADR-0012](adr/0012-diff-review-lane-for-file-write-permissions.md)
The same route also answers a **workflow-lane** permission (B1): `handlePerm` is
lane-aware — it drops the request from whichever lane holds it and refreshes
`#lanes` out-of-band instead of the chat timeline. Lane permissions are per-lane,
not a cross-lane FIFO. — see [ADR-0017](adr/0017-per-lane-tool-and-permission-surface-for-parallel-workflow-lanes.md)

The `/snippets…` group (item 3.4) is CRUD for the prompt/snippet library
(mirroring skills, **minus toggle** — a snippet is never compiled into a session).
There is no run/insert route: insertion rides the existing `GET /commands`
autocomplete (a snippet menu entry carries its body, inserted client-side by
`fillSnippet`) and `POST /send` (a bare `/trigger` expands-and-sends via
`snippetExpansion`; reserved command/page slugs always win). — see
[ADR-0015](adr/0015-prompt-snippet-library-forge-backed-composer-insertion.md)

`GET /telemetry/export.csv` streams the full persisted spend ledger as a CSV
attachment (header `at,session,model,input,cached,output,usd,credits,aiu,agent,workflow,lane`;
one row per metered turn). The `agent,workflow,lane` attribution columns are
**appended at the end** so the pre-v2 column positions are unchanged
(backward-compatible header bump). Empty (header only) when no ledger is wired. —
see [ADR-0009](adr/0009-persisted-spend-history-append-only-ledger.md),
[ADR-0018](adr/0018-additive-attribution-tags-on-spend-records.md)

`GET /runs/export.csv` (item V11) is the **orchestration sibling** of the spend
export: it streams the full persisted **workflow-run history** as a CSV attachment
(`telemetry.WriteRunsCSV` → header
`run,workflow,name,mode,startedAt,finishedAt,durationSeconds,outcome,lane,agent,status,credits`).
It flattens to **one row per lane** (run-level columns repeated on each), so a
branched run's **skipped** lane — which leaves no spend record, the reason the run
store exists alongside the ledger — is first-class in the export (something the spend
CSV can't carry). Empty (header only) when no run store is wired. A **pure-reader +
route** composition over the existing v1 run records — no schema change, no new store,
no ADR (pre-blessed by the ADR-0009 export precedent). — see
[ADR-0022](adr/0022-workflow-run-history-sibling-append-only-run-store.md),
[ADR-0009](adr/0009-persisted-spend-history-append-only-ledger.md)

`GET /telemetry/reconcile.csv` (item V17) is the **convergence** export sibling of the
spend and runs exports: it streams the cross-store reconciliation as a CSV attachment
(`telemetry.WriteReconcileCSV` → header `grain,workflow,lane,ledgerCredits,runCredits,delta`), so
the ledger-vs-runs **divergence** the Telemetry page surfaces can **leave the tool** for
outside analysis. **One file carries both grains:** the per-workflow rows
(`WorkflowReconcile`, V15) first, then the per-`(workflow, lane)` rows (`LaneReconcile`, V16),
each labelled by a leading **`grain` column** (`"workflow"` | `"lane"`) so a consumer filters
totals from breakdown on `grain` and never double-counts (the `lane` cell is blank on a workflow
row, the lane index on a lane row). Rows are the readers' own output, so ordering is
deterministic (biggest |delta| first within each grain); empty (header only) when no run store
is wired or there is nothing to reconcile. A **pure-writer + route** composition over the
existing reconciliation readers — no schema change, no new store, no ADR (pre-blessed by the
ADR-0009 export precedent). — see
[ADR-0009](adr/0009-persisted-spend-history-append-only-ledger.md)

The **Telemetry page** (`GET /page/telemetry`) reads the persisted ledger account-wide
(month-to-date budget rows, the spend-over-time trend, per-model / per-agent / per-workflow
shares — ADR-0009/0016/0018) plus the account-wide burn-rate **Forecast** line (ADR-0019). The
per-agent / per-workflow "Cost by …" share bars each carry a per-bucket burn **trajectory** (F3):
a `li.trajectory` line — *"at ~X cr/day, on pace for ~Y cr this month"* (or the Idle sentence, or
none when no budget) — built by `spendShares(now)` joining `telemetry.BucketForecasts` (§4) onto
each share row keyed by raw agent/workflow id under `forgeMu`, with **one** `now` threaded into
both the per-bucket `Forecast` and the month projection. It is a **pure-reader composition** over
the existing ledger (no schema change, no new IO); a wired-but-degenerate bucket (idle / too-new /
no budget) renders its explanatory sentence or no line, never a bogus date. Values flow through
`html/template` (ADR-0001). The **spend-over-time trend** carries a **window selector** (G3): an
optional **`?window=` query param** on `GET /page/telemetry` (threaded `handlePage` →
`renderPage` → `telemetryPartial` → `spendTrend`) chooses the trend's day window from the allowed
set **{14, 30, 90}**, **defaulting to 14** (the historical behavior) and **clamping** an
empty / unparseable / out-of-range value back to 14 (`clampWindow`). The selector renders as three
buttons (the active window marked) that re-fetch `GET /page/telemetry?window=N` into `#main`. The
trend slices `DailyTotals` to the chosen window **first**, **then** computes the bar-scaling
`maxUSD` over what's shown — the scaling stays **window-local** so an off-window all-time peak can
never shrink the visible bars (REGRESSIONS #14, now asserted per window). It is a presentation-layer
slice over the existing pure reader (no schema change, no new store). Below "Cost by workflow" the
page renders a **"Ledger vs runs"** reconciliation table (V15) from `telemetry.WorkflowReconcile`
(§4) — the pure cross-store reader that joins the per-workflow ledger spend (the `WorkflowShares`
grain) against the workflow's recorded-run spend (the `RunAggregates` grain) and surfaces their
**delta** — resolving workflow ids → display names under `forgeMu` (like the share rows) and
**ambering** a non-trivial delta (a turn metered outside a recorded run, or a run metered under a
different attribution). It renders only when **both** a spend store and a run store are wired (the
reconciliation needs two sides). Below it the page renders a **"Ledger vs runs by lane"** table
(V16) from `telemetry.LaneReconcile` (§4) — the **same** cross-store join one grain finer, per
`(workflow, lane)` — naming each row `"<workflow> · step <n>"` (n = lane index + 1, like the Runs
page's "Cost by lane") under `forgeMu` and ambering a non-trivial delta, so a divergence the
per-workflow row only totals is locatable at the exact step. Beside the "Ledger vs runs"
heading an **"Export CSV"** link (a **disjoint `reconcile-export`** marker class, kept distinct
from the spend export's `a.export` selector) downloads `GET /telemetry/reconcile.csv` (V17, above),
rendered only when there is a reconciliation to show. A pure-reader composition over the
two existing stores (no schema change, no new store, no ADR). — see
[ADR-0019](adr/0019-budget-burn-rate-forecast-trailing-window-average.md),
[ADR-0018](adr/0018-additive-attribution-tags-on-spend-records.md),
[ADR-0009](adr/0009-persisted-spend-history-append-only-ledger.md)

The **Settings page** (`GET /page/settings`, `POST /settings`) edits `config.json`'s
user-facing knobs through `editConfig` (snapshot → apply → validating `Save` →
rollback-on-invalid). Alongside the model/agent/budget/keybinding fields it carries the
**per-model price-override table** (G1): one row per model — seeded from
`telemetry.DefaultPriceBook().Models()` ∪ any model already overridden, sorted — with
three numeric (float) fields each (input · cached · output USD per million tokens),
pre-filled from `config.Telemetry.PriceOverrides` with the built-in default shown as the
placeholder. Field names are index-keyed (`price.<i>.{model,in,cached,out}`, the model id
in a hidden field) so a model id containing dots can't collide with the delimiter. On
save the rows parse into the override map: a row whose three fields are all blank/zero
contributes **no** override (so the model keeps its default — a blank row must never
persist a `$0`-rate override), a negative rate is rejected by `config.Validate` (rolled
back). The save then **reprices the live meter** — it rebuilds the price book from
`telemetry.BuildPriceBook(overrides)` (defaults + overrides) and `Replace`s the shared
book's contents in place, so the account meter **and** every per-session meter (which
share the one `*PriceBook` by reference) price the next turn at the new rate without a
restart — the same live-apply discipline `refreshBudget` uses for the budget knobs. The
section is preserved-on-absent (a partial POST that omits it leaves stored overrides
untouched, like the keyboard-shortcut section). Values flow through `html/template`
(ADR-0001); the rate fields are numbers and the model id labels are escaped. No ADR —
additive UI over an existing config field, with the live-apply seam an obvious mirror of
the startup price-book build (`internal/bootstrap`). — see
[ADR-0008](adr/0008-budget-guardrails-soft-warn-and-hard-cap-gate.md)

The **Connection page** (`GET /page/connection`, `POST /connection`, A1/issue 0068) is the
auth surface: the live credential (the seam's read-only `AuthStatus`, §1), the credential-
precedence ladder with the configured rung marked, and the method chooser —
`config.AuthMethod` ∈ {auto, token, gh} persisted through `editConfig` and **applied at the
next launch** (no runtime auth write surface, no live re-dial). A pasted token lands **only**
in the process environment (the `setEnv` seam, default `os.Setenv`); config persists the
`${VAR}` **name** (`GitHubTokenEnv`), never the value, and the value never renders — no
secret at rest (ADR-0020). Preflights make a method that will degrade visible: `gh` missing
on PATH (the `lookPath` seam) and the token var unset in this process (the `lookupEnv` seam).
Device flow cannot start in-app (no SDK surface); the page says to run `copilot` in a
terminal and relaunch. — see
[ADR-0039](adr/0039-connection-page-auth-method-via-config-status-via-auth-getstatus.md)

The **Sessions page** (`GET /page/sessions`) lists the runtime's persisted sessions
(title + relative age, with resume/delete controls) and — when a spend store is wired —
a **per-session cost cell** (G2): `sessionRows` joins `SessionShares(s.spend.Records())`
(§4) onto each `copilot.SessionMeta` row **keyed by session id**, surfacing *"N turns ·
X cr"* beside the title/age. It is a **pure-reader composition** over the existing ledger
(no schema change — `SessionID` was already tagged, ADR-0018): a listed session with no
spend keeps its row and shows *"no cost yet"* (never dropped); a spend bucket whose id
matches no listed session (a since-deleted session) is simply not shown; with **no spend
store** wired the rows render their prior shape (title + age, no cost cell). The join
takes neither `s.mu` nor `forgeMu` (the spend store's own mutex is a leaf), so it can't
invert the `forgeMu → s.mu` lock order. Credits flow through `telemetry.FormatCredits`
and all values through `html/template` (ADR-0001). The decision is pre-blessed by the
same cost ⋈ surface convergence rationale as V4/0025 and F3/0026 (no ADR). — see
[ADR-0018](adr/0018-additive-attribution-tags-on-spend-records.md),
[ADR-0002](adr/0002-restore-sdk-session-resume-for-session-pick-start-continue.md)

## 4. Persisted schemas (forge + config)

**Producer/consumer:** `internal/ctxforge` and `internal/config`; written to disk.
**Stability: stable** (JSON tags are the on-disk contract; changes must stay backward-readable
or ship a migration). Writes are atomic (temp-file + rename + validate).

- **`ctxforge.Agent`** (`types.go:45`): `id, name, description, model, reasoningEffort`
  (low|medium|high|xhigh), `systemMessage?`, `skills?` (skill IDs always activated),
  `allowedTools?` (empty = all). — see [ADR-0003](adr/0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md)
- **`ctxforge.Skill`** (`types.go:22`), **`ctxforge.Instruction`** (`types.go:35`).
- **`ctxforge.MCPServer`** (`types.go:77`): `id, name, command, args?, env?, enabled`.
  A stdio server: `command`+`args` are exec'd by the runtime (`MCPStdioServerConfig`).
  Managed via the MCP page (validated builders, rollback-on-invalid). `env` is **edited
  via masked key/value rows**: a literal value persists verbatim, while a **secret** row
  persists **only** a `${VAR}` reference (the on-disk shape `[A-Z_][A-Z0-9_]*`), never
  the secret itself — it is resolved from the process environment at session start
  (`web.MCPServerSpecs`, behind a lookup seam), following the `config.GitHubTokenEnv`
  precedent (**no secret at rest**). A pre-C1 forge (all literal values) loads
  identically. Curated defaults are seeded **disabled** and key-free; the page
  preflights `command` on `PATH` and flags an enabled server's `${VAR}` that resolves
  empty. — see
  [ADR-0010](adr/0010-mcp-server-management-page-curated-defaults-disabled-with-preflight.md),
  [ADR-0020](adr/0020-mcp-secrets-via-env-var-reference-indirection.md)
- **`ctxforge.Workflow`** / **`ctxforge.WorkflowStep`** (`workflow.go`): a multi-agent
  run. `Workflow` is `{id, name, description, mode, steps}` where `mode` ∈
  {`sequential`, `parallel`} (`""` reads as sequential); each `WorkflowStep` is
  `{agentId, prompt, when?}`. Persisted under the additive `workflows` key on
  `forge.json` (omitempty, so older files read clean). `Validate` enforces id slug,
  name, a valid mode, ≥1 step, and each step's agent+prompt; **whole-forge Validate**
  additionally enforces step→agent referential integrity (the `agentId` must resolve,
  the built-in `chat` agent included), like agent→skill. `CompileWorkflow` reuses
  `Compile` to produce a `SessionSpec` per step, deterministically (and carries each
  step's `when` into the compiled step). — see [ADR-0013](adr/0013-multi-agent-workflow-run-handoff-surface.md)
- **`ctxforge.WorkflowStep.When`** / **`ctxforge.StepCondition`** (`workflow.go`): the
  optional **branching** predicate gating a step on a prior step's settled outcome
  (B2). `StepCondition` is `{step, condition, value?}`: `step` is the **1-based index
  of a strictly-prior step** the predicate reads, `condition` ∈ {`succeeded`,
  `failed`, `output-contains`, `always`}, and `value` is the (case-insensitive)
  substring for `output-contains`. **Additive + backward-readable:** `when` is
  `omitempty` and a nil `When` means *always runs*, so a pre-B2 workflow loads and
  behaves identically and a v1 reader ignores the key. `Workflow.Validate` checks the
  predicate purely (known condition; a strictly-prior `step` for the step-reading
  conditions — which forbids self/forward references and so makes a dependency cycle
  structurally impossible; a non-empty `value` for `output-contains`). An unsatisfied
  step is **skipped** (a distinct, terminal lane state — `laneSkipped`, rendered with
  a `⊘` glyph and `lane-skipped` class), **not** failed; a skipped lane counts as
  settled so the run still terminates. Predicate evaluation is a pure function over
  prior lanes' settled status/output in the `workflowRun` engine. — see
  [ADR-0021](adr/0021-conditional-branching-workflow-steps-declarative-predicate.md)
- **`ctxforge.Snippet`** (`snippet.go`): `{id, name, body}` — a saved, reusable
  composer prompt (a one-shot user prompt, **not** system-message context like a
  Skill, so it carries no model/effort/tools/enabled and is never `Compile`d).
  Persisted under the additive `snippets` key on `forge.json` (omitempty, so older
  files read clean). `Validate` enforces a slug id, a name, and a body; managed via
  the Snippets page (validated builders, rollback-on-invalid). The `id` doubles as
  the composer `/trigger`. — see [ADR-0015](adr/0015-prompt-snippet-library-forge-backed-composer-insertion.md)
- **`ctxforge.Hook`** (`hook.go`): `{id, event (pre-tool-use|post-tool-use), match
  {toolKind?, pattern?, outsideWorkspace?}, action (allow|deny|ask), reason?, enabled,
  mandatory?, modes?, command?, commandArgs?}` — a first-class forge **governance rule**. Persisted under the additive
  `hooks` key on `forge.json` (omitempty, so older files read clean) and CRUD-managed via the
  shared `mutate` rollback discipline (`AddHook`/`UpdateHook`/`ToggleHook`/`RemoveHook`, like
  MCP/snippet) — the management **UI** is the Hooks page (G4, §3). `Validate` enforces a slug id, a
  known `event` and `action`, a non-empty `match` (a valid `toolKind` ∈ {read, write, shell, mcp}
  when set; `outsideWorkspace` also satisfies the non-empty requirement), **no dangling `${VAR}`**
  reference (the well-formed `${NAME}` shape of ADR-0020) in `pattern`/`reason`/`command`/`commandArgs`, and a known
  `mode` ∈ {autopilot, interactive, plan} in each `modes` entry. `mandatory` marks a hook whose
  decision is **unbypassable by config** (the dangerous-action ruleset); `outsideWorkspace` is the
  path-aware **workspace fence** dimension (G2); `modes` is the **mode-binding** scope (G4, empty =
  every mode). `command`/`commandArgs` (G5, ADR-0032) is the **PostToolUse executor command** — a
  single local program (+ args) run after a matching tool completes, `${VAR}`-bearing and resolved
  at execution by the seam; `Validate` permits a command **only on a post-tool-use hook** (a
  PreToolUse hook carrying one is rejected — untrusted output must never be a pre-gate control
  surface) and requires a non-empty `command` when `commandArgs` is set. `PostToolUseCommands(hooks,
  toolKind, command, workspace, mode)` is the pure selector of the matching command hooks (the
  post-path companion to `Evaluate`; it makes no allow/deny/ask decision). `HasCommand()` reports
  whether a hook carries one. The pure
  **`Evaluate(hooks, event, toolKind, command, workspace, mode) Decision{Action, Reason, HookID, Mandatory}`**
  resolves the policy: a hook participates when `Enabled`, its `Event` matches, its `Modes` is
  empty or lists `mode`, and its `Match` applies (an empty `toolKind` matches any kind; a `pattern`
  with `*`/`?` is a glob over the **whole** command, else a substring; `outsideWorkspace` requires
  the command — a write's target path — to resolve outside `workspace` via `filepath.Rel`, empty
  `workspace` = inert). Precedence is **deny > ask > allow** and the no-match default is **ask**
  (fall through to the gate) — order-independent, deterministic. `Decision.Mandatory` reports
  whether a mandatory hook drove the winning action; `Decision.HookID` names the winning hook (for
  the timeline "why"). The bridge enforces a mandatory deny/ask **even under `AutoApproveTools`**
  (§1), while a user `deny` (more restrictive) still wins over a mandatory `ask`. The pure
  **`EffectiveAutoApprove(mode, configDefault)`** resolves the mode-bound auto-approve baseline
  (autopilot on / interactive off / else config; mode binding, G4), and the exported
  **`MatchPattern`/`PatternIsGlob`** back the UI preflight. `DefaultHooks()` is the built-in
  safe-read set and `DangerousHooks()` is the built-in mandatory dangerous ruleset; `Forge.Compile`
  prepends both to the enabled user hooks into `SessionSpec.Hooks` (§1).
  **Determinism:** `Compile`'s hook order is part of its stable output. — see
  [ADR-0029](adr/0029-hooks-forge-entity-bridge-enforced-allow-deny-ask-safe-read-defaults.md),
  [ADR-0030](adr/0030-dangerous-action-deny-and-mandatory-hitl-unbypassable-by-config.md),
  [ADR-0031](adr/0031-hooks-management-ui-mode-binding-and-timeline-why.md),
  [ADR-0032](adr/0032-posttooluse-hook-command-execution-untrusted-output.md)
- **`config.Config`** / **`config.TelemetryConfig`** (`config.go`): user settings + pricing
  overrides (`DefaultPriceBook`). `TelemetryConfig.WarnFraction` (soft-warn threshold,
  `[0,1]`) and `TelemetryConfig.HardCapCredits` (absolute credit ceiling, `>= 0`,
  `0` = off) back the budget guardrails. — see [ADR-0008](adr/0008-budget-guardrails-soft-warn-and-hard-cap-gate.md)
  `Config.AuthMethod` (`authMethod`, omitempty — older files read clean) selects how the app
  authenticates at dial time: `""` (auto: the CLI's own chain), `"token"` (the explicit
  `${GitHubTokenEnv}` token), `"gh"` (a token resolved from `gh auth token`, in-memory only).
  Membership-only validation; a method that can't produce a token degrades to auto at dial
  (`copilot.ResolveAuthMethod`) and the Connection page preflight surfaces it. `GitHubTokenEnv`
  stays a NAME, never a token (no secret at rest). — see
  [ADR-0039](adr/0039-connection-page-auth-method-via-config-status-via-auth-getstatus.md)
  `TelemetryConfig.PriceOverrides` (`priceOverrides`, omitempty) maps a model id to a
  `[]float64` of USD-per-million-token rates: `[input, cached, output]` and an **optional
  4th** `cacheWrite` element (ADR-0034). It is applied over `DefaultPriceBook` at startup
  (`internal/bootstrap`) and live on a Settings save (G1). `Validate` enforces a length of
  **3 or 4** and each rate **`>= 0`** (a negative or wrong-length row is rejected; absent
  overrides stay valid). **Backward-readable migration:** the value was a fixed
  `[3]float64` pre-0059; a variable-length slice reads both the legacy 3- and the new
  4-element JSON natively — a 3-element row derives cache-write at the 1.25×-input default.
  The **live-reprice seam** is pure and dependency-free in `internal/telemetry`:
  `telemetry.BuildPriceBook(overrides map[string][]float64) *PriceBook` builds a fresh book
  from defaults + overrides (rebuild-not-incremental — a removed override reverts to its
  default; a 3-element row derives cache-write off the overridden input), and
  `(*PriceBook).Replace(src *PriceBook)` atomically swaps a shared book's contents in
  place under its own RWMutex, so every meter holding that book by reference reprices at
  once. — see [ADR-0008](adr/0008-budget-guardrails-soft-warn-and-hard-cap-gate.md)
  `Config.KeyBindings` (`keyBindings`, omitempty) holds per-action keyboard-shortcut
  **overrides** keyed by action id; the rebindable action set is fixed in code
  (`config.KeyActions()` — ordered `{id, label, default}`), and `Config.Keymap()`
  resolves the effective key per action (override-or-default). `Validate` enforces a
  known action id, a single-character key, and no duplicate key across actions.
  Surfaced in the help overlay + Settings form; edited through `editConfig`
  (rollback-on-invalid). Older files (no `keyBindings`) read clean. The action→key
  map reaches the frontend as `<body data-keymap>` (JSON via `keymapJSON`, shared by
  the index render and the live-apply swap below) for the vanilla-JS dispatcher.
  **Live-apply (V10):** on a successful keybinding `POST /settings`, the response
  appends — beside the persisted settings partial — an `hx-swap-oob` re-render of
  the `#help-overlay` **and** a `<script>applyKeymap(…)</script>` that updates
  `<body data-keymap>` and rebuilds the dispatcher's reverse map from one source, so
  a rebind takes effect **without a full reload** (closes TECH_DEBT #13). The emitted
  keymap is read back from the **persisted** config, so a no-op/rolled-back save
  re-emits the in-sync keymap (the error branch emits none) and the live attribute
  can never desync from disk; binding text flows through `html/template`/`esc` and
  the script JSON through `encoding/json` (ADR-0001). This **completes** the ADR-0014
  mechanism (no new ADR). — see
  [ADR-0014](adr/0014-keybinding-surface-config-backed-keymap-with-minimal-js-dispatch.md),
  [REGRESSIONS #18](REGRESSIONS.md)
- **`telemetry.SpendStore`** / **`telemetry.SpendRecord`** (`history.go`): the persisted
  spend ledger at `<configDir>/spend.json`. **Shared machinery (H1):** `SpendStore` and
  `RunStore` (below) are now thin typed embeddings of one generic
  `telemetry.AppendOnlyStore[T any]` (`store.go`) that single-sources the persistence
  discipline (atomic temp-file+rename, missing=empty, present-but-invalid=error,
  newer-`version`-tolerant, `dir==""`-ephemeral, mutex-guarded `Append`/`Records`/`Count`);
  the **on-disk JSON tags (`version` + `records`/`runs`) are the unchanged stable contract**
  — the generic store writes byte-identically to the pre-H1 stores. A refactor-only paydown
  that preserves this and the run envelope (issue 0033, TECH_DEBT #14; no new ADR — see
  ADR-0009 / ADR-0022). On-disk shape is a versioned envelope
  `{"version":3,"records":[…]}`; each record is
  `{at, session?, model, in, cached, out, cw?, reasoning?, usd, aiu?, agent?, workflow?, lane?}`
  (JSON tags are the contract). Written **atomically** (temp-file + rename); missing file =
  empty, present-but-invalid = error. **Migration note:** `version` gates the schema and
  the `records` array is the stable surface — bumps must add fields only (older readers
  ignore unknown keys; newer readers tolerate a higher `version`) or ship a converting
  migration. **v2** added the additive `agent`/`workflow`/`lane` attribution tags
  (`omitempty`); **v3** added the additive `cw` (cache-write, priced) + `reasoning`
  (display-only subset of output) token counts (`omitempty`, ADR-0034); **v4** added the
  additive `sub`/`subname` sub-agent instance id/name tags (`omitempty`, ADR-0042). Each is
  additive: an older file loads unchanged (the new fields read back `0`/empty) and an older
  reader ignores the new keys and tolerates the higher `version`. `agent` is the active
  persona id (empty = built-in chat); `workflow`+`lane` are set only when a workflow run
  owned the turn; `sub`+`subname` are set only when a sub-agent instance owned the turn. The
  pure `AgentShares` / `WorkflowShares` / `SubagentShares` aggregations (cousins of
  `ModelShares`) roll spend up by tag — and `AgentShares` **excludes** sub-agent-tagged turns
  so a sub-agent's spend is not double-counted into the persona buckets; `ModelBreakdowns`
  rolls per-model token counts (incl. cache-write/reasoning) up for the all-time table. The
  export CSV appends `cacheWrite`,`reasoning`,`subagent`,`subagentName` columns at the end
  (legacy column positions unchanged). — see
  [ADR-0009](adr/0009-persisted-spend-history-append-only-ledger.md),
  [ADR-0018](adr/0018-additive-attribution-tags-on-spend-records.md),
  [ADR-0034](adr/0034-price-cache-write-additive-reasoning-is-output-subset.md),
  [ADR-0042](adr/0042-per-subagent-cost-attribution-and-the-budget-leash.md)
  **Per-session roll-up (G2):** `telemetry.SessionShares(records) []SessionShare` is
  another pure cousin of the `*Shares` readers — it rolls spend up **per copilot session
  id** to `SessionShare{SessionID, Credits, Turns}` (turn count + total credits), sorted
  by credits descending then session id ascending (a total, deterministic order). It
  **excludes** the empty-`SessionID` bucket (`includeEmpty=false`, like `WorkflowShares`):
  a session row needs a real id to join against, and the picker only lists real sessions,
  so a pre-attribution v1 row — or a turn recorded before a copilot session bound — has
  nothing to attach to. The turn count rides `shareBy`'s per-group `Count` (one pass, no
  schema change). Powers the cost-aware Sessions picker (§3). — see
  [ADR-0009](adr/0009-persisted-spend-history-append-only-ledger.md),
  [ADR-0018](adr/0018-additive-attribution-tags-on-spend-records.md)
  **Read-source (account-wide budget accounting):** the persisted ledger — not the
  in-process `Meter` — is the source of truth for the account-wide "Total cost /
  Monthly budget / Remaining" rows, the topbar cost footer, `/cost`, and the
  hard-cap projection baseline. They read `telemetry.MonthToDate(records, now)` (a
  new **pure reader** over the existing v1 `records`, UTC calendar month — no schema
  change), so spend survives a restart. The per-session statusline (`sessionMeter`,
  ADR-0011) and the live token split stay on the in-process meter — one source per
  surface. — see [ADR-0016](adr/0016-ledger-is-source-of-truth-for-account-wide-budget-accounting.md)
  **Forecast (predictive):** `telemetry.Forecast(daily []DayTotal, budget Budget,
  now time.Time) Projection` is another **pure reader** (no schema change) over the
  same ledger — a trailing-7-day-average burn rate projecting days/turns to the
  monthly allowance and an exhaustion date, with degenerate cases explicit in
  `Projection.Status` (no-budget / idle / exhausted / ok). Surfaced on the
  Telemetry page and (compact) in the statusline. — see
  [ADR-0019](adr/0019-budget-burn-rate-forecast-trailing-window-average.md)
  **Bucketed forecast (predictive ⋈ attributable — F3):** `telemetry.DailyTotalsBy(records
  []SpendRecord, keyOf func(SpendRecord) string, includeEmpty bool) map[string][]DayTotal`
  buckets the daily series by the A2 agent/workflow tag (mirroring `shareBy`'s `keyOf`;
  `includeEmpty=false` skips the empty-key bucket, like `WorkflowShares`), and
  `telemetry.BucketForecasts(records, budget, now, keyOf, includeEmpty) []BucketProjection`
  runs the **same `Forecast` slope unchanged** over each bucket's own daily series, returning
  `BucketProjection{Key, Credits, Projection}` sorted by spend descending then key ascending (a
  total, deterministic order, like the `*Shares` readers). Reusing `Forecast` per bucket keeps
  the ADR-0019 slope single-sourced — its denominator is **elapsed observed days clamped to the
  bucket's own ledger age**, so a single-day bucket clamps to one day, not a mostly-absent week.
  **Per-bucket framing:** a bucket has no own allowance, so the account-wide `Projection`
  fields `DaysToCap`/`ExhaustionDate` are **not** surfaced per bucket — the Telemetry view
  reads only the bucket's `DailyRate`/`UsedCredits` (rate + a month projection, no per-bucket
  cap). Another **pure reader**, no schema change. — see
  [ADR-0019](adr/0019-budget-burn-rate-forecast-trailing-window-average.md),
  [ADR-0018](adr/0018-additive-attribution-tags-on-spend-records.md)
- **`telemetry.RunStore`** / **`telemetry.RunRecord`** / **`telemetry.RunLane`**
  (`runs.go`): the persisted **workflow-run history** at `<configDir>/runs.json` — a
  **sibling** of the spend ledger, not a merged file (each keeps its own grain: spend
  is one row per metered turn, runs is one row per orchestrated run). Shares the generic
  `telemetry.AppendOnlyStore[T]` machinery with `SpendStore` (H1, above) — same atomic-write
  / missing=empty / newer-version-tolerant / ephemeral discipline, same unchanged on-disk
  tags. On-disk shape is the versioned envelope `{"version":1,"runs":[…]}`; each `RunRecord` is
  `{id, workflow, name, mode, startedAt, finishedAt, outcome, lanes:[{index, agentId?,
  status, credits?}]}` (JSON tags are the contract). A lane's `status` ∈ {`done`,
  `failed`, `skipped`} — so a **branched** run's per-lane outcomes, including a
  **skipped** lane that incurred no cost, are first-class (the reason a spend record
  can't stand in: a branch that didn't run leaves no metered turn). `outcome` ∈
  {`finished`, `failed`}. Written **atomically** (temp-file + rename); missing file =
  empty, present-but-invalid = error, empty dir = ephemeral (demo/tests). **Migration
  note:** `version` gates the schema and the `runs` array is the stable surface —
  bumps must add fields only (older readers ignore unknown keys; newer readers
  tolerate a higher `version`) or ship a converting migration. The web layer records a
  run **once on completion** (`recordRun` → `Append`, best-effort: a disk error is
  logged, not surfaced); the Runs page (§3) queries it. — see
  [ADR-0022](adr/0022-workflow-run-history-sibling-append-only-run-store.md),
  [ADR-0009](adr/0009-persisted-spend-history-append-only-ledger.md)
  **Aggregations (pure readers — V1):** `telemetry.RunAggregates(records
  []RunRecord) []RunAggregate` rolls the run history up **per workflow** — a cousin
  of the `*Shares` spend readers — to `RunAggregate{WorkflowID, Name, Runs, Failures,
  TotalCredits, AvgCredits, TotalDuration, AvgDuration, LastOutcome, LastStartedAt}`
  (with `FailureRate()` in `[0,1]`), sorted by run count descending then workflow id
  ascending (a total, deterministic order). A run whose `Outcome` is `"failed"` counts
  as a failure; a skipped lane adds zero cost (`RunRecord.Credits` excludes it).
  `LastOutcome`/`LastStartedAt` name the workflow's **most recent run** (the "last run"
  signal the Workflows page badges, V4): the latest by `StartedAt`, a tie broken by the
  later `FinishedAt` then stable input order — a deterministic pick regardless of record
  order (the zero value is never produced for a present aggregate, so `Runs > 0` is the
  guard). `RunRecord.Duration() time.Duration` is the run's wall-clock span
  (`FinishedAt − StartedAt`), guarding a zero/unset/negative span → 0 so callers
  sum/average without per-entry guards. All are **pure** over the existing v1 records —
  **no schema change** — joining the run grain (count / failure rate / duration /
  last-run) to the cost the runs metered, the orchestration half that `WorkflowShares`
  can't answer. The Runs page (§3) renders a per-workflow summary from `RunAggregates`
  above the history and a duration cell per run — surfacing both `TotalCredits` (the
  workflow's **cumulative** orchestrated spend, V13) and `AvgCredits` beside each other,
  the orchestration analogue of the Telemetry per-workflow share's total; the
  **Workflows page** (§3) badges each
  row with the last-run signal + run count joined to `WorkflowShares` spend. — see
  [ADR-0022](adr/0022-workflow-run-history-sibling-append-only-run-store.md)
  **CSV export (pure reader — V11):** `telemetry.WriteRunsCSV(w io.Writer, records
  []RunRecord) error` is the orchestration sibling of `WriteCSV` — it flattens the run
  history to **one row per lane** (run-level columns repeated) with the fixed header
  `run,workflow,name,mode,startedAt,finishedAt,durationSeconds,outcome,lane,agent,status,credits`,
  so a branched run's **skipped** lane (which leaves no spend record) is first-class.
  Streamed by `GET /runs/export.csv` (§3); pure (the writer is the only IO), no schema
  change, no ADR. — see
  [ADR-0022](adr/0022-workflow-run-history-sibling-append-only-run-store.md)
  **Per-lane roll-up (pure reader — V14):** `telemetry.LaneShares(records []RunRecord)
  []LaneShare` is the **per-lane cousin** of `RunAggregates` — it rolls the run history up
  **per (workflow, lane)** to `LaneShare{WorkflowID, LaneIndex, AgentID, Runs, Failures,
  Credits, Fraction}`, sorted by **credits descending** (ties → workflow id ascending then
  lane index ascending, a total deterministic order). A skipped lane adds **zero cost**
  (`RunLane.Credits` is zero) but still counts toward `Runs`; a lane whose `Status` is
  `"failed"` counts as a failure; `Fraction` is each lane's share of all lane credits (0
  when nothing metered); `AgentID` is the **raw id** (latest seen, chronological) — the
  web layer resolves it to a label. Empty history → empty slice. The finest
  orchestration-attribution grain (*which lane in a workflow costs / fails most?*), below
  `RunAggregates`' per-workflow roll-up. **Pure** over the existing v1 records — no schema
  change, no cross-package seam, no ADR; the Runs page (§3) renders it as a "Cost by lane"
  share list. — see
  [ADR-0022](adr/0022-workflow-run-history-sibling-append-only-run-store.md)
- **`telemetry.WorkflowReconcile`** (`reconcile.go`): the pure **cross-store** reconciliation
  reader (V15) — the convergence of the two persisted stores. `WorkflowReconcile(spend
  []SpendRecord, runs []RunRecord) []WorkflowRecon{WorkflowID, LedgerCredits, RunCredits,
  Delta}` **joins** the spend ledger's per-workflow roll-up (`WorkflowShares` grain —
  workflow-attributed spend, the empty-workflow chat bucket excluded) against the run
  history's per-workflow roll-up (`RunAggregates.TotalCredits` grain — each workflow's
  recorded runs' metered credits, a skipped lane adding zero), keyed by workflow id, with
  `Delta = LedgerCredits − RunCredits` (0 = the two stores agree). A workflow present in
  **one** store but not the other yields a row with the other side zero (Delta its full
  magnitude). Sorted by **absolute delta descending** (the biggest discrepancy first; ties
  → ledger credits descending, then workflow id ascending — a total deterministic order
  over the unique workflow key). Empty inputs → empty slice. The cross-store cousin of the
  two per-workflow readers it joins — it takes **both** record slices and returns **ids**
  (the web layer resolves labels under `forgeMu`), so there is **no cross-package seam**;
  **pure**, no schema change, **no ADR**. The Telemetry page (§3) renders it as a "Ledger
  vs runs" comparison table, ambering a non-trivial delta. — see
  [ADR-0022](adr/0022-workflow-run-history-sibling-append-only-run-store.md),
  [ADR-0018](adr/0018-additive-attribution-tags-on-spend-records.md),
  [ADR-0009](adr/0009-persisted-spend-history-append-only-ledger.md)
  **Per-lane reconciliation (pure cross-store reader — V16):** `telemetry.LaneReconcile(spend
  []SpendRecord, runs []RunRecord) []LaneRecon{WorkflowID, LaneIndex, LedgerCredits,
  RunCredits, Delta}` is the **per-lane cousin** of `WorkflowReconcile` — the same cross-store
  join one grain **finer**, keyed by `(workflow, lane)`. It joins the ledger's lane-tagged
  spend (`SpendRecord` grouped by `WorkflowID + LaneIndex` — the **`LaneShares` ledger
  cousin**, ADR-0018's lane attribution, the empty-workflow chat bucket excluded) against the
  run history's per-lane credits (`RunLane.Credits` summed by `(workflow, lane Index)` — the
  **`LaneShares` grain**, a skipped lane adding zero), with `Delta = LedgerCredits −
  RunCredits`. A `(workflow, lane)` present in **one** store but not the other yields a row
  with the other side zero; a lane **zero on both sides** (a skipped run lane with no ledger
  spend — nothing to reconcile) is **omitted** (mirroring how `WorkflowReconcile` never emits a
  both-zero workflow). Sorted by **absolute delta descending** (ties → ledger credits
  descending, then workflow id ascending, then lane index ascending — a total deterministic
  order over the unique `(workflow, lane)` key). Empty inputs → empty slice. Takes **both**
  record slices and returns **ids** (the web layer resolves labels under `forgeMu`), so there
  is **no cross-package seam**; **pure**, no schema change, **no ADR**. The Telemetry page (§3)
  renders it as a "Ledger vs runs by lane" comparison table below the per-workflow one,
  ambering a non-trivial delta — so a divergence the per-workflow row only totals is locatable
  at the exact step. — see
  [ADR-0022](adr/0022-workflow-run-history-sibling-append-only-run-store.md),
  [ADR-0018](adr/0018-additive-attribution-tags-on-spend-records.md),
  [ADR-0009](adr/0009-persisted-spend-history-append-only-ledger.md)
  **CSV export (pure writer — V17):** `telemetry.WriteReconcileCSV(w io.Writer, spend
  []SpendRecord, runs []RunRecord) error` is the **export sibling** of `WriteCSV` (spend) and
  `WriteRunsCSV` (runs) — it serializes the cross-store reconciliation so the divergence can
  **leave the tool**. **One file carries both grains** with the fixed header
  `grain,workflow,lane,ledgerCredits,runCredits,delta`: the per-workflow rows (`WorkflowReconcile`)
  first, then the per-`(workflow, lane)` rows (`LaneReconcile`), each labelled by a leading
  **`grain` column** (`"workflow"` | `"lane"`) so a consumer filters totals from breakdown and
  never double-counts — credits via `csvFloat` (the sibling writers' precision-rounded format).
  Rows are the readers' own deterministic output (biggest |delta| first within each grain);
  empty/chat-only input → header only. Streamed by `GET /telemetry/reconcile.csv` (§3); **pure**
  (the `io.Writer` the caller owns is the only IO), no schema change, **no ADR** (pre-blessed by
  the ADR-0009 export precedent). — see
  [ADR-0009](adr/0009-persisted-spend-history-append-only-ledger.md)

## 5. Invariants (promises that aren't a signature)

- **Determinism:** `Forge.Compile` and pricing produce identical output for identical input
  (guarded by tests on ordering/rates). — *ADR needed (backfill)*
- **Totality:** pricing returns a value for every model (unknown → default rate, never panics);
  event normalization is total (`EvUnknown` fallback).
- **Escaping:** all model-originated text is HTML-escaped before reaching the browser; only
  server-rendered markdown for committed turns emits HTML, via the sanitized renderer. — see [ADR-0001](adr/0001-render-markdown-server-side-for-committed-agent-turns.md)
- **Seam purity:** no SDK import outside `SDKClient`; `telemetry`/`ctxforge`/`config` stay
  dependency-free.
- **Persistence atomicity:** config/forge writes are temp-file + rename, validated before save.

---
*Consolidated from the scattered tables in ARCHITECTURE.md and WEB_UI_PLAN.md. Drift check:
re-run `scripts/extract-interfaces.sh` and reconcile against this file.*
