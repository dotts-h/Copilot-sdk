# 0030. Dangerous-action deny + mandatory HITL: a built-in ruleset unbypassable by config, enforced on the auto path

- Status: accepted
- Date: 2026-06-08
- Deciders: Horia
- Related: the **second build child** (G2) of the safe-autopilot governance epic
  ([0052](../issues/0052-epic-safe-autopilot-governance.md), roadmap v10) —
  issue [0054](../issues/0054-dangerous-action-deny-mandatory-hitl.md). Builds directly on
  [ADR-0029](0029-hooks-forge-entity-bridge-enforced-allow-deny-ask-safe-read-defaults.md)
  (the `ctxforge.Hook` entity, the pure `Evaluate`, the bridge-enforced allow/deny/ask, the
  safe-read defaults, built-ins-as-hooks) and the `AutoApproveTools` switch it generalizes.
  Reuses the `${VAR}` reference shape from [ADR-0020](0020-mcp-secrets-via-env-var-reference-indirection.md).
  Touches `internal/ctxforge` (the `Hook.Mandatory` flag, the `HookMatch.OutsideWorkspace`
  fence dimension, the workspace arg + `Decision.Mandatory` on `Evaluate`, `DangerousHooks`,
  `Forge.Compile`), `internal/copilot` (the `SessionSpec.Workspace` field, the per-session
  `sessionPolicy`, the always-on policy-aware `permissionHandler`), `internal/web` +
  `internal/bootstrap` (`SeamSpec`/`Hub` workspace threading), and the doc records (CONTEXT
  **mandatory gate** / **workspace fence** terms, CONTRACTS §1/§4, this ADR). **No new HTTP
  route, no UI** — the Hooks management UI is the next child (G4).

## Context

ADR-0029 shipped the hooks foundation: a forge-backed `Hook`, a pure `Evaluate` with
**deny > ask > allow** precedence and an **ask** default, and a safe-read default that
auto-approves reads. Two gaps remained for *safe autopilot*:

1. **No dangerous-action policy.** Nothing hard-denies the clearly-destructive (`rm -rf /`,
   `curl … | sh`) or forces a human past the risky-but-legitimate (`sudo`, a write that
   escapes the project tree). A user on autopilot has no floor.
2. **`AutoApproveTools` bypasses the policy entirely.** It was wired as the SDK's blanket
   `PermissionHandler.ApproveAll`, which **never calls `Evaluate`**. So the moment a user
   enables auto-approve — exactly the unattended case that most needs a floor — every hook,
   including any deny, is skipped. Deny-rules that live only in config and can be flipped off
   have documented bypass bugs; the enforcement has to sit in the bridge and be unbypassable.

This ADR settles: **what the dangerous ruleset is**, **how the workspace fence is expressed**
(the glob matcher can't do path containment), and **how the mandatory subset is made
unbypassable by config — including on the auto-approve path**.

## Considered options

- **The workspace fence — how to express "a write outside the project tree".**
  - **A new match dimension (`HookMatch.OutsideWorkspace`) + a workspace root threaded into
    `Evaluate` (chosen).** The fence is a real built-in hook
    (`{write, OutsideWorkspace} → mandatory ask`) flowing through the **same** `Evaluate` as
    every other rule, preserving the one-evaluator property. The evaluator stays **pure**: it
    takes the workspace root as an argument and normalizes the write's target path against it
    with `filepath.Rel` (an absolute target is compared directly; a relative one is resolved
    against the root; `..`-escaping or a different volume counts as outside; an empty root
    makes the fence inert). The root is a **runtime fact**, not a forge fact, so it is threaded
    at the seam via `copilot.SessionSpec.Workspace` (mirroring how `Hooks` flow) and recorded
    per session — `ctxforge` never learns the cwd.
  - *A dedicated predicate checked directly in `permissionHandler`, outside the hook model.*
    Rejected: it would be a second policy path the user can't see, can't reorder against their
    own hooks, and can't have surfaced in the future timeline "why" — defeating ADR-0029's
    single-evaluator invariant.

- **The mandatory/unbypassable tier — how it survives `AutoApproveTools`.**
  - **A `Hook.Mandatory` flag + auto-path enforcement, keeping one `Evaluate` (chosen).** The
    built-in `DangerousHooks` are marked `Mandatory`; the safe-read defaults and user hooks are
    not. `Evaluate`'s precedence is unchanged (deny > ask > allow), which **already** stops a
    user `allow` from weakening a built-in deny or ask (deny/ask both beat allow). The flag adds
    nothing to that precedence — its sole job is the **auto path**. `Decision.Mandatory` reports
    whether a mandatory hook drove the winning action, and the bridge stops wiring the SDK's
    `ApproveAll`: `permissionHandler` **always runs**, and under `AutoApproveTools` it
    blanket-approves only the **non-mandatory** remainder — a mandatory deny still rejects and a
    mandatory ask still gates. A user `deny` (more restrictive) still wins over a mandatory ask,
    because we never want the policy to fight the user in the *safe* direction.
  - *Make the mandatory tier outrank a user `deny` too.* Rejected: a user can only make a rule
    **more** restrictive than a mandatory ask (deny), which is always safe to honor; overriding
    it would needlessly fight the user.
  - *Leave `ApproveAll` wired and special-case the dangerous set before it.* Rejected: that is
    two policy paths again (the dangerous check, then ApproveAll), re-introducing the divergence
    the single evaluator exists to prevent. One handler, one evaluator, a per-decision mandatory
    bit is the DRY shape.

- **The ruleset itself — deny vs. force-gate per pattern.** **Unambiguously** destructive / RCE /
  exfiltration patterns are **hard-denied** (`rm -rf` of `/`·`~`·`$HOME`; `curl|sh` / `wget|sh`
  incl. `bash` and `|nano`/`|vim`, where the interpreter token must follow the pipe **directly** so
  a benign later `sh` substring like `curl … | grep ssh` is **not** caught; pipe-to-`nc`; POSTing an
  SSH private key `id_rsa` via curl/wget). **Heuristic** patterns that could also hit a benign
  command are **force-gated** as a *mandatory ask*, not denied — a false positive asks a human
  instead of an unoverridable block: a curl referencing a credential **store** (`.ssh/`,
  `.aws/credentials`, `.netrc` — substrings that can appear in a URL path), `sudo`, and an
  out-of-workspace write. Patterns are **unanchored** (match anywhere, so a leading token can't
  dodge them) and deliberately conservative: this is defense-in-depth at the permission gate,
  **not** a hardened sandbox. Accepted, documented matcher limits: a recursive force-delete of any
  **absolute** path under `/` or `~`/`$HOME` is denied (relative cleanup like `rm -rf ./build` is
  left to the gate — the required near-miss); an interpreter sharing a prefix with the pipe target
  (`curl … | sha256sum`) is a rare residual over-match; and non-pipe netcat (`nc host < file`) plus
  exotic obfuscation (process substitution, unusual spacing) are out of scope for the string
  matcher. The workspace fence also treats a `~`/`$VAR` write target as outside (fail-safe — such a
  path is never workspace-relative).

## Decision

Add `Hook.Mandatory` (unbypassable-by-config) and `HookMatch.OutsideWorkspace` (the path-aware
fence dimension). `Evaluate` gains a `workspace` argument (normalizes the fence via
`filepath.Rel`, empty = inert) and a `Decision.Mandatory` bit; precedence is otherwise
unchanged. `DangerousHooks()` is the built-in **mandatory** ruleset — hard-deny the destructive,
force-ask the risky-but-legitimate — and `Forge.Compile` folds it into every session's policy
alongside the safe-read defaults, through the one evaluator. The seam threads the workspace root
via `copilot.SessionSpec.Workspace` (set to the process cwd by `bootstrap`, carried on every
`SeamSpec` path) and records it per session. `permissionHandler` is now the **only** permission
handler (the SDK `ApproveAll` is never wired): it evaluates the full policy, rejects a deny,
gates a mandatory ask, and only blanket-approves the non-mandatory remainder when
`AutoApproveTools` is set — so the dangerous policy is **enforced in the bridge, unbypassable by
config**.

## Consequences

- **Unbypassable floor.** A destructive command is denied and `sudo` / an out-of-workspace write
  is force-gated **even with `AutoApproveTools=true`** — table-tested at the domain layer and at
  the seam. Autopilot now has a floor it cannot turn off.
- **`AutoApproveTools` is narrower (and safer).** It no longer means "approve literally
  everything" — it blanket-approves the **ask/allow remainder**, but a mandatory deny/ask (and,
  as a natural consequence of always running the handler, a **user** deny) is honored. This is a
  deliberate behavior change from ADR-0029's "coarse override above the policy": the override now
  sits **above the non-mandatory policy and below every deny**. A read is still auto-approved with
  no gate; a benign in-workspace write under auto-approve still flows with no gate.
- **`ctxforge` stays pure.** The workspace root is a runtime argument to `Evaluate`, threaded at
  the seam like `Hooks` — the forge never learns the cwd, and the evaluator remains a pure
  function (deterministic, table-tested).
- **Additive + backward-readable.** `Hook.Mandatory` (`mandatory,omitempty`) and
  `HookMatch.OutsideWorkspace` (`outsideWorkspace,omitempty`) are omitempty; a pre-G2
  `forge.json` loads unchanged. `copilot.SessionSpec.Workspace` defaults to empty (fence inert).
- **`Evaluate`'s signature changed** (a `workspace` argument; `Decision` gained `Mandatory`) —
  an internal seam, updated at its three call sites (the handler + the domain tests) and recorded
  in CONTRACTS §4.
- **Next children (tracked on the epic):** G4 the Hooks management UI (list + form + preflight
  CRUD like the MCP/workflow pages, mode binding, timeline "why" surfacing); and external
  command-ref hook execution (PostToolUse running a user command with `${VAR}` + preflight,
  untrusted output) — a later child.
