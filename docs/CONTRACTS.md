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

**`SubagentInfo`** (`copilot.go:79`): `ToolCallID, Name, DisplayName, Description, Model,
Success, Detail` — carried by `EvSubagentStart`/`EvSubagentEnd`. The web layer's
sub-agent activity strip (`web.renderSubagents` → `subagentChip`) shows one chip per
running sub-agent (`DisplayName` + `Model`) and surfaces `Description` as the chip's
`title=` tooltip so concurrent sub-agents in a parallel run say *what* they are doing;
an empty `Description` renders the prior chip (no `title`). The description is
model/SDK-originated text and flows through `html/template` auto-escaping like every
other chip value (ADR-0001), never `trusted()` raw.

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
the history. It is a
query over the persisted `telemetry.RunStore` (§4) and its pure `RunAggregates`
roll-up; adding it as a top-level nav page bumps the `pageNames` / e2e `pages` count.
A run is recorded **once on completion** by the web adapter (`workflow.go`
`recordRun`, where `runFrags` already clears `busy`). — see
[ADR-0022](adr/0022-workflow-run-history-sibling-append-only-run-store.md)

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
slice over the existing pure reader (no schema change, no new store). — see
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
- **`config.Config`** / **`config.TelemetryConfig`** (`config.go`): user settings + pricing
  overrides (`DefaultPriceBook`). `TelemetryConfig.WarnFraction` (soft-warn threshold,
  `[0,1]`) and `TelemetryConfig.HardCapCredits` (absolute credit ceiling, `>= 0`,
  `0` = off) back the budget guardrails. — see [ADR-0008](adr/0008-budget-guardrails-soft-warn-and-hard-cap-gate.md)
  `TelemetryConfig.PriceOverrides` (`priceOverrides`, omitempty) maps a model id to its
  `[input, cached, output]` USD-per-million-token rate triple; it is applied over
  `DefaultPriceBook` at startup (`internal/bootstrap`) and live on a Settings save (G1).
  `Validate` enforces each rate **`>= 0`** (a negative rate is rejected; absent overrides
  stay valid, so older configs load unchanged). The **live-reprice seam** is pure and
  dependency-free in `internal/telemetry`: `telemetry.BuildPriceBook(overrides
  map[string][3]float64) *PriceBook` builds a fresh book from defaults + overrides
  (rebuild-not-incremental — a removed override reverts to its default), and
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
