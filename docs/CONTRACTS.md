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
| `Abort(ctx, sessionID) error` | Cancel the in-flight turn. |
| `Respond(id, approve) error` | Answer a pending `EvPermission`. |
| `RespondInput(id, answer) error` | Answer a pending `EvUserInput` (ask_user). |
| `RespondPlan(id, approved, action, feedback) error` | Answer a pending `EvPlanReview`. |
| `RespondElicit(id, action, content) error` | Answer a pending `EvElicitation`; `action` ∈ {accept, decline, cancel}. |
| `ListModels(ctx) ([]ModelInfo, error)` | Models available to the account. |
| `ListSessions(ctx) ([]SessionMeta, error)` | Persisted sessions, most-recent first. |
| `ResumeSession(ctx, sessionID, SessionSpec) (id, error)` | Reattach to a persisted session, wiring the same handlers as `CreateSession`; runtime restores full history. — see [ADR-0002](adr/0002-restore-sdk-session-resume-for-session-pick-start-continue.md) |
| `SessionHistory(ctx, sessionID) ([]Event, error)` | A session's conversation as normalized events, for rebuilding the transcript. |
| `DeleteSession(ctx, sessionID) error` | Permanently remove a persisted session. |
| `Events() <-chan Event` | Single normalized-event stream until `Close`. |
| `Close() error` | Release all resources. |

**`SessionSpec`** (`copilot.go:198`): `Model, ReasoningEffort, SystemMessage, Streaming,
AutoApproveTools, MCPServers, AllowedTools`. `AllowedTools` empty = all tools; otherwise maps
to the SDK session's `AvailableTools`. — see [ADR-0003](adr/0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md)
`MCPServers` carries the forge's **enabled** servers (compiled in, translated via
`web.MCPServerSpecs`); each `copilot.MCPServer` registers under its unique `Key()`
(its `ID`, or `Name` for legacy callers) so a non-unique `Name` can't collide in the
SDK config map. — see [ADR-0010](adr/0010-mcp-server-management-page-curated-defaults-disabled-with-preflight.md)

## 2. Normalized event vocabulary — `EventType` (`Ev*`)

**Producer:** `SDKClient` normalizes SDK events into these; `MockClient` emits them directly.
**Consumer:** the Hub's `pump` routes each by `SessionID`; the Server's reducer renders SSE
fragments. **Stability: stable** — the wire vocabulary between runtime and UI.

`EvMessage`, `EvMessageDelta`, `EvReasoning`, `EvReasoningDelta`, `EvToolStart`,
`EvToolProgress`, `EvToolEnd`, `EvUsage`, `EvContextWindow`, `EvPermission`, `EvUserInput`,
`EvUserMessage`, `EvPlanChanged`, `EvPlanReview`, `EvElicitation`, `EvCompactionStart`,
`EvCompactionEnd`, `EvSubagentStart`, `EvSubagentEnd`, `EvError`, `EvIdle`, `EvUnknown`.

**`Event` shape** (`copilot.go`): `Type, SessionID, Text, Tool, ToolCall*, Usage, Context,
Permission*, Input*, Plan*, Elicit*, Subagent*, Err`. Pointer fields are set only for the
matching event type (e.g. `Permission` for `EvPermission`). `SessionID` is empty for
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
| Telemetry | `GET /telemetry/export.csv` |
| Skills | `GET /skills/new` · `GET /skills/{id}/edit` · `POST /skills` · `POST /skills/{id}` · `POST /skills/{id}/toggle` · `POST /skills/{id}/delete` |
| Instructions | `POST /instructions/import` · `GET /instructions/new` · `GET /instructions/{id}/edit` · `POST /instructions` · `POST /instructions/{id}` · `POST /instructions/{id}/toggle` · `POST /instructions/{id}/delete` |
| Agents | `GET /agents/new` · `GET /agents/{id}/edit` · `POST /agents` · `POST /agents/{id}` · `POST /agents/{id}/select` · `POST /agents/{id}/delete` |
| MCP servers | `GET /mcp/new` · `GET /mcp/{id}/edit` · `POST /mcp` · `POST /mcp/{id}` · `POST /mcp/{id}/toggle` · `POST /mcp/{id}/delete` |
| Workflows | `GET /workflows/new` · `GET /workflows/{id}/edit` · `POST /workflows` · `POST /workflows/{id}` · `POST /workflows/{id}/run` · `POST /workflows/{id}/delete` |
| Snippets | `GET /snippets/new` · `GET /snippets/{id}/edit` · `POST /snippets` · `POST /snippets/{id}` · `POST /snippets/{id}/delete` |
| Sessions | `POST /sessions/new` · `POST /sessions/{id}/resume` · `POST /sessions/{id}/delete` |
| Settings | `POST /settings` · `POST /models/{id}/select` · `POST /effort/{value}/select` |

`/instructions/import`, `/agents/{id}/select`, and the `/sessions/*` routes are the
phase-2–4 additions. — see [ADR-0002](adr/0002-restore-sdk-session-resume-for-session-pick-start-continue.md), [ADR-0003](adr/0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md)

The `/mcp…` group (item 2.2) closes MCP-server CRUD, the last forge entity without
a UI; it mirrors the skills/agents routes. — see [ADR-0010](adr/0010-mcp-server-management-page-curated-defaults-disabled-with-preflight.md)

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

The **Runs** view (`GET /page/runs`, item B3) is a read-only history of completed
workflow runs (most recent first) — each run's name, mode, outcome, when it ran,
total metered cost, and a per-lane breakdown (agent, settled status incl.
**skipped**, credits). It is a query over the persisted `telemetry.RunStore` (§4);
adding it as a top-level nav page bumps the `pageNames` / e2e `pages` count. A run is
recorded **once on completion** by the web adapter (`workflow.go` `recordRun`, where
`runFrags` already clears `busy`). — see [ADR-0022](adr/0022-workflow-run-history-sibling-append-only-run-store.md)

`POST /budget/{action}` (`action` ∈ {proceed, raise, cancel}) resolves a turn the
hard cap paused before `Send`: **proceed** dispatches the held prompt and keeps the
cap, **raise** lifts (disables) and persists the cap then dispatches, **cancel**
drops the turn. It is an **app-level** gate, not an SDK permission — distinct from
`/perm/{id}` despite reusing the inline-form look. — see [ADR-0008](adr/0008-budget-guardrails-soft-warn-and-hard-cap-gate.md)

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
  Managed via the MCP page (validated builders, rollback-on-invalid). `env` is **not**
  edited in the UI (no secrets surface yet) but is preserved across edits. Curated
  defaults are seeded **disabled** and key-free; the page preflights `command` on
  `PATH`. — see [ADR-0010](adr/0010-mcp-server-management-page-curated-defaults-disabled-with-preflight.md)
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
- **`config.Config`** / **`config.TelemetryConfig`** (`config.go`): user settings + pricing
  overrides (`DefaultPriceBook`). `TelemetryConfig.WarnFraction` (soft-warn threshold,
  `[0,1]`) and `TelemetryConfig.HardCapCredits` (absolute credit ceiling, `>= 0`,
  `0` = off) back the budget guardrails. — see [ADR-0008](adr/0008-budget-guardrails-soft-warn-and-hard-cap-gate.md)
  `Config.KeyBindings` (`keyBindings`, omitempty) holds per-action keyboard-shortcut
  **overrides** keyed by action id; the rebindable action set is fixed in code
  (`config.KeyActions()` — ordered `{id, label, default}`), and `Config.Keymap()`
  resolves the effective key per action (override-or-default). `Validate` enforces a
  known action id, a single-character key, and no duplicate key across actions.
  Surfaced in the help overlay + Settings form; edited through `editConfig`
  (rollback-on-invalid). Older files (no `keyBindings`) read clean.
  — see [ADR-0014](adr/0014-keybinding-surface-config-backed-keymap-with-minimal-js-dispatch.md)
- **`telemetry.SpendStore`** / **`telemetry.SpendRecord`** (`history.go`): the persisted
  spend ledger at `<configDir>/spend.json`. On-disk shape is a versioned envelope
  `{"version":2,"records":[…]}`; each record is
  `{at, session?, model, in, cached, out, usd, aiu?, agent?, workflow?, lane?}` (JSON
  tags are the contract). Written **atomically** (temp-file + rename); missing file =
  empty, present-but-invalid = error. **Migration note:** `version` gates the schema and
  the `records` array is the stable surface — bumps must add fields only (older readers
  ignore unknown keys; newer readers tolerate a higher `version`) or ship a converting
  migration. **v2** added the additive `agent`/`workflow`/`lane` attribution tags
  (`omitempty`): a v1 file loads unchanged (empty tags) and a v1 reader ignores the new
  keys and tolerates `version:2`. `agent` is the active persona id (empty = built-in
  chat); `workflow`+`lane` are set only when a workflow run owned the turn. The pure
  `AgentShares` / `WorkflowShares` aggregations (cousins of `ModelShares`) roll spend up
  by tag. — see [ADR-0009](adr/0009-persisted-spend-history-append-only-ledger.md),
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
- **`telemetry.RunStore`** / **`telemetry.RunRecord`** / **`telemetry.RunLane`**
  (`runs.go`): the persisted **workflow-run history** at `<configDir>/runs.json` — a
  **sibling** of the spend ledger, not a merged file (each keeps its own grain: spend
  is one row per metered turn, runs is one row per orchestrated run). On-disk shape is
  the versioned envelope `{"version":1,"runs":[…]}`; each `RunRecord` is
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
