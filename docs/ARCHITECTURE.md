# my-orchestra — Architecture

This document is the engineering deep dive. The
[GitHub Pages site](https://dotts-h.github.io/Copilot-sdk/) is the friendlier overview.

## Design goals

1. **Cost is a first-class citizen.** Token accounting and credit conversion are
   core domain logic, not an afterthought.
2. **The UI is testable without a network.** The web layer depends on the
   `copilot.Client` interface, so handlers and the event→fragment reducer run
   against an in-memory mock — no browser or runtime required.
3. **Context is reproducible.** What the agent "knows" is compiled from a
   file-backed forge, so it is diffable and shareable.
4. **Pure core, thin edges.** Business logic is dependency-free Go; the SDK and
   the HTTP/SSE transport are kept at the boundary.

## Module map

```
cmd/my-orchestra        entrypoint: load config+forge, build meter, dial client, serve web
internal/web            net/http server: handlers, SSE hub, html/template partials, vendored htmx
internal/convo          UI-agnostic transcript model: Turn · ToolView · State reducer (pure)
internal/copilot        copilot.Client interface · SDKClient (Go SDK) · MockClient
internal/ctxforge       Skill/Instruction/Agent/MCPServer · Forge.Compile → SessionSpec
internal/telemetry      PriceBook · Meter (concurrent) · Budget · Cost · AIU
internal/config         Config (settings + key bindings), JSON, atomic save
```

## The `copilot.Client` seam

The single most important boundary. The web layer never imports
`github.com/github/copilot-sdk/go`. It depends only on:

```go
type Client interface {
    CreateSession(ctx, spec SessionSpec) (string, error)
    Send(ctx, sessionID, prompt string) error
    Abort(ctx, sessionID string) error
    Events() <-chan Event
    Close() error
}
```

Two implementations:

- **`SDKClient`** — wraps the official SDK. It creates a `sdk.Client`, opens
  sessions, registers an `On(...)` handler, and **normalizes** the SDK's rich
  event stream into a compact `Event` the UI understands. This is the only file
  that knows the SDK exists.
- **`MockClient`** — in-memory. Records `Send`/`Abort` calls and lets a test push
  arbitrary events. Powers the offline mode and every web/convo unit test.

Because the seam emits already-decoded `Event`s, the web layer's `handleEvent`
reducer is a pure mapping from `Event` to HTML fragments over SSE — easy to test
against the mock, impossible to flake.

### Event normalization

The SDK exposes many event types. `SDKClient.makeHandler` collapses them:

| SDK event | normalized |
|-----------|------------|
| `AssistantMessageDeltaData` | `EvMessageDelta` (text) |
| `AssistantReasoningDeltaData` | `EvReasoningDelta` (text) |
| `AssistantReasoningData` | `EvReasoning` (full thinking block) |
| `AssistantMessageData` | `EvMessage` (full text) |
| `AssistantUsageData` | `EvUsage` (normalized tokens + AIU) |
| `ToolExecutionStartData` | `EvToolStart` (name + `ToolCall{ID, Args, MCPServer}`) |
| `ToolExecutionProgressData` | `EvToolProgress` (`ToolCall{ID, Progress}`) |
| `ToolExecutionCompleteData` | `EvToolEnd` (`ToolCall{ID, Success, Result}`) |
| `SessionIdleData` | `EvIdle` |

Tool completion events don't carry a name, so `SDKClient` keeps a
`toolCallID → name` map populated on start and consumed on completion.

**Tool timeline & reasoning split.** Tool events carry a normalized `ToolCall`
threaded by `ID` across start → progress → completion. `summarizeArgs` reduces
the raw argument map to a one-line summary (shell command, file path, or compact
JSON) and `toolResultText` prefers the SDK's `DetailedContent` (full diffs) over
the model-facing summary, surfacing the error message on failure. The chat state
renders each tool as a first-class timeline `Turn` (status glyph, args, live
progress, bounded result) interleaved in execution order. Reasoning is kept in a
**separate buffer** from assistant message text — switching modes commits the
other buffer first — so "thinking" renders as its own dim block and is never
concatenated into the answer.

### Interactive permissions (sync ↔ async bridge)

When auto-approve is off, the SDK calls `OnPermissionRequest` and expects a
decision **synchronously**. The web UI answers asynchronously, so `permBridge`
mediates: the callback `begin()`s a one-shot channel and emits an `EvPermission`;
the server queues it and streams an inline **approve / reject** form over SSE; a
`POST /perm/{id}` calls `Client.Respond`, which `resolve()`s the channel and
unblocks the callback with `ApproveOnce` or `Reject{Feedback}`. If the client
closes first, the callback selects `done` and returns `UserNotAvailable`, so it
never hangs. Requests queue FIFO; decisions match by id, so out-of-order answers
are safe.

### Forge CRUD

The Skills/Instructions/Agents pages toggle (`POST /skills/{id}/toggle`), select
(`POST /agents/{id}/select`), and delete (`POST /{kind}/{id}/delete`) entities,
each swapping the affected fragment back over htmx. Construction goes through
pure, validated builder functions; saves **roll back** the in-memory forge if
validation fails, so `forge.json` is never written in a bad state. Add/edit
forms are a tracked follow-up (see `docs/WEB_UI_PLAN.md`).

### Slash commands (planned)

Slash commands (`/model`, `/agent`, `/attach`, …) are not yet wired into the web
composer — a tracked follow-up. The legacy intent was: intercept `/`-prefixed
input (never sending it to the agent): `/help /clear /cost /skills /agents
/settings`, plus `/model <name>` and
`/agent <id>` which persist the choice and restart the session.

## Telemetry: estimate + authoritative

GitHub bills tokens → USD → **AI Credits** (`1 credit = $0.01`). Two independent
signals:

1. **Estimate** (`telemetry` package). A `PriceBook` maps each model to
   per-million-token USD rates for input / cached / output. `Price()` is a pure
   function; `Meter` accumulates it concurrently and exposes per-model totals
   sorted by spend. Model names are normalized (`GPT-5`, `gpt_5`, `gpt-5` all
   match), and unknown models fall back to a non-zero rate so nothing is ever
   "free by accident".
2. **Authoritative** (from the runtime). `AssistantUsageData.CopilotUsage.TotalNanoAiu`
   is GitHub's own per-call cost in nano-AI units. The meter accumulates it
   (`RecordReportedAIU`) and the Telemetry page shows it beside the estimate.

Reasoning tokens are billed at the output rate, so the server folds
`reasoningTokens` into output when recording usage.

The pricing engine has a **fuzz** target asserting cost is total (never negative
or NaN for non-negative token inputs), and the meter has a **concurrency** test.

## The my-ctx forge

`forge.json` holds four kinds of building block: **Skill**, **Instruction**,
**Agent**, **MCPServer**, each with a slug `id` and `Validate()`. `Forge.Validate`
additionally enforces unique IDs per kind and that agents only reference real
skills.

`Forge.Compile(agentID)` is the heart: it deterministically assembles a
`SessionSpec`:

```
SessionSpec
├─ Model / ReasoningEffort   ← from the selected agent (optional)
├─ SystemMessage             ← agent message
│                              + enabled instructions (priority asc, then id)
│                              + active skill prompts
├─ EnabledSkillIDs           ← agent-pinned (in order) ∪ globally-enabled, de-duped
├─ SlashCommands             ← from skills that declare a command
└─ MCPServers                ← enabled servers only
```

Determinism (stable sort, ordered de-dup) means the same forge always yields the
same context — important for reproducibility and for snapshotting in tests.

## Web UI (htmx + SSE)

The frontend is server-rendered HTML streamed over Server-Sent Events — no JS
build chain. The design and the full event→fragment contract live in
[`WEB_UI_PLAN.md`](WEB_UI_PLAN.md). In outline:

- **Transport.** `GET /events` opens one long-lived SSE stream that ranges over
  `Client.Events()`; client→server actions (`POST /send`, `/perm/{id}`, `/abort`,
  forge toggles, `GET /page/{name}`) are ordinary htmx requests. Decoupling send
  from receive respects SSE's unidirectionality and keeps the composer live.
- **Event → fragment reducer.** `session.go:handleEvent` maps each normalized
  `copilot.Event` to an HTML fragment: message/reasoning deltas append to the
  current bubble (fast path); structural events (tool start/progress/end, message
  finalize) re-render `#timeline`; `EvUsage` swaps the cost footer out-of-band;
  `EvPermission` streams the inline approve/reject form. All model text is routed
  through `html/template`/`esc()` so output is escaped.
- **Transcript model.** The append/finalize logic lives in the pure
  `internal/convo` package (`State`, `Turn`, `ToolView`) — UI-agnostic and
  unit-tested independently of the HTTP layer.
- **State.** Single local user → one in-memory session on the `Server` (the old
  TUI `Model` minus rendering): active `sessionID`, the `convo` transcript, the
  permission queue, pending attachments.

Handlers and the reducer are exercised by `internal/web` tests using the mock
client and synthetic events — no browser required. Vendored `htmx.org` +
`htmx-ext-sse` live under `internal/web/static/`.

## Failure & offline behavior

- `dialClient` authenticates with the logged-in `copilot` CLI session by default
  (`copilot.ResolveAuth`); an explicit token is used only when `githubTokenEnv`
  names a set env var. If the `copilot` CLI is missing or not logged in, it logs a
  notice and returns a `MockClient`; the server still starts and every page works.
- Config and forge **Load** treat a missing file as "empty/defaults", so first
  runs never error. **Save** is atomic (temp file + rename) and validates first.

## CI/CD

- **CI** (`.github/workflows/ci.yml`): gofmt check, `go vet`, golangci-lint;
  race tests with a coverage floor; a fuzz smoke job; a 6-target build matrix.
- **Pages** (`pages.yml`): deploys `docs/` (this site) on push to `main`.
- **Release** (`release.yml`): on a `v*` tag, cross-compiles six targets, writes
  `checksums.txt`, and publishes a GitHub Release.

## Testing philosophy (TDD + SDET)

Each package was written test-first. Beyond happy paths we assert on:
edge cases (unknown models, empty forge, upgrade-time config backfill),
invariants (pricing totality via fuzz), concurrency (meter under 16×100 writers),
and translation correctness (every SDK event → expected normalized event).
