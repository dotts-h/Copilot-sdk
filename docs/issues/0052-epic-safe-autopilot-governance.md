---
id: 0052
title: "Epic: Hooks & safe autopilot — first-class forge-managed Pre/PostToolUse hooks + a safe-by-default tool-governance policy (roadmap v10)"
status: open
severity: high
group:
github:
links:
  adr: [0029, 0030, 0031]
  prs: [84, 83, 85]
  issues: [0053, 0054, 0055]
  regression: []
---

## Charter

The product can run agents on **autopilot** (auto mode) and orchestrate multi-lane workflows — but there
is **no governance layer** that makes that safe. Two concrete gaps in the current code:

1. **The permission gate is all-or-nothing.** `permissionHandler` (`internal/copilot/handlers.go:17`)
   **always** emits an interactive `EvPermission` and blocks for a human decision. The only automation is
   a **flat `AutoApproveTools` allowlist** (`internal/copilot/sdkclient.go:144`) — a list of tool names,
   no pattern matching, no deny side. So a user either click-approves everything or disables approvals —
   neither is safe for autopilot.
2. **No hooks.** The repo has **no application/agent-level Pre/PostToolUse hooks** (its only "hooks" are
   the CI workflow-guard scripts). There is no place to enforce policy a user cannot bypass.

This is arguably the **third product pillar** (alongside cost-awareness and orchestration): governance is
what lets orchestration run *unattended* without being reckless. Reference models (Claude Code's
allow/deny/ask rules + PreToolUse hooks + an auto-mode risk classifier) inform the *shape*, but the goal
is **our own, in-app feature**: **auto-approve read-only ops, hard-deny destructive patterns, and force a
mandatory human-in-the-loop gate for the risky-but-legitimate** — enforced in the bridge, *not just
config* (deny-rules-alone have documented bypass bugs; combine them with a hook).

**Hooks are a first-class forge primitive, managed in the app.** The headline deliverable is that a
my-orchestra user can **add, edit, enable/disable, and remove their own Pre/PostToolUse hooks from the
UI** — a new `ctxforge.Hook` entity persisted in `forge.json` and CRUD-managed exactly like skills,
instructions, agents, MCP servers, workflows, and snippets (same form/list/preflight patterns). The
safe-by-default governance policy (G0–G2) is then expressible as **built-in hooks/rules** on that same
mechanism — the user's hooks and the shipped defaults run through one evaluator.

### The seam

`permissionHandler` already receives the tool `req.Kind()` (read/write/shell/…) and, for shell, the
command string (`describePermission`). A policy layer slots in **before the gate emits**: evaluate the
policy → **allow** (auto-approve), **deny** (`PermissionDecisionReject` with a reason back to the agent),
or **ask** (the existing HITL gate). This generalizes `AutoApproveTools` from a name list to a ruleset.

## Children

- [x] **G0 · Policy model + seam** (M; ADR). A forge-backed policy of `allow / deny / ask` rules matched
      on tool **kind** + **bash patterns**, evaluated inside `permissionHandler` before the gate. Deny
      wins over allow. Pure, table-tested matcher; the bridge consults it. — **shipped in V25**
      ([0053](0053-hooks-foundation-forge-entity-bridge-evaluator.md), ADR-0029).
- [x] **G1 · Default safe policy (auto-approve reads)** (M). Ship a safe-by-default policy: read-only
      tools (file read, search, navigation, plan transitions) auto-approved; writes/exec → the gate.
      The default build is safe out of the box. — **shipped in V25**
      ([0053](0053-hooks-foundation-forge-entity-bridge-evaluator.md), `ctxforge.DefaultHooks`).
- [x] **G2 · Dangerous-action deny + mandatory HITL** (M/L; ADR). Built-in deny/gate for destructive
      patterns — `rm -rf` on `$HOME`/root, `curl|sh` / pipe-a-download-into-an-editor-or-shell, writes
      outside the workspace, `sudo`, obvious exfiltration — hard-denied or forced through a **mandatory**
      gate **even in auto mode**, enforced in the bridge (unbypassable). — **shipped in V26**
      ([0054](0054-dangerous-action-deny-mandatory-hitl.md), ADR-0030): `ctxforge.DangerousHooks`
      (mandatory ruleset) + the `OutsideWorkspace` fence + the always-on policy-aware
      `permissionHandler` that enforces the mandatory subset even with `AutoApproveTools`.
- [x] **G3 · Hooks as a first-class forge entity** (L; ADR) — *the headline feature*. A new
      `ctxforge.Hook` `{id, event (pre/post-tool-use), match (tool kind / pattern), action
      (command or built-in allow|deny|ask), enabled}`, compiled into the session and fired by the bridge:
      PreToolUse returns `allow|deny|ask`; PostToolUse observes/logs. Persisted in `forge.json` like every
      other forge type. Reuse `${VAR}` + preflight; **hook command output is untrusted** — sanitize. —
      **mechanism shipped in V25** ([0053](0053-hooks-foundation-forge-entity-bridge-evaluator.md),
      ADR-0029): the entity + persistence + compile + built-in allow/deny/ask. **External command-ref
      execution** (PostToolUse running a user command, `${VAR}` + preflight, untrusted output) is
      deferred to a later child.
- [x] **G4 · Hook/policy editor UI + mode binding** (M; ADR). Full **CRUD in the app** — add / edit /
      enable-disable / remove hooks from a Hooks page, exactly like the skills/MCP/workflow forms (list +
      form + preflight). Bind a hook set to **agent modes** (auto mode → strict defaults on; ask mode →
      fully interactive); surface *why* a call was auto-approved/denied in the timeline. — **shipped in V27**
      ([0055](0055-hooks-ui-mode-binding-timeline-why.md), ADR-0031): the `/hooks…` page (built-ins
      read-only + user CRUD + preflight), `Hook.Modes` + `EffectiveAutoApprove` mode binding, and the
      `EvToolDecision` timeline "why" annotation.

## Acceptance (epic)

- [ ] Read-only tools are auto-approved by default; writes/exec are gated; the default build is safe.
- [ ] A documented set of destructive patterns is hard-denied or force a mandatory gate **even in auto
      mode**, enforced in the bridge (not bypassable by config alone), and **table-tested**.
- [x] Hooks are a **first-class forge entity** persisted in `forge.json`, with full **add/edit/
      enable-disable/remove CRUD in the app** (the Hooks page, G4); Pre/PostToolUse hooks fire from
      the bridge, return allow/deny/ask. *(Treating hook command output as untrusted is the last
      child — external command-ref execution.)*
- [x] Hooks/policy are bound to agent modes; every auto-approve/deny decision is explainable in the UI.
- [ ] Each child: failing test first, ADR where it sets policy semantics, `make lint && make test`
      (floor 65%) + `make e2e` green, born in its PR, SemVer minor.

## Sequencing

G0 → G1 → G2 (the safe-autopilot MVP) → **(dedicated quality audit)** → G3 → G4. Co-lead with epic 0050
(billing fidelity); both rank above epic 0051 (auth).

## Notes

Reference model + sources (Claude Code allow/deny/ask, PreToolUse hooks, auto-mode classifier; the
deny-rules-alone caveat) are in NEXT_FEATURES.md "Roadmap v10". Builds on the existing perm bridge
(ADR-0017 per-lane permission surface, ADR-0012 diff-review lane) and `AutoApproveTools`.
