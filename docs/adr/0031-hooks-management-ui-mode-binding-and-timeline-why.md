# 0031. Hooks management UI + mode binding + the timeline "why" annotation

- Status: accepted
- Date: 2026-06-08
- Deciders: Horia
- Related: the **fourth build child** (G4) of the safe-autopilot governance epic
  ([0052](../issues/0052-epic-safe-autopilot-governance.md), roadmap v10) —
  issue [0055](../issues/0055-hooks-ui-mode-binding-timeline-why.md). Builds on
  [ADR-0029](0029-hooks-forge-entity-bridge-enforced-allow-deny-ask-safe-read-defaults.md)
  (the `ctxforge.Hook` entity, the pure `Evaluate`, the bridge-enforced allow/deny/ask, the
  safe-read defaults, built-ins-as-hooks) and
  [ADR-0030](0030-dangerous-action-deny-and-mandatory-hitl-unbypassable-by-config.md) (the
  mandatory dangerous ruleset, the `OutsideWorkspace` fence, the always-on policy-aware
  `permissionHandler`, the workspace arg on `Evaluate`). Mirrors the MCP page CRUD
  ([ADR-0020](0020-mcp-secrets-via-env-var-reference-indirection.md)) and the per-turn agent
  mode wired into `Send`. Touches `internal/ctxforge` (the `Hook.Modes` field, the `mode` arg +
  `Decision.HookID` on `Evaluate`, `EffectiveAutoApprove`, the exported preflight matchers),
  `internal/copilot` (the `EvToolDecision` event + `ToolDecision` payload, `PermissionRequest.Reason`,
  the per-session `mode`, the decision-emitting `permissionHandler`), `internal/convo`
  (`RoleDecision` + `DecisionView`), `internal/web` (the new `/hooks…` page + the timeline
  reducer/render), and the doc records (CONTEXT **mode binding** term, CONTRACTS §1/§3/§4, this
  ADR).

## Context

ADR-0029/0030 shipped the hooks *mechanism* — a forge-backed `Hook`, a pure `Evaluate` with
**deny > ask > allow**, the safe-read defaults, and the mandatory dangerous floor — but with **no
in-app surface**. G4 is the surface plus two policy decisions:

1. **CRUD UI.** A user can add/edit/enable-disable/remove their own hooks, exactly like
   skills/MCP/workflows, and can *see* the shipped built-in policy (read-only) so the active
   governance is legible.
2. **Mode binding.** Today the per-turn agent mode (`autopilot`/`interactive`/`plan`, threaded
   into `Send`) is only an SDK hint — it does **not** affect approvals. "Autopilot" therefore did
   not actually run tools unattended unless the *separate* `AutoApproveTools` config was on, and
   "ask/interactive" could not force *more* gates. The policy needs to bind to the mode.
3. **Timeline "why".** A hook decision is invisible: an `allow` auto-approves silently and a
   `deny` is fed back to the agent with **no tool card** (the tool never runs), so the user can't
   tell why a call ran without a prompt or never happened.

## Considered options

- **Mode-binding data model.**
  - **A per-hook `Modes []string` + a mode-bound auto-approve baseline, threading the active mode
    into `Evaluate` (chosen).** A hook participates only when its `Modes` set is empty (every mode)
    or lists the active mode — the *first-class* reading of "bind a hook set to modes": a user can
    scope a rule to autopilot or interactive. The mandatory dangerous ruleset leaves `Modes`
    **empty**, so the G2 floor holds in every mode (table-tested: a dangerous command is still
    denied in the most permissive mode). Separately, `EffectiveAutoApprove(mode, configDefault)`
    resolves the baseline that the bridge applies to the *non-mandatory* remainder — `autopilot`
    → on (strict defaults on, unattended), `interactive` → off (more gates), any other mode →
    the session's `AutoApproveTools` config. This is what makes the two modes meaningfully
    different without requiring hand-authored hooks; the mandatory subset is enforced **before**
    the baseline, so a `true` here can never bypass a mandatory deny/ask. The active mode is a
    **runtime fact** threaded like the workspace root: recorded on the per-session policy and
    updated at `Send`, so `Evaluate` stays pure (a `mode` argument).
  - *A coarse "strict in auto" switch only* (the mode drives the auto-approve baseline, no
    per-hook field). Rejected as the sole mechanism: it delivers the UX but is not really
    "binding a hook set to modes" — a user can't scope their own rule to a mode. It is kept as the
    *baseline half* of the chosen option.
  - *A mode→hookset compiled per mode in `Forge.Compile`.* Rejected: the policy is compiled once
    at session creation while the mode changes per turn, so a compile-time split fits the seam
    worst and would recompile on every mode flip.

- **Timeline "why" mechanism.**
  - **A new normalized `EvToolDecision` event emitted by `permissionHandler` (chosen).** The SDK's
    `PermissionInvocation` carries **only a SessionID — no tool-call id** — so a decision cannot be
    correlated to a specific tool card, and a hard-deny produces **no card at all**. A dedicated
    event emitted at decision time sidesteps both: it carries the resolved `{Kind, HookID, Reason,
    Detail}` (strings lifted from the pure `ctxforge.Decision` — **no SDK type crosses into
    ctxforge**) and reduces into a compact, muted timeline annotation (`RoleDecision`). To avoid
    flooding the transcript, it is emitted for the cases that have **no other surface and real
    value**: a **deny** (the only way the user learns the call was blocked) and a **user** allow
    ("auto-approved by *hook*" — `Decision.HookID` distinguishes a user hook from a `builtin-*`
    one). The blanket safe-read auto-approve and the autopilot baseline approval stay **silent**
    (they are the expected baseline; the active mode already shows in the statusline). A **gated
    (ask)** decision is *not* an `EvToolDecision` — it already surfaces as the `EvPermission`
    form, which now carries the same `Reason` so the human sees *why* they're asked.
  - *A `Decision`/`Reason` field on the existing tool card (`ToolView`).* Rejected: a hard-deny
    yields no card, and with no tool-call id at permission time the decision can't be attached to
    the right card anyway — it would need a synthetic card, i.e. the event approach with extra
    indirection.

## Decision

Add `Hook.Modes` (mode scoping; empty = every mode) and thread the session's active `mode` into
`Evaluate`; the mandatory ruleset stays unscoped so the floor is mode-independent.
`EffectiveAutoApprove(mode, configDefault)` resolves the auto-approve baseline the bridge applies
to the non-mandatory remainder (autopilot on / interactive off / else config). `Evaluate` also
returns `Decision.HookID` (the winning hook) so the bridge can name it. `permissionHandler` emits
a new normalized `EvToolDecision` for a deny and a user allow, and carries the hook `Reason` onto
the `EvPermission` gate; the reducer renders both as a compact `RoleDecision` annotation / a line
on the gate. A new **Hooks page** (`/hooks…`, in the **Build** nav group) lists the read-only
built-in policy plus full user CRUD (list + form + preflight), mirroring the MCP page — validated
builders with rollback-on-invalid, a mode-binding checkbox set, and a pattern preflight that calls
the **same** matcher (`ctxforge.MatchPattern` / `PatternIsGlob`) against a sample command.

## Consequences

- **Autopilot is real.** Selecting autopilot now actually runs the non-mandatory remainder without
  a prompt (mandatory floor still gates), and interactive forces *more* gates than even the config
  default — table-tested at the domain layer (`EffectiveAutoApprove`, mode scoping) and the seam.
- **Every block is explainable.** A hard-deny — which has no tool card — surfaces as an inline
  "denied: *reason*" annotation, and a user allow as "auto-approved by *hook*". The annotation is
  **not a gate** (the call already proceeded or was blocked), so it doesn't re-introduce the HITL
  pause ADR-0029 removed for reads.
- **`ctxforge` stays pure.** `mode` is a runtime argument to `Evaluate` (like the workspace root);
  the reason/hook-id surfaced in the UI are plain strings on the pure `Decision`; no SDK type
  enters the domain.
- **Additive + backward-readable.** `Hook.Modes` (`modes,omitempty`) is omitempty — a pre-G4
  `forge.json` loads unchanged (empty = every mode, the prior behavior). `EvToolDecision` /
  `PermissionRequest.Reason` are new event-vocabulary additions; older replays without them are
  inert.
- **`Evaluate`'s signature changed** (a `mode` argument; `Decision` gained `HookID`) — an internal
  seam, updated at its call sites (the handler + the domain tests) and recorded in CONTRACTS §4.
- **Scoped to the chat timeline (accepted gaps).** The `EvToolDecision` annotation surfaces in the
  **chat** transcript; a **workflow lane**'s denser surface does not render it (a deny in a lane is
  still enforced — the tool never runs — but isn't annotated there), and the annotation is **live,
  not persisted**, so it is not replayed on session resume (the SDK history has no decision event to
  reconstruct). Both match the original spec ("surface in the chat timeline") and are noted as
  follow-ups, not bugs.
- **Reserved hook ids.** A user hook can't claim the `builtin-` prefix (would spoof the read-only
  built-in distinction and suppress its own "auto-approved by X") nor a `/hooks` route literal
  (`new`/`preflight`, which would shadow the update route and make the hook un-editable) — both
  rejected at the CRUD layer, the domain prefix in `ctxforge.AddHook`/`UpdateHook` and the route
  literals in the web create handler.
- **The Hooks page is the headline G4 surface** and completes the epic's "managed in the app"
  promise. The last child remains: **external command-ref hook execution** (PostToolUse running a
  user command with `${VAR}` + preflight, treating output as untrusted).
