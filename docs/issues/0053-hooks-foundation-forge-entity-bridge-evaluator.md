---
id: 0053
title: "Hooks foundation — the Hook forge entity + bridge evaluator + safe-read defaults (roadmap v10, V25 = G0 + G3-mechanism + G1)"
status: open
severity: high
group: 0052
github:
links:
  adr: [0029]
  prs: []
  issues: [0052]
---

## Summary

The first **build** child of the safe-autopilot governance epic
[0052](0052-epic-safe-autopilot-governance.md) (roadmap v10). It makes hooks a **real, enforced,
first-class forge primitive** — *before* the management UI exists — by landing the domain entity, the
bridge evaluator, and the safe-by-default policy together. It folds the epic's **G0** (policy model +
seam), the **G3 mechanism** (the `Hook` forge entity, persisted + compiled), and **G1** (the
default safe policy) into one PR; the dangerous-deny ruleset (G2) and the management UI (G4) are the
next children.

Before this change the permission gate was **all-or-nothing**: `permissionHandler` always emitted an
interactive `EvPermission`, and the only automation was the flat `AutoApproveTools` approve-everything
switch. There was no place to enforce a per-tool policy a user couldn't bypass.

**V25 lands:**

- **A pure `ctxforge.Hook` forge entity.** `{id, event (pre-tool-use|post-tool-use), match {toolKind?,
  pattern?}, action (allow|deny|ask), reason?, enabled}`, persisted under an additive `hooks` key on
  `forge.json` and CRUD-managed via the shared `mutate` rollback discipline
  (`AddHook`/`UpdateHook`/`ToggleHook`/`RemoveHook`, like MCP/snippet). `Validate` rejects an unknown
  event/action, an empty match, an invalid tool kind, and a dangling `${VAR}` reference.
- **A pure evaluator.** `Evaluate(hooks, event, toolKind, command) Decision` — a hook participates when
  `Enabled`, its `Event` matches, and its `Match` applies (empty `toolKind` = any kind; a `*`/`?`
  pattern is a glob over the whole command, else a substring). Precedence is **deny > ask > allow** and
  the no-match default is **ask** (fall through to the gate) — order-independent and deterministic.
  Table-tested exhaustively.
- **Bridge enforcement.** `Forge.Compile` folds the built-in defaults + the enabled user hooks into
  `SessionSpec.Hooks`, threaded through the seam via `web.SeamSpec` (mirroring `MCPServers`) and
  recorded per `SessionID`. `permissionHandler` consults `Evaluate` **before** the gate: allow →
  `PermissionDecisionApproveOnce` (no `EvPermission` emitted), deny →
  `PermissionDecisionReject{Feedback: reason}`, ask → the existing emit-and-block gate. This
  generalizes `AutoApproveTools` into a per-tool ruleset, enforced in the bridge.
- **Safe defaults (G1).** `ctxforge.DefaultHooks()` auto-approves read-only tool kinds and leaves
  writes/shell/MCP to the gate; built-ins run through the **same** `Evaluate` as user hooks, so a user
  `deny` on reads still wins. The default build is safe out of the box.

It takes **ADR-0029** for the decisions (forge entity vs. config flag; the evaluator's match/precedence
semantics; bridge enforcement; built-ins-as-hooks; the `copilot → ctxforge` pure-domain coupling). The
seam's `copilot.SessionSpec` gains a `Hooks` field; `internal/copilot` imports the pure `ctxforge` for
the single shared `Hook` type + evaluator. **No new HTTP route, no UI** — the schema is additive and
forge-only; a pre-V25 `forge.json` loads unchanged.

## Acceptance

- [ ] `ctxforge.Hook` is a first-class forge entity persisted under `forge.json`'s additive `hooks`
      key, with `Validate` rejecting unknown event/action, empty match, invalid tool kind, and dangling
      `${VAR}`; CRUD builders mirror the other entities.
- [ ] `Evaluate` is pure and table-tested: allow/deny/ask precedence (deny > ask > allow), pattern
      match (glob + substring), disabled-hook-ignored, event-mismatch, empty-set → ask default.
- [ ] `Forge.Compile` includes the compiled hook set (built-in defaults + enabled user hooks) in the
      session spec, deterministically.
- [ ] The bridge consults the policy in `permissionHandler` before the gate: a read tool is
      auto-approved with **no** gate emitted; a denied pattern rejects with the hook's reason; an ask
      falls through to the gate — seam-tested.
- [ ] Read-only tools are auto-approved by the built-in defaults; writes/shell are gated; the default
      build is safe out of the box; built-ins run through the same evaluator as user hooks.
- [ ] Go gates green (`make lint && make test`, coverage floor 65%) and `make e2e` green; self-review
      with `/code-review`. Born in its PR; ADR-0029 + CONTEXT/CONTRACTS/CODEMAP folded into the branch.

## Out of scope (the next children)

- **G2** — dangerous-action deny ruleset + mandatory HITL: `rm -rf $HOME`/root, `curl|sh` /
  pipe-download-into-shell-or-editor, writes outside the workspace, `sudo`, obvious exfiltration —
  hard-denied or forced through a mandatory gate **even in auto mode**, enforced in the bridge.
- **G4** — the Hooks management UI: add/edit/enable-disable/remove CRUD (list + form + preflight) like
  the MCP/workflow pages, mode binding, and timeline "why" surfacing.
- **External command-ref hook execution** — PostToolUse running a user command with `${VAR}` +
  preflight, treating output as untrusted.
