# 0032. PostToolUse hook command execution: a bounded local command with untrusted, display-only output

- Status: accepted
- Date: 2026-06-09
- Deciders: Horia
- Related: the **fifth and final build child** (G5) of the safe-autopilot governance epic
  ([0052](../issues/0052-epic-safe-autopilot-governance.md), roadmap v10) — issue
  [0056](../issues/0056-posttooluse-hook-command-execution.md). Builds on
  [ADR-0029](0029-hooks-forge-entity-bridge-enforced-allow-deny-ask-safe-read-defaults.md) (the
  `ctxforge.Hook` entity, the pure evaluator, built-ins-as-hooks; it noted PostToolUse
  "observes/logs only in this build — no command execution yet"),
  [ADR-0030](0030-dangerous-action-deny-and-mandatory-hitl-unbypassable-by-config.md) (the mandatory
  floor + the `OutsideWorkspace` fence), and
  [ADR-0031](0031-hooks-management-ui-mode-binding-and-timeline-why.md) (the Hooks UI, mode binding,
  and the `EvToolDecision` timeline "why"). Reuses the `${VAR}` reference shape from
  [ADR-0020](0020-mcp-secrets-via-env-var-reference-indirection.md) and the escape-everything output
  discipline from [ADR-0001](0001-server-rendered-htmx-over-spa.md). Touches `internal/ctxforge`
  (the `Hook.Command`/`CommandArgs` field + `PostToolUseCommands`), `internal/copilot` (the executor
  off the tool-completion flow + the `EvHookRun` event/`HookRun` payload), `internal/convo`
  (`RoleHookRun` + `HookRunView`), `internal/web` (the form's command field + command preflight + the
  timeline render), and the doc records (CONTEXT **hook command** term, CONTRACTS §1/§2/§3/§4, this
  ADR).

## Context

ADR-0029 shipped hooks as a first-class forge entity with a Pre/PostToolUse `event`, but only the
**PreToolUse** path is wired to behavior (the allow/deny/ask gate). A **PostToolUse** hook is inert —
it "observes/logs only". The epic's remaining promise (0052, G3 notes; 0055 "out of scope") is
**external command-ref hook execution**: a PostToolUse hook that runs a user-defined local command
after a matching tool completes — e.g. run `gofmt`/a linter after a write, emit a notification, touch
a marker file. This closes epic 0052.

The decisions this ADR settles: **the command field's shape** (and how it executes), **the executor's
policy** (timeout, output bounding, cwd, `${VAR}` resolution), and — the load-bearing one — **how the
command's output is treated**: it is **untrusted** and must never become a control surface.

## Considered options

- **Command field shape + execution.**
  - **`Command string` + `CommandArgs []string`, exec'd DIRECTLY (no shell) (chosen).** The program
    and its argv are separate fields; the executor runs them via `exec.CommandContext` with **no
    shell** — so there are no pipes, redirects, globbing, or command chaining, and no shell-injection
    surface. This matches the epic's "single bounded local command" scope exactly. Each field may carry
    a `${VAR}` reference resolved at execution.
  - *A single `Command string` run through `sh -c` (rejected).* It would allow pipes/redirects, but
    re-introduces a shell interpreter (an injection surface over the resolved `${VAR}` values) and
    blurs the "no chaining" boundary the epic draws. A post-tool command that needs a pipeline can name
    a script as its program — the capability isn't lost, only the implicit shell.

- **Where the command lives + validation.**
  - **An additive `command`/`commandArgs` on `ctxforge.Hook`, validated post-only, pure (chosen).**
    Reusing the existing entity (not a parallel type) keeps one CRUD/persist/preflight story. `Validate`
    permits a command **only on a `post-tool-use` hook** — a PreToolUse hook carrying a command is
    **rejected** — and reuses `hasDanglingVarRef` so a malformed `${VAR` never persists. The domain stays
    **pure**: it validates the field's *shape* but never resolves `${VAR}` or executes anything (ADR-0020
    keeps the reference's *meaning* out of `ctxforge`). A pure `PostToolUseCommands(...)` selects the
    matching command hooks for the seam — the post-path companion to `Evaluate`, making **no**
    allow/deny/ask decision.
  - *A new `CommandHook` type (rejected).* A second entity would duplicate CRUD, persistence, the UI
    form, and the matcher for no gain — the `Match`/`Modes`/`Enabled` machinery is identical.

- **Output handling — the load-bearing decision.**
  - **Untrusted, bounded, display-only telemetry; never a gate, never agent-visible (chosen).** A
    PostToolUse command's stdout/stderr is **attacker-influenced** (a tool may have written content the
    command echoes). It is therefore: **bounded** (~2KB, capture stops at the cap so a runaway command
    can't exhaust memory or flood the timeline); **escaped** (rendered through `html/template`, ADR-0001,
    so it can never inject markup); **never fed back to the agent** (it is not appended to the
    conversation, unlike a tool result); and **never consulted on the permission path** (a decision is
    resolved *pre*-tool, before any command runs). A non-zero exit is **surfaced** as an `exited N`
    annotation but is **not** a gate — explicitly *not* a PreToolUse-style deny-on-exit-code, because
    that would make untrusted output a control surface.
  - *Feed output back as context / let exit code gate (rejected).* Either turns untrusted output into a
    prompt-injection or a control surface — the exact failure mode the epic's "treat output as untrusted"
    clause forbids.

- **Where the executor fires.**
  - **Off the tool-completion flow in the seam (chosen).** Hooks ride the **compiled** `SessionSpec`, so
    the executor consults the per-session policy at `EvToolEnd` and needs **no** live forge at the SDK
    boundary. The SDK's completion event carries no tool kind/arguments, so the match context (an `mcp`
    kind for MCP tools, else empty; the tool name + its one-line argument summary as the match string) is
    captured at tool **start** keyed by `ToolCallID` and read at completion. Each command runs in its
    **own goroutine** (off the event-handler path) with a **5s timeout** and the session **workspace** as
    cwd, then emits an `EvHookRun` annotation — so a command's latency never blocks event delivery, which
    is correct for display-only telemetry. The completion event reports only a tool **name** (not the
    `req.Kind()` the pre-tool path gets), so the match `toolKind` is a **best-effort** map over the
    built-in Copilot tools (`toolKindFromName`: `edit`→write, `bash`→shell, `view`/`grep`→read, an MCP
    tool→mcp, unknown→empty so the hook falls back to `Pattern`). A mis-classification only runs/skips a
    side-effect command — never a control-surface decision — so a heuristic is acceptable here where it
    would not be on the pre-tool gate.

## Decision

Add `Hook.Command`/`CommandArgs` (omitempty, backward-readable), valid **only on a post-tool-use hook**
and rejecting a dangling `${VAR}`; `ctxforge` stays pure (shape only). The seam (`SDKClient`) runs the
executor off `EvToolEnd`: `ctxforge.PostToolUseCommands` selects the matching enabled command hooks, and
each is run via a `runCmd` seam (default `execCommand`, **direct exec, no shell**) with `${VAR}` resolved
via a `lookupEnv` seam (default `os.Getenv`; an unset reference → empty, **never** the literal), a 5s
timeout, ~2KB bounded combined output, and the workspace as cwd. The result is a normalized **`EvHookRun`**
event (`HookRun{HookID, Command, Output, ExitCode, TimedOut, Failed}`) reduced into a compact
`convo.RoleHookRun` annotation; `Output` is auto-escaped. The Hooks form gains the command field + a
**command preflight** (`POST /hooks/command-preflight`) that shows the resolved command line and flags an
unset `${VAR}` — **never executing**. The command's output is untrusted, display-only, and **never** a
gate or agent-visible.

## Consequences

- **PostToolUse hooks now *do* something.** A user can run a formatter/linter/notifier after a matching
  tool — the last epic deliverable. Mode binding and the mandatory floor still govern *whether the
  triggering tool ran*; the command is a side effect of a completed call, not a new gate.
- **Untrusted output stays off the control surface — table-tested.** The seam test asserts a post-hook
  command printing "deny" cannot flip a later read's auto-approve; output is bounded, escaped, and never
  appended to the conversation. A timeout is enforced; a missing `${VAR}` resolves unset, never the
  literal.
- **`ctxforge` stays pure.** The command is opaque domain data the seam resolves and executes; no SDK
  type or `os`/`exec` dependency enters the domain. The `${VAR}` *meaning* is decoded only in the seam
  and the web preflight (ADR-0020), not in `ctxforge`.
- **Additive + backward-readable.** `command`/`commandArgs` are omitempty — a pre-G5 `forge.json` loads
  unchanged. `EvHookRun`/`HookRun` and `RoleHookRun` are new event/transcript-vocabulary additions; older
  replays without them are inert (the annotation is **live, not persisted** — like the `EvToolDecision`
  "why" — so it is not replayed on resume).
- **Accepted scope limits (future work).** A single bounded local command per hook — **no chaining, no
  pipelines** (name a script for those), **no PreToolUse command-gates** that deny on exit code (that
  would make untrusted output a control surface — explicitly forbidden). Post-tool **match** `toolKind`
  is **best-effort** (`toolKindFromName` maps the tool name, since the completion event carries no
  `req.Kind()`): it covers the built-in tools + MCP and falls back to the `Pattern` (matched against the
  tool name + argument summary) for an unknown tool. The annotation surfaces in the **chat** timeline,
  not a workflow lane, and is not persisted across resume — matching the `EvToolDecision` scope (ADR-0031).
