# 0020. MCP secrets — env-var-reference indirection, no secret at rest

- Status: accepted
- Date: 2026-06-06
- Deciders: Horia
- Related: unblocks [ADR-0010](0010-mcp-server-management-page-curated-defaults-disabled-with-preflight.md)
  (the MCP page shipped key-free *because* there was no secrets surface); pays down
  TECH_DEBT #10; mirrors the existing `config.GitHubTokenEnv` → `config.GitHubToken()`
  pattern (`internal/config/config.go`); touches `internal/ctxforge` (`MCPServer.Env`),
  `internal/web` (`mcp.go` form, `forms.go` masked field), and the
  forge→seam translation (`web.MCPServerSpecs`); `docs/NEXT_FEATURES.md` item C1,
  [issue 0019](../issues/0019-mcp-secrets-env-editor.md)

> Lead-with-a-decision ADR (ADR-0004): C1 (MCP secrets / Env editor) cannot be built
> until *where secrets live* is decided. This record makes that choice; the build
> (issue 0019) follows it. **No feature code ships with this ADR.**

## Context

The MCP-server management page (ADR-0010) shipped with curated, **key-free** servers
and a form that edits `command`/`args`/`enabled` but **not** `Env` — an existing
`MCPServer.Env` is *preserved* across edits (`mcp.go handleMCPServerUpdate`) but is
only settable by hand-editing `forge.json`. This blocks the highest-value MCP servers
(GitHub, web search), which need an API key — and MCP is how a user grows the agent's
tools, so this is the gate to the product's extensibility story.

The constraint the roadmap set is explicit: **secrets must not live in plaintext in
`forge.json`.** `forge.json` is the forge document — checked into a user's config dir,
read/written on every CRUD edit, and conceptually shareable/exportable. A bearer token
sitting in it in cleartext is the wrong default.

The repo already has a secrets pattern. `config.GitHubTokenEnv` stores the **name** of
an environment variable; `config.GitHubToken()` resolves it with `os.Getenv` at use
time. The secret value never touches `config.json` — only the *reference* to it does.
The question for C1 is whether to follow that precedent or introduce a new secret-at-
rest store.

## Considered options

- **Env-var-reference indirection (chosen).** An `MCPServer.Env` value may be either a
  **literal** (a non-secret like `REGION=eu`) or a **reference** of the form
  `${VAR_NAME}`, resolved from the process environment at session start (in the
  forge→seam translation, `web.MCPServerSpecs`). The MCP form's Env editor marks a row
  as *secret*, and for a secret row **only the `${VAR_NAME}` reference is persisted to
  `forge.json`** — never the secret itself. The secret lives in the user's environment
  (shell, a `.env` the user sources, an OS keychain exporting into the env), exactly as
  `GitHubTokenEnv` already works. Unresolved references are flagged in the page
  preflight (the same surface that already flags an absent `command` on PATH), so a
  missing key is visible *before* a session surprise-fails.
  - *Why:* zero new dependencies, zero secret-at-rest, and it is the pattern the
    codebase already uses for the GitHub token. `telemetry`/`ctxforge`/`config` stay
    dependency-free; the only new IO is reading `os.Getenv`, which is pure-ish and
    trivially testable behind a lookup seam (cf. the `s.lookPath` PATH-preflight seam).

- **A dedicated 0600 `secrets.json` sibling store (rejected as primary).** A new
  append/replace store at `<configDir>/secrets.json`, file-mode `0600`, gitignored,
  atomic temp-file+rename (the SpendStore/RunStore discipline), holding the actual
  secret values keyed by name; `MCPServer.Env` references them by key.
  - *Why not:* it puts secrets **at rest on disk** (even at 0600, even gitignored) for
    a marginal UX gain (in-UI entry of the raw value) in a **single-user localhost**
    tool whose user already has a shell. It adds a new store, its atomic-write +
    validate machinery, a gitignore obligation, and a backup/leak surface — diverging
    from the established env-indirection precedent. Kept as a **documented follow-up**
    if a non-technical, no-shell install flow (cf. desktop installers, TECH_DEBT #5)
    ever makes out-of-band env setup untenable.

- **OS keychain (go-keyring / libsecret / Keychain / DPAPI) (rejected).** Most secure
  at rest, but pulls a CGO/platform-variant dependency into a tool whose pure-Go web
  binary is a hard invariant (CONVENTIONS architecture rules; the desktop CGO split,
  ADR-0006). Disproportionate for a localhost dev tool. The env-reference option can
  *consume* a keychain indirectly (a keychain helper exports into the env) without the
  app linking it.

- **Plaintext `Env` in `forge.json` (rejected — the thing the roadmap forbids).**
  Editing the raw value straight into the forge doc. This is the status quo's only
  manual path and exactly what C1 exists to replace.

## Decision

MCP server secrets use **env-var-reference indirection**; no secret is written to disk
by my-orchestra.

- **`MCPServer.Env` semantics (additive, no schema change).** An `Env` value is a
  literal unless it matches the reference form `${VAR_NAME}` (a `VAR_NAME` of
  `[A-Z_][A-Z0-9_]*`). The existing `env?` map (CONTRACTS §4) is unchanged on disk;
  this ADR only assigns meaning to a value *shape*. A pre-C1 forge with literal `Env`
  values loads and behaves identically (no `${…}` → all literals).
- **Resolution at session start.** `web.MCPServerSpecs` (the single forge→seam
  translation) expands `${VAR_NAME}` via a lookup seam (default `os.Getenv`,
  injectable for tests) when building the `copilot.MCPServer` spec. A reference that
  resolves empty is left **unset** (the server starts without that key) and surfaced as
  unresolved in the page preflight — never silently sent as the literal string
  `${VAR_NAME}`.
- **The form.** The Env editor (a repeatable key/value row plus a *secret* checkbox)
  writes literals inline and, for a secret row, persists only `${VAR_NAME}` — the value
  input is masked and never round-trips to disk. Mirrors the forge-CRUD
  validated-builder + rollback-on-invalid pattern (`forgecrud.go`).
- **Preflight.** `mcpServersPartial` already probes `command` on PATH behind
  `s.lookPath`; it additionally reports any `${VAR_NAME}` in an enabled server's `Env`
  that resolves empty, so a missing key is visible in the UI before a session starts.

## Consequences

- Positive: the gate to **key-requiring MCP servers** (GitHub, web search) opens with
  **no secret at rest** and **no new dependency** — consistent with the existing
  `GitHubTokenEnv` precedent and the pure-Go / dependency-free invariants. Secrets live
  where the user already keeps them (the environment); the forge doc stays shareable.
- Trade-off accepted: the user sets the secret **out-of-band** (export the env var /
  source a `.env`) — there is no in-UI entry of the raw value. For a single-user
  localhost tool this is the right default; the 0600 `secrets.json` store is the
  documented escape hatch if a no-shell install flow ever needs in-UI entry.
- Contract change (lands with the build, issue 0019, **not** this ADR): CONTRACTS §4
  `MCPServer.Env` note flips from "*not edited in the UI*" to "*edited via masked
  rows; secret values are `${VAR}` references resolved at session start, never stored*",
  and a REGRESSIONS entry guards "a secret reference is never persisted as a literal /
  never sent unexpanded." Escaping (ADR-0001) holds: an Env key/value reaches the
  browser through the same `html/template` auto-escaping as the rest of the form.
- Reversible: because only a value *shape* gains meaning, switching later to a
  `secrets.json` store is additive — references could resolve from the store first,
  then the env, without a forge migration.
