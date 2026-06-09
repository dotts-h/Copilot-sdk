---
id: 0056
title: "PostToolUse hook command execution — a bounded local command with untrusted, display-only output (roadmap v10, V28 = G5)"
status: closed
severity: high
group: 0052
github:
links:
  adr: [0032]
  prs: [87]
  issues: [0052]
---

## Summary

The fifth and **final build** child of the safe-autopilot governance epic
[0052](0052-epic-safe-autopilot-governance.md) (roadmap v10), following the hooks foundation
[0053](0053-hooks-foundation-forge-entity-bridge-evaluator.md) (V25), the dangerous-action floor
[0054](0054-dangerous-action-deny-mandatory-hitl.md) (V26), and the Hooks UI + mode binding +
timeline why [0055](0055-hooks-ui-mode-binding-timeline-why.md) (V27). It is the **external
command-ref hook execution** deferred since ADR-0029: a **PostToolUse** hook that runs a user-defined
local command after a matching tool completes, treating the command's output as **untrusted**. This
**closes epic 0052**.

**V28 lands (ADR-0032):**

- **The command field.** `ctxforge.Hook` gains `Command`/`CommandArgs` (omitempty, backward-readable),
  validated **post-only** (a PreToolUse hook carrying a command is rejected) with no dangling `${VAR}`
  (reusing `hasDanglingVarRef`). The domain stays pure — it validates the *shape*, never resolving or
  executing. `PostToolUseCommands(...)` is the pure selector of the matching command hooks.
- **The executor** (seam). Off the tool-completion flow (`EvToolEnd`), the `SDKClient` runs each
  matching PostToolUse command — the program exec'd **directly** (no shell, no chaining), `${VAR}`
  resolved at execution via the same env seam as MCP (ADR-0020; an unset ref → empty, never the
  literal), a **5s timeout**, ~2KB of bounded combined output, and the **workspace** as cwd. The
  output is **untrusted**: emitted only as an `EvHookRun` annotation, never fed back to the agent and
  **never** consulted on the permission path — a post-tool command can never flip a decision. A
  non-zero exit is annotated, not a gate.
- **UI + timeline.** The Hooks form gains the command field + a **command preflight**
  (`POST /hooks/command-preflight`) that shows the resolved command line and flags an unset `${VAR}`,
  **never executing**. The timeline surfaces a run as a compact, **escaped** `convo.RoleHookRun`
  annotation (the **hook-run note**) carrying the hook id + a bounded output snippet.

## Acceptance

- [x] The command field validates (post-only, dangling-`${VAR}` rejected, slug rules); a PreToolUse
      hook with a command is rejected; a pre-G5 `forge.json` loads backward-readable — domain-tested.
- [x] A matching PostToolUse hook runs its command and the bounded, escaped output reaches the
      timeline; a timeout is enforced; a missing `${VAR}` resolves unset (never the literal); output
      can **never** flip a permission/decision — seam-tested.
- [x] The form round-trips the command + a command preflight reports the resolved command + unset
      `${VAR}` warnings; built-ins stay read-only — web seam-tested.
- [x] Go gates green (`make lint && make test`, coverage floor 65%) and `make e2e` green
      (`hooks.spec.ts`: a PostToolUse command hook + its preflight); self-review with `/code-review`.
      Born in its PR; ADR-0032 + CONTEXT/CONTRACTS/CODEMAP folded into the branch.

## Out of scope (future work)

- Anything beyond a **single bounded local command** per PostToolUse hook: **no chaining/pipelines**
  (name a script for those), and **no PreToolUse command-gates** that deny on exit code — that would
  make untrusted output a control surface, which this child explicitly forbids.
- Post-tool **match** `toolKind` is **best-effort** (derived from the tool name via `toolKindFromName`,
  since the completion event carries no `req.Kind()`); it covers the built-in tools + MCP and falls back
  to a `pattern` (matched against the tool name + argument summary) for an unknown tool. The annotation
  surfaces in the **chat** timeline (not a workflow lane) and is **not persisted** across resume —
  matching the ADR-0031 scope.

## Resolution (shipped)

Shipped in **PR #87** (V28 = G5). `Hook.Command`/`CommandArgs` + `PostToolUseCommands` (pure domain),
the seam executor off `EvToolEnd` with the `runCmd`/`lookupEnv`/`hookTimeout` seams + `EvHookRun`,
`convo.RoleHookRun` + the escaped hook-run note, and the form command field + `/hooks/command-preflight`.
ADR-0032 records the field shape, the executor policy (5s timeout / ~2KB bound / workspace cwd / direct
exec), and the untrusted-output discipline (display-only, never a gate, never agent-visible). **Epic
0052 is fully shipped** with this child.
