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
cmd/my-orchestra        entrypoint: serve the web UI (pure-Go, CGO-free)
cmd/my-orchestra-desktop native desktop window over the same UI via Wails v3 (build tag `desktop`)
internal/bootstrap      shared assembly: config+forge → meter → spec → client → *web.Hub; ServeLocal
internal/web            net/http server: handlers, SSE hub, html/template partials, vendored htmx
internal/convo          UI-agnostic transcript model: Turn · ToolView · State reducer (pure)
internal/copilot        copilot.Client interface · SDKClient (Go SDK) · MockClient
internal/ctxforge       Skill/Instruction/Agent/MCPServer/Workflow · Forge.Compile → SessionSpec
internal/telemetry      PriceBook · Meter (concurrent) · Budget · Cost · AIU · SpendStore (persisted ledger)
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
  event stream into a compact `Event` the UI understands. It is the only
  component that imports the SDK, split across three same-package files by
  responsibility: `sdkclient.go` (session lifecycle + `Client` impl),
  `normalize.go` (the pure SDK→`Event` translation), and `handlers.go` (the
  sync↔async permission/input/plan/elicit bridges).
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
concatenated into the answer. The runtime emits a full `AssistantReasoningData`
block after a segment that already streamed as deltas; `SDKClient` tracks whether
a segment streamed and **drops the duplicate full block**, so the thinking is
never rendered twice.

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
pure, validated builder functions; **every** `Add*`/`Update*`/`Remove*` routes
through one `Forge.mutate(apply)` that snapshots the forge, applies the change,
runs a whole-forge `Validate`, and **rolls back** on failure — so `forge.json` is
never written in a bad state and referential checks (agent→skill, workflow→agent)
are enforced uniformly. The web add/edit form lifecycle (New/Edit/Create/Update/
Delete) for the uniform entities is a generic `forgeCRUD[T]` (skills,
instructions, agents, snippets); the lock/lookup/list-fallback and
save-or-re-render-on-error logic lives once. The forge→seam `SessionSpec`
translation (startup, agent-restart, workflow lane) is the single pure
`web.SeamSpec`.

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
`reasoningTokens` into output when recording usage. Prompt-cache **writes** and
reasoning tokens are also accumulated as display-only counts (not re-priced) to
feed the live chat **statusline**, which shows the active model and mode, the
context-window fill, the session timer, message/tool counts, the in/out/
cache-read/cache-write token split with cache-hit rate, and the running
credits/USD.

The pricing engine has a **fuzz** target asserting cost is total (never negative
or NaN for non-negative token inputs), and the meter has a **concurrency** test.

The live `Meter` is in-memory and resets on restart; a persisted, append-only
**`SpendStore`** (`internal/telemetry/history.go`) is the accountable ledger
behind it. Each metered turn appends a `SpendRecord` to `<configDir>/spend.json`
(a versioned JSON document written atomically — temp-file + rename — exactly like
config); the Telemetry page reads it back as a trend (spend over time, per-model
share, and a per-agent / per-workflow cost breakdown) and a
`GET /telemetry/export.csv` download. The store is the one IO edge in an otherwise
pure package: `DailyTotals`/`ModelShares`/`AgentShares`/`WorkflowShares`/
`MonthToDate`/`Forecast`/`WriteCSV` are pure functions over a record slice. An empty dir makes
it ephemeral (demo/tests never write to a real config directory). See
[ADR-0009](adr/0009-persisted-spend-history-append-only-ledger.md).

**Attribution (orchestration-aware cost).** Each `SpendRecord` is tagged
additively (schema v2) with the **agent** persona that incurred it and, when a
workflow run owned the turn, the **workflow id + lane index** — so the ledger
answers *"which agent / which workflow burned the budget?"* across time. The chat
reducer tags the active `Server.agentID`; the workflow-lane reducer tags the run +
lane through the same `recordUsage`. `AgentShares` (empty-agent bucket = built-in
chat, included) and `WorkflowShares` (non-workflow spend excluded) roll the tags
up for the Telemetry breakdown. v1 ledgers read back with empty tags. See
[ADR-0018](adr/0018-additive-attribution-tags-on-spend-records.md).

**One source per surface (three now).** The account-wide budget accounting —
the "Total cost / Monthly budget / Remaining" rows, the topbar cost footer,
`/cost`, and the hard-cap projection baseline — reads **month-to-date from the
ledger** (`telemetry.MonthToDate`, UTC calendar month), so "remaining this month"
survives a restart rather than resetting with the process. The **per-session
statusline** (`sessionMeter`, ADR-0011) and the **live token split** (cache-write /
reasoning display counts, cache-hit rate) stay on the in-process meter — this-
conversation and this-process signals the ledger doesn't carry. A single shared
`recordUsage` helper (`session.go`) records every turn into both meters **and** the
ledger, so no surface drifts. See [ADR-0016](adr/0016-ledger-is-source-of-truth-for-account-wide-budget-accounting.md).

**Predictive (burn-rate forecast).** From the same ledger, `telemetry.Forecast`
(a pure reader over `DailyTotals` + the `Budget` allowance) projects *when* the
monthly allowance is exhausted at the recent spend rate — moving cost from
**reactive** (warn at 80%, hard cap) to **predictive**. The slope is a
trailing-7-day average (not a regression — robust on a sparse single-user
series), "used" is month-to-date (consistent with `MonthToDate`), and every
degenerate case is explicit in `Projection.Status` (no budget / idle / exhausted
/ ok). The Telemetry page shows a forecast sentence ("at ~X cr/day … around
&lt;date&gt; — ~N turns left") and the statusline a compact `cap ~Nd` cell that
ambers when on track to blow the budget before month-end. Pure, no IO, no schema
change. See [ADR-0019](adr/0019-budget-burn-rate-forecast-trailing-window-average.md).

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

A fifth entity, **Workflow**, composes agents into a multi-agent run: an ordered
list of `WorkflowStep{AgentID, Prompt, When?}` plus a `Mode` (sequential | parallel).
`CompileWorkflow(id)` reuses `Compile` to produce one `SessionSpec` per step;
the web layer runs each step as a **lane** — a sub-run on the seam's
`CreateSession`/`Send` lifecycle, watched in a dedicated panel. Sequential mode
hands each lane's output to the next; parallel fans them out, each lane attributed
by the event's `SessionID` (`laneFor`). A lane card surfaces its own tool timeline
and inline file-write permissions, not just output + cost. The parallel path is now
exercised offline — `MockClient.CreateSession` returns distinct ids and the demo
lane tags its events with them — so the demo/e2e drive concurrent lanes. The run
logic is a pure `workflowRun` state machine (`internal/web/workflow.go`),
unit-tested with no client.

**Branching (control flow, B2).** A step may carry a `When` predicate
(`StepCondition{Step, Condition, Value}`) gating it on a prior step's settled
outcome (`succeeded` / `failed` / `output-contains` / `always`) — moving a workflow
from a fixed pipe to real control flow (e.g. *"run the fix agent only if the review
flagged issues"*). `Step` references a **strictly-prior** step (1-based), so a
dependency cycle is structurally impossible and the run always terminates. The
engine evaluates the predicate as a pure function over settled lanes (`evalWhen`):
in sequential mode `advance` walks forward, running the first satisfied step and
**skipping** unsatisfied ones; in parallel mode `evalPending` launches every lane
whose dependency has settled (to a fixpoint), run-or-skip. An unsatisfied step is
**skipped** (`laneSkipped`, rendered distinctly), not failed, and a skipped lane
still counts as settled so the run completes. See
[ADR-0013](adr/0013-multi-agent-workflow-run-handoff-surface.md),
[ADR-0017](adr/0017-per-lane-tool-and-permission-surface-for-parallel-workflow-lanes.md),
[ADR-0021](adr/0021-conditional-branching-workflow-steps-declarative-predicate.md).

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

## Desktop shell (Wails v3)

`cmd/my-orchestra-desktop` wraps the exact same UI in a native OS window. It is a
thin edge: `internal/bootstrap.Build` produces the same configured `*web.Hub` the
web binary uses, `bootstrap.ServeLocal` serves that `http.Handler` on an ephemeral
**loopback** port, and a Wails `WebviewWindow` is pointed at
`http://127.0.0.1:<port>/`. Because the window loads an external loopback URL
(not Wails' `wails://` asset protocol), **SSE streams natively** in the OS webview
(WebView2 / WKWebView / WebKitGTK) — the UI is byte-identical to the browser, so
the whole existing test pyramid covers it unchanged. We use Wails only for the
window + `OnShutdown` lifecycle (no Go↔JS IPC); teardown is idempotent so the
defer and `OnShutdown` never double-close the client. A `-serve` flag runs
headless (no window) for CI smoke and Playwright.

The shell needs **CGO + the platform webview toolchain**, so it is isolated
behind the `desktop` build tag — the pure-Go `cmd/my-orchestra` and the default
`go build/test ./...` never compile it. It builds on native runners
(`.github/workflows/desktop.yml`), not the `CGO_ENABLED=0` cross-compile in
`release.yml`. See [ADR-0006](adr/0006-desktop-shell-via-wails-v3-localhost-window.md).

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

Fixed bugs are logged in [REGRESSIONS.md](REGRESSIONS.md), each mapped to the
test that now guards it — a fix without a guard is the thing we're trying not to
ship.

The web layer adds a consolidated HTTP **contract** suite (`api_test.go`:
content-types, output escaping/XSS, cookie hardening, per-session isolation, SSE
greeting, malformed-payload tolerance), **benchmarks** for the render/reducer
hot paths, and a **concurrent multi-session load** test (`bench_test.go`) that
drives the full mux from many goroutines under `-race` to catch data races and
lock-ordering deadlocks.

### Browser suite (`e2e/`)

A Playwright suite runs against the offline demo server and covers what the Go
tests can't: real htmx swaps, the live SSE transport, focus/keyboard behaviour,
responsive layout, and WCAG 2.1 A/AA conformance (`axe-core`) — split into
**e2e · api · a11y · ux · perf** layers. Writing it surfaced two real defects
since fixed (the composer wiped input on each keystroke because the form's
`hx-on::after-request` caught the autocomplete GET's bubbled event; the topbar
overflowed at tablet width), plus a documented contrast baseline. See
[`e2e/README.md`](../e2e/README.md).
