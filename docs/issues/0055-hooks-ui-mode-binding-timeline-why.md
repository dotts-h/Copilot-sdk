---
id: 0055
title: "Hooks management UI + mode binding + timeline why — in-app CRUD over governance hooks (roadmap v10, V27 = G4)"
status: open
severity: high
group: 0052
github:
links:
  adr: [0031]
  prs: []
  issues: [0052]
---

## Summary

The fourth **build** child of the safe-autopilot governance epic
[0052](0052-epic-safe-autopilot-governance.md) (roadmap v10), following the hooks foundation
[0053](0053-hooks-foundation-forge-entity-bridge-evaluator.md) (V25) and the dangerous-action floor
[0054](0054-dangerous-action-deny-mandatory-hitl.md) (V26). It is the **headline surface**: a
my-orchestra user can add/edit/enable-disable/remove their own Pre/PostToolUse hooks **from the
app**, see the shipped built-in policy (read-only), bind hooks to **agent modes**, and read *why*
each call was auto-approved / denied / gated in the chat timeline.

**V27 lands:**

- **The Hooks page** (`/hooks…`, **Build** nav group) — full CRUD mirroring the MCP/workflow pages:
  a list distinguishing the **read-only** built-ins (`DefaultHooks` + the mandatory `DangerousHooks`,
  the mandatory ones badged *unbypassable*) from **user** hooks (add/edit/toggle/delete), a form
  (event, match {toolKind, pattern, outsideWorkspace}, action, reason, modes, enabled) with
  validated builders + rollback-on-invalid, and a **preflight** that calls the same pure matcher
  (`ctxforge.MatchPattern`/`PatternIsGlob`) against a sample command so a rule can be seen firing
  before it's saved. All values flow through `html/template` (ADR-0001).
- **Mode binding** (ADR-0031). A per-hook `Hook.Modes` set (empty = every mode) threaded into the
  pure `Evaluate` as the `mode` argument: a user hook can be scoped to autopilot or interactive,
  while the mandatory ruleset (unscoped) holds in **every** mode (the G2 floor never weakens). Plus
  `EffectiveAutoApprove(mode, configDefault)` — the bridge's mode-bound auto-approve baseline
  (autopilot on / interactive off / else the `AutoApproveTools` config) — so selecting autopilot
  *actually* runs the non-mandatory remainder unattended and interactive forces *more* gates. The
  active mode is recorded on the per-session policy and updated at `Send`, like the workspace root.
- **Timeline "why"** (ADR-0031). A new normalized `copilot.EvToolDecision` event
  (`ToolDecision{Kind, HookID, Reason, Detail}`) emitted by `permissionHandler` for a **deny**
  (which has no tool card — the tool never runs) and a **user** allow ("auto-approved by *hook*"),
  reduced into a compact `convo.RoleDecision` annotation. A gated **ask** is not a `ToolDecision` —
  it surfaces on the existing `EvPermission` form, now carrying the hook `Reason`. The reason is the
  pure `ctxforge.Decision`'s — no SDK type crosses into `ctxforge`. The annotation is **not a gate**.

It takes **ADR-0031** for the decisions (the mode-binding data model; the timeline-why mechanism and
why it's not a gate). `Evaluate` gains a `mode` argument and `Decision` a `HookID`; the schema is
additive and backward-readable (a pre-V27 `forge.json` loads unchanged — empty `modes` = every mode).

## Acceptance

- [ ] The Hooks page lists the built-in policy **read-only** (built-in + unbypassable badges, no
      CRUD controls) and the user hooks with full add/edit/toggle/delete — seam-tested via
      `MockClient`.
- [ ] The form add/edit/toggle/delete round-trips through the forge with rollback-on-invalid; the
      preflight reports glob-vs-substring and match/no-match for a sample command — seam-tested.
- [ ] Mode binding: the evaluated policy differs by mode as decided, and the **mandatory dangerous
      set is present in every mode** (a dangerous command is still denied in the most permissive
      mode) — table-tested; `EffectiveAutoApprove` and the seam's mode-bound auto-approve baseline
      tested.
- [ ] Timeline "why": the decision + reason reaches the rendered timeline (a deny annotation, a
      user-allow "auto-approved by X", the gate carrying the reason) — web reducer/render seam.
- [ ] Go gates green (`make lint && make test`, coverage floor 65%) and `make e2e` green
      (`hooks.spec.ts`: list + form + preflight + a built-in being read-only); self-review with
      `/code-review`. Born in its PR; ADR-0031 + CONTEXT/CONTRACTS/CODEMAP folded into the branch.

## Out of scope (the next child — the last of the epic)

- **External command-ref hook execution** — a PostToolUse hook running a user **command** with
  `${VAR}` substitution + a preflight, treating the command's output as **untrusted** (sanitize).
  The hook entity already validates the `${VAR}` shape; this child adds the executor.
