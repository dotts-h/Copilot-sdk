# my-orchestra — Architecture

This document is the engineering deep dive. The
[GitHub Pages site](https://dotts-h.github.io/copilot-sdk/) is the friendlier overview.

## Design goals

1. **Cost is a first-class citizen.** Token accounting and credit conversion are
   core domain logic, not an afterthought.
2. **The UI is testable without a network.** The TUI depends on an interface, so
   the entire update loop runs against an in-memory mock.
3. **Context is reproducible.** What the agent "knows" is compiled from a
   file-backed forge, so it is diffable and shareable.
4. **Pure core, thin edges.** Business logic is dependency-free Go; the SDK and
   terminal are kept at the boundary.

## Module map

```
cmd/my-orchestra        entrypoint: load config+forge, build meter, dial client, run TUI
internal/tui            Bubble Tea Model: pages, key handling, views, event→msg plumbing
internal/copilot        copilot.Client interface · SDKClient (Go SDK) · MockClient
internal/ctxforge       Skill/Instruction/Agent/MCPServer · Forge.Compile → SessionSpec
internal/telemetry      PriceBook · Meter (concurrent) · Budget · Cost · AIU
internal/config         Config (settings + key bindings), JSON, atomic save
```

## The `copilot.Client` seam

The single most important boundary. The TUI never imports `github.com/github/copilot-sdk/go`.
It depends only on:

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
  arbitrary events. Powers the offline mode and every TUI unit test.

Because the seam emits already-decoded `Event`s, the TUI's `decodeEvent` is a
trivial, pure mapping to Bubble Tea messages — easy to test, impossible to flake.

### Event normalization

The SDK exposes many event types. `SDKClient.makeHandler` collapses them:

| SDK event | normalized |
|-----------|------------|
| `AssistantMessageDeltaData` | `EvMessageDelta` (text) |
| `AssistantReasoningDeltaData` | `EvReasoningDelta` (text) |
| `AssistantMessageData` | `EvMessage` (full text) |
| `AssistantUsageData` | `EvUsage` (normalized tokens + AIU) |
| `ToolExecutionStartData` | `EvToolStart` (tool name) |
| `ToolExecutionCompleteData` | `EvToolEnd` (name via `toolCallID` map) |
| `SessionIdleData` | `EvIdle` |

Tool completion events don't carry a name, so `SDKClient` keeps a
`toolCallID → name` map populated on start and consumed on completion.

### Interactive permissions (sync ↔ async bridge)

When auto-approve is off, the SDK calls `OnPermissionRequest` and expects a
decision **synchronously**. The TUI answers asynchronously, so `permBridge`
mediates: the callback `begin()`s a one-shot channel and emits an
`EvPermission`; the model queues it and renders `⚠ allow … ? [y]/[n]`; the
keypress calls `Client.Respond`, which `resolve()`s the channel and unblocks the
callback with `ApproveOnce` or `Reject{Feedback}`. If the client closes first,
the callback selects `done` and returns `UserNotAvailable`, so it never hangs.
Requests queue FIFO; decisions match by id, so out-of-order answers are safe.

### Forge CRUD

The Skills/Instructions/Agents pages add (`a`), edit (`e`), and delete (`d`)
entities through a modal form. Construction goes through pure, validated
builder functions; saves **roll back** the in-memory forge if validation fails,
so `forge.json` is never written in a bad state.

### Slash commands

The composer intercepts `/`-prefixed input (never sending it to the agent):
`/help /clear /cost /skills /agents /settings`, plus `/model <name>` and
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

Reasoning tokens are billed at the output rate, so the TUI folds
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

## TUI

A single Bubble Tea `Model` with a `Page` enum. `Update` is a pure reducer over
`tea.Msg`:

- `tea.KeyMsg` → global nav (`tab`/`shift+tab`, `ctrl+c`) then page-specific.
- `copilotEventMsg` → decode to a specific msg, **and re-arm** `listenForEvents`
  so the client's event channel becomes an endless stream of messages.
- `usageMsg` → record into the meter.
- list pages mutate the forge/config and persist atomically on toggle.

Rendering is split by page in `views.go`; styling is centralized in `styles.go`
(palette → precomputed lipgloss styles), making theming a one-struct change.

Everything in `Update` is exercised by `model_test.go` using the mock client and
synthetic key/event messages — no real terminal required.

## Failure & offline behavior

- If the `copilot` CLI / token is missing, `dialClient` logs a notice and returns
  a `MockClient`; the TUI still launches and every page works.
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
