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
`MockClient` events; a single-session consumer may ignore it.

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
| Sessions | `POST /sessions/new` · `POST /sessions/{id}/resume` · `POST /sessions/{id}/delete` |
| Settings | `POST /settings` · `POST /models/{id}/select` · `POST /effort/{value}/select` |

`/instructions/import`, `/agents/{id}/select`, and the `/sessions/*` routes are the
phase-2–4 additions. — see [ADR-0002](adr/0002-restore-sdk-session-resume-for-session-pick-start-continue.md), [ADR-0003](adr/0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md)

`POST /budget/{action}` (`action` ∈ {proceed, raise, cancel}) resolves a turn the
hard cap paused before `Send`: **proceed** dispatches the held prompt and keeps the
cap, **raise** lifts (disables) and persists the cap then dispatches, **cancel**
drops the turn. It is an **app-level** gate, not an SDK permission — distinct from
`/perm/{id}` despite reusing the inline-form look. — see [ADR-0008](adr/0008-budget-guardrails-soft-warn-and-hard-cap-gate.md)

`GET /telemetry/export.csv` streams the full persisted spend ledger as a CSV
attachment (header `at,session,model,input,cached,output,usd,credits,aiu`; one row
per metered turn). Empty (header only) when no ledger is wired. — see [ADR-0009](adr/0009-persisted-spend-history-append-only-ledger.md)

## 4. Persisted schemas (forge + config)

**Producer/consumer:** `internal/ctxforge` and `internal/config`; written to disk.
**Stability: stable** (JSON tags are the on-disk contract; changes must stay backward-readable
or ship a migration). Writes are atomic (temp-file + rename + validate).

- **`ctxforge.Agent`** (`types.go:45`): `id, name, description, model, reasoningEffort`
  (low|medium|high|xhigh), `systemMessage?`, `skills?` (skill IDs always activated),
  `allowedTools?` (empty = all). — see [ADR-0003](adr/0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md)
- **`ctxforge.Skill`** (`types.go:22`), **`ctxforge.Instruction`** (`types.go:35`),
  **`ctxforge.MCPServer`** (`types.go:77`).
- **`config.Config`** / **`config.TelemetryConfig`** (`config.go`): user settings + pricing
  overrides (`DefaultPriceBook`). `TelemetryConfig.WarnFraction` (soft-warn threshold,
  `[0,1]`) and `TelemetryConfig.HardCapCredits` (absolute credit ceiling, `>= 0`,
  `0` = off) back the budget guardrails. — see [ADR-0008](adr/0008-budget-guardrails-soft-warn-and-hard-cap-gate.md)
- **`telemetry.SpendStore`** / **`telemetry.SpendRecord`** (`history.go`): the persisted
  spend ledger at `<configDir>/spend.json`. On-disk shape is a versioned envelope
  `{"version":1,"records":[…]}`; each record is
  `{at, session?, model, in, cached, out, usd, aiu?}` (JSON tags are the contract).
  Written **atomically** (temp-file + rename); missing file = empty, present-but-invalid
  = error. **Migration note:** `version` gates the schema and the `records` array is the
  stable surface — bumps must add fields only (older readers ignore unknown keys; newer
  readers tolerate a higher `version`) or ship a converting migration. — see [ADR-0009](adr/0009-persisted-spend-history-append-only-ledger.md)

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
