---
id: 0052
title: "Epic: Safe autopilot — tool-governance policy (allow/deny/ask) + Pre/PostToolUse hooks (roadmap v10)"
status: open
severity: high
group:
github:
links:
  adr: []
  prs: []
  issues: []
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
what lets orchestration run *unattended* without being reckless. Industry practice (Claude Code's
allow/deny/ask rules + PreToolUse hooks + an auto-mode risk classifier) is the reference model:
**auto-approve read-only ops, hard-deny destructive patterns, and force a mandatory human-in-the-loop
gate for the risky-but-legitimate** — enforced in the bridge, *not just config* (deny-rules-alone have
documented bypass bugs; the mitigation is to combine them with a PreToolUse hook).

### The seam

`permissionHandler` already receives the tool `req.Kind()` (read/write/shell/…) and, for shell, the
command string (`describePermission`). A policy layer slots in **before the gate emits**: evaluate the
policy → **allow** (auto-approve), **deny** (`PermissionDecisionReject` with a reason back to the agent),
or **ask** (the existing HITL gate). This generalizes `AutoApproveTools` from a name list to a ruleset.

## Children

- [ ] **G0 · Policy model + seam** (M; ADR). A forge-backed policy of `allow / deny / ask` rules matched
      on tool **kind** + **bash patterns**, evaluated inside `permissionHandler` before the gate. Deny
      wins over allow. Pure, table-tested matcher; the bridge consults it.
- [ ] **G1 · Default safe policy (auto-approve reads)** (M). Ship a safe-by-default policy: read-only
      tools (file read, search, navigation, plan transitions) auto-approved; writes/exec → the gate.
      The default build is safe out of the box.
- [ ] **G2 · Dangerous-action deny + mandatory HITL** (M/L; ADR). Built-in deny/gate for destructive
      patterns — `rm -rf` on `$HOME`/root, `curl|sh` / pipe-a-download-into-an-editor-or-shell, writes
      outside the workspace, `sudo`, obvious exfiltration — hard-denied or forced through a **mandatory**
      gate **even in auto mode**, enforced in the bridge (unbypassable).
- [ ] **G3 · PreToolUse / PostToolUse hooks** (L; ADR). A forge **hook** surface: run a user-defined
      command/check before a tool call returning `allow|deny|ask` (PreToolUse) and after (PostToolUse,
      observe/log). The built-in policy is the first consumer. Reuse `${VAR}` + preflight; **hook output
      is untrusted** — sanitize.
- [ ] **G4 · Policy/hook editor UI + mode binding** (M). Edit rules + hooks in the UI; bind a policy to
      **agent modes** (auto mode → strict default; ask mode → fully interactive); surface *why* a call
      was auto-approved/denied in the timeline.

## Acceptance (epic)

- [ ] Read-only tools are auto-approved by default; writes/exec are gated; the default build is safe.
- [ ] A documented set of destructive patterns is hard-denied or force a mandatory gate **even in auto
      mode**, enforced in the bridge (not bypassable by config alone), and **table-tested**.
- [ ] Pre/PostToolUse hooks exist, return allow/deny/ask, and treat hook output as untrusted.
- [ ] Policy + hooks are editable in the UI and bound to agent modes; decisions are explainable.
- [ ] Each child: failing test first, ADR where it sets policy semantics, `make lint && make test`
      (floor 65%) + `make e2e` green, born in its PR, SemVer minor.

## Sequencing

G0 → G1 → G2 (the safe-autopilot MVP) → **(dedicated quality audit)** → G3 → G4. Co-lead with epic 0050
(billing fidelity); both rank above epic 0051 (auth).

## Notes

Reference model + sources (Claude Code allow/deny/ask, PreToolUse hooks, auto-mode classifier; the
deny-rules-alone caveat) are in NEXT_FEATURES.md "Roadmap v10". Builds on the existing perm bridge
(ADR-0017 per-lane permission surface, ADR-0012 diff-review lane) and `AutoApproveTools`.
