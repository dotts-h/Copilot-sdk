# 0029. Hooks: a forge-backed Pre/PostToolUse entity, bridge-enforced allow/deny/ask, deny-wins, safe-read defaults

- Status: accepted
- Date: 2026-06-08
- Deciders: Horia
- Related: the **first build child** of the safe-autopilot governance epic
  ([0052](../issues/0052-epic-safe-autopilot-governance.md), roadmap v10) — issue
  [0053](../issues/0053-hooks-foundation-forge-entity-bridge-evaluator.md) (V25 = G0 + the
  G3 mechanism + G1 defaults). Builds on the sync↔async permission bridge
  (`internal/copilot/handlers.go`, [ADR-0017](0017-per-lane-tool-and-permission-surface-for-parallel-workflow-lanes.md)
  per-lane permission surface, [ADR-0012](0012-diff-review-lane-for-file-write-permissions.md) diff-review
  lane), the existing flat `AutoApproveTools` switch, and the forge-entity CRUD/persist pattern shared
  by skills/instructions/agents/MCP/workflows/snippets ([ADR-0003](0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md),
  [ADR-0010](0010-mcp-server-management-page-curated-defaults-disabled-with-preflight.md),
  [ADR-0013](0013-multi-agent-workflow-run-handoff-surface.md),
  [ADR-0015](0015-prompt-snippet-library-forge-backed-composer-insertion.md)). Reuses the `${VAR}`
  reference shape from [ADR-0020](0020-mcp-secrets-via-env-var-reference-indirection.md).
  Touches `internal/ctxforge` (the new `Hook` entity + `Evaluate` + `DefaultHooks`; `Forge.Compile`),
  `internal/copilot` (the `SessionSpec.Hooks` field, the per-session hook map, the policy-aware
  `permissionHandler`), `internal/web` (`SeamSpec`/`applyAgentSpec` threading), and the doc records
  (CONTEXT **hook** term, CONTRACTS §1/§4, this ADR). **No new HTTP route, no UI** — the management UI
  is a later child (G4), so the schema is additive and forge-only here.

## Context

The product can run agents on **autopilot** and orchestrate multi-lane workflows, but it has **no
governance layer** that makes that safe. Two concrete gaps in the code before this change:

1. **The permission gate is all-or-nothing.** `permissionHandler` always emits an interactive
   `EvPermission` and blocks for a human decision; the only automation is a **flat `AutoApproveTools`
   switch** that approves *everything*. A user either click-approves every call or disables approvals —
   neither is safe for unattended autopilot.
2. **No hooks.** There is no application/agent-level place to enforce a policy a user cannot bypass.

Governance is arguably the **third product pillar** (alongside cost-awareness and orchestration): it is
what lets orchestration run unattended without being reckless. The headline goal of the epic is that a
user can **add/edit/enable/remove their own Pre/PostToolUse hooks** — so hooks must be a **first-class
forge primitive**, not a config flag. This first child makes hooks a **real, enforced** forge entity
*before* the management UI exists: the domain entity + the bridge evaluator + the safe-by-default
policy. (Out of scope here, noted as the next children: G2 dangerous-action deny ruleset + mandatory
HITL; G4 the Hooks management UI; external command-ref hook execution.)

The decisions this ADR settles: **what a hook is** (its schema and where it lives), **how a decision is
reached** (the evaluator's match + precedence semantics), **where it is enforced** (the seam), and **how
the default build is safe out of the box**.

## Considered options

- **Where the policy lives — config flag vs. forge entity.**
  - **A first-class `ctxforge.Hook` forge entity (chosen).** Persisted in `forge.json` under an
    additive `hooks` key, validated and CRUD-managed exactly like every other forge type (Add/Update/
    Toggle/Remove through the shared `mutate` rollback discipline). This is the epic's headline
    requirement — hooks are user-managed context, diffable and shareable with the rest of the forge —
    and it lets the safe-default policy be expressed *as built-in hooks on the same mechanism*.
  - *A widened `AutoApproveTools` allowlist / a `config` ruleset.* Rejected: it is not user-curated
    context, it would need a parallel CRUD/persist story, and deny-rules-in-config-alone have documented
    bypass bugs — the enforcement must sit in the bridge, and the rules belong with the forge.

- **How a decision is reached — the evaluator.**
  - **A pure `Evaluate(hooks, event, toolKind, command) Decision` in `ctxforge` (chosen).** A hook
    participates when it is `Enabled`, its `Event` matches, and its `Match` (tool kind + optional
    command pattern) applies. **Action precedence is most-restrictive-wins: `deny > ask > allow`**, and
    **with no participating hook the default is `ask`** (fall through to the interactive gate) — so the
    policy is never silently permissive. Precedence is order-independent (the function scans for the
    first hook of each action class), so hook ordering only chooses *which same-action reason* is
    surfaced, keeping the result deterministic. The matcher treats a `*`/`?` pattern as a glob over the
    **whole** command and a metacharacter-free pattern as a substring — table-tested exhaustively
    (allow/deny/ask precedence, pattern match, disabled-hook-ignored, event-mismatch, empty-set
    default).
  - *Deny-only, or allow-only with an implicit-deny default.* Rejected: the three-way allow/deny/ask
    maps exactly onto the bridge's three permission outcomes and lets the safe default (auto-approve
    reads, gate the rest) and the future dangerous-deny ruleset (G2) coexist in one evaluator.

- **Where it is enforced — the seam.**
  - **Inside `permissionHandler`, before the gate emits (chosen).** The compiled hook set is threaded
    through `copilot.SessionSpec.Hooks` (mirroring how `MCPServers` flows via `SeamSpec`) and recorded
    per `SessionID` in the `SDKClient`. When the SDK's synchronous permission callback fires, the
    handler consults `Evaluate`: **allow** → `PermissionDecisionApproveOnce` (no `EvPermission`
    emitted), **deny** → `PermissionDecisionReject{Feedback: reason}` (the reason is fed back to the
    agent), **ask** → the existing emit-and-block gate. This generalizes `AutoApproveTools` from an
    all-or-nothing switch to a per-tool ruleset and is **unbypassable by config alone** — the decision
    is made in the bridge. `AutoApproveTools=true` is preserved as a blanket approve-all escape hatch
    above the policy.
  - *Evaluate in the web layer and pass a pre-baked decision.* Rejected: the SDK callback fires
    synchronously inside the seam with the tool kind + command already in hand; that is the one place a
    decision must be returned inline. Pushing it up would re-introduce a parallel async round-trip for
    what is a pure function call.
  - **Note on package coupling.** This makes `internal/copilot` import `internal/ctxforge` for the
    `Hook` type and `Evaluate`. `ctxforge` is the **pure, dependency-free domain** package (no SDK), so
    the dependency is one-directional onto pure domain and does **not** violate seam purity (no SDK type
    leaks into `ctxforge`; the rule is that no SDK import crosses *out* of the seam). Keeping a single
    `Hook` type and a single evaluator (rather than mirroring a second copy in `copilot`, the way
    `MCPServer` is mirrored) is the DRY choice the epic mandates — *built-ins and user hooks run through
    the same `Evaluate`*.

- **How the default build is safe — built-ins as hooks.**
  - **`DefaultHooks()` returns a built-in safe-read allow hook, prepended by `Compile` (chosen).**
    Read-only tool kinds are auto-approved; writes/shell/MCP fall through to the gate. The built-ins run
    through the **same** `Evaluate` as user hooks, so a user `deny` on reads still wins (deny > allow)
    and a user `ask` downgrades a read to the gate. The default build is safe out of the box with an
    empty user forge.
  - *Hard-code the read auto-approve in the handler.* Rejected: it would be a second, divergent policy
    path the user could neither see nor override, defeating the "one evaluator" property.

## Decision

Add a first-class `ctxforge.Hook` forge entity `{id, event (pre-tool-use|post-tool-use), match (toolKind
+ optional command pattern), action (allow|deny|ask), reason, enabled}`, persisted in `forge.json` under
an additive `hooks` key and CRUD-managed like every other forge type. A pure
`Evaluate(hooks, event, toolKind, command) Decision` resolves the policy with **deny > ask > allow**
precedence and an **ask** default. `Forge.Compile` prepends the built-in `DefaultHooks()` (auto-approve
reads) to the forge's enabled hooks into `SessionSpec.Hooks`, threaded through the seam's
`copilot.SessionSpec.Hooks` and recorded per session. `permissionHandler` consults `Evaluate` before the
interactive gate: allow auto-approves, deny rejects with the reason, ask gates. Built-ins and user hooks
share one evaluator; the policy is enforced in the bridge.

## Consequences

- **Safe by default.** A fresh install auto-approves read-only tools and gates writes/shell — autopilot
  no longer means "approve everything or nothing". The behavior is table-tested at the domain layer and
  at the seam (a read is approved with **no** gate emitted; a denied pattern rejects with its reason; an
  ask falls through to the gate).
- **Additive + backward-readable.** `hooks` is `omitempty`; a pre-V25 `forge.json` loads unchanged and
  an older reader ignores the key. The seam's `SessionSpec` gains a `Hooks` field (default nil →
  built-ins still apply via `Compile`).
- **`AutoApproveTools` is now a coarse override** layered above the policy, not the only automation. A
  later child may fold it into a built-in allow-all hook.
- **Coupling:** `internal/copilot` now depends on `internal/ctxforge` (pure domain). New SDK behavior
  still lives only in the seam; `ctxforge` stays SDK-free.
- **Next children (tracked on the epic):** G2 dangerous-action deny ruleset (`rm -rf $HOME`, `curl|sh`,
  pipe-download-into-shell, `sudo`, writes outside the workspace) + mandatory HITL even in auto mode;
  G4 the Hooks management UI (list + form + preflight CRUD, like the MCP/workflow pages) with mode
  binding and timeline "why" surfacing; and external command-ref hook execution (PostToolUse running a
  user command with `${VAR}` + preflight, treating output as untrusted).
