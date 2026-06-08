---
id: 0054
title: "Dangerous-action deny + mandatory HITL — a built-in unbypassable ruleset enforced on the auto path (roadmap v10, V26 = G2)"
status: closed
severity: high
group: 0052
github:
links:
  adr: [0030]
  prs: [83, 85]
  issues: [0052]
---

## Summary

The second **build** child of the safe-autopilot governance epic
[0052](0052-epic-safe-autopilot-governance.md) (roadmap v10), following the hooks foundation
[0053](0053-hooks-foundation-forge-entity-bridge-evaluator.md) (V25). It adds a built-in
**dangerous-action ruleset** that is **hard-denied or force-gated even in auto mode**, enforced in
the bridge so config alone can't bypass it — the safe-autopilot floor.

Before this change, two gaps remained after V25: there was no dangerous-action policy (nothing
hard-denied `rm -rf /` / `curl|sh` or forced a human past `sudo` / an out-of-workspace write), and
`AutoApproveTools` was wired as the SDK's blanket `ApproveAll`, which **never called `Evaluate`** —
so enabling auto-approve (the unattended case that most needs a floor) skipped every hook.

**V26 lands:**

- **A built-in mandatory ruleset (`ctxforge.DangerousHooks`).** Expressed as `Hook`s through the
  **same** `Evaluate`: hard-`deny` the clearly-destructive (`rm -rf` of `/`·`~`·`$HOME`, `curl|sh`
  / `wget|sh` incl. `bash`, download-into-editor, pipe-to-`nc`, POSTing `id_rsa`/`.ssh`/
  `.aws/credentials`/`.netrc` via curl), force-`ask` the risky-but-legitimate (`sudo`, an
  out-of-workspace write). Each rule documents its intent in `Reason`. Patterns are unanchored
  (a leading token can't dodge them) and conservative — defense-in-depth at the gate, not a
  sandbox; a relative `rm -rf ./build` is the documented near-miss left to the gate.
- **The workspace fence.** A new path-aware match dimension `HookMatch.OutsideWorkspace` the glob
  matcher can't express: a write whose target resolves OUTSIDE the session workspace root is
  force-gated. The root is threaded at the seam (`copilot.SessionSpec.Workspace`, set to the
  process cwd by `bootstrap`) into the pure `Evaluate`, which normalizes the path via
  `filepath.Rel` (empty root = inert) — `ctxforge` never learns the cwd.
- **Unbypassable by config.** `Hook.Mandatory` + `Decision.Mandatory`: the bridge stops wiring the
  SDK `ApproveAll`, so `permissionHandler` **always runs**; under `AutoApproveTools` it
  blanket-approves only the **non-mandatory** remainder, while a mandatory deny rejects and a
  mandatory ask gates even with auto-approve. Precedence is unchanged (deny > ask > allow), so a
  user `deny` (more restrictive) still wins over a mandatory `ask`.

It takes **ADR-0030** for the decisions (the workspace-fence match dimension + threaded root; the
mandatory flag + auto-path enforcement; the ruleset's deny-vs-gate split). The seam's
`copilot.SessionSpec` gains a `Workspace` field; `Evaluate` gains a `workspace` argument and
`Decision` a `Mandatory` bit. **No new HTTP route, no UI** — the management UI is the next child
(G4); the schema is additive and backward-readable (a pre-V26 `forge.json` loads unchanged).

## Acceptance

- [ ] `ctxforge.DangerousHooks()` is the built-in mandatory ruleset: every form (`rm -rf` root/home,
      `curl|sh` / `wget|bash`, download-into-editor, pipe-to-`nc`, secret exfiltration via curl) is
      denied; `sudo` and an out-of-workspace write are force-gated — table-tested exhaustively,
      with near-miss benign forms (`rm -rf ./build`, `curl … -o file`, `| sync`) NOT caught.
- [ ] The workspace fence (`HookMatch.OutsideWorkspace` + the threaded root) is a path-aware
      predicate the evaluator normalizes via `filepath.Rel`, pure and table-tested (in/out/relative/
      sibling-prefix/empty-root).
- [ ] Precedence: a user `allow` does NOT override a built-in mandatory `deny`/`ask`; a user `deny`
      (more restrictive) wins over a mandatory `ask` — table-tested.
- [ ] The mandatory set applies **even with `AutoApproveTools=true`**: a dangerous shell command is
      rejected and an out-of-workspace write is gated at the seam, while a benign in-workspace write
      is still blanket-approved — seam-tested.
- [ ] Go gates green (`make lint && make test`, coverage floor 65%) and `make e2e` green; self-review
      with `/code-review`. Born in its PR; ADR-0030 + CONTEXT/CONTRACTS/CODEMAP folded into the branch.

## Out of scope (the next children)

- **G4** — the Hooks management UI: add/edit/enable-disable/remove CRUD (list + form + preflight)
  like the MCP/workflow pages, mode binding, and timeline "why" surfacing.
- **External command-ref hook execution** — PostToolUse running a user command with `${VAR}` +
  preflight, treating output as untrusted.

## Resolution (shipped)

Shipped in **PR #83** (V26), with the auto-mode wiring follow-up in **PR #85**. `Hook.Mandatory` +
`HookMatch.OutsideWorkspace`, the workspace-aware `Evaluate(…, workspace)` + `Decision.Mandatory`,
and `DangerousHooks()` (hard-deny destructive/RCE/`id_rsa`/netcat; mandatory-gate `sudo`,
out-of-workspace writes, heuristic credential-store refs) all landed, enforced on the auto path —
the SDK `ApproveAll` was dropped so `permissionHandler` always runs and the mandatory subset fires
even with `AutoApproveTools` — ADR-0030. `/code-review` (high) caught and fixed three issues before
merge (the `~`/`$HOME` fence bypass, the `curl*|*sh` over-deny, an over-aggressive credential-store
deny). #85 then threaded the `AutoApproveTools` config toggle through `SeamSpec` so the guarantee is
reachable in production, not just in tests. Next child: G4 the Hooks management UI.
