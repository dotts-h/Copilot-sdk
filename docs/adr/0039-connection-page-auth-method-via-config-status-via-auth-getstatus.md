# 0039. Connection page — auth method chosen via config (applies at next launch), live credential via `auth.getStatus`, no secret at rest

- Status: accepted
- Date: 2026-06-10
- Deciders: Horia
- Related: [ADR-0020](0020-mcp-secrets-via-env-var-reference-indirection.md) (the `${VAR}`
  indirection this reuses), epic [0051](../issues/0051-epic-auth-and-connection.md),
  issue [0067](../issues/0067-auth-spike-sdkclient-auth-today.md) (the A0 spike whose findings
  this decision consumes), issue [0068](../issues/0068-connection-page-auth-method-surface.md)
  (the A1 surface this shapes), CONTRACTS §1 (the seam this extends)

## Context

The app reaches Copilot through whatever credential the CLI resolves, invisibly; an auth
failure is indistinguishable from "runtime missing" (both fall back to the offline mock).
Copilot supports four auth methods with a defined precedence
(`COPILOT_GITHUB_TOKEN → GITHUB_TOKEN → GH_TOKEN → gh CLI → device-flow login`), and the
headless fallback some tools use stores a **plaintext** token at `~/.copilot/config.json` —
which this project refuses (ADR-0020: no secret at rest).

The A0 spike (issue 0067) established the facts that bound the design:

1. The SDK's only public **read** surface is `Client.GetAuthStatus` (`auth.getStatus` RPC →
   `{IsAuthenticated, AuthType, Host, Login, StatusMessage}`).
2. There is **no public write surface**: no login/logout RPC (`session.auth.setCredentials`
   is generated-RPC-only), and the CLI runs `--headless`, so **device flow cannot be
   initiated in-app**. Changing the method means re-dialing the client with different
   `Options`.
3. The spawned CLI inherits the full parent environment, so the env-var precedence applies
   transparently; an explicit `GitHubToken` (injected as `COPILOT_SDK_AUTH_TOKEN`, never
   written) outranks everything.
4. The app already holds the no-secret-at-rest token path: `config.GitHubTokenEnv` stores an
   env-var **name**, resolved at dial time.

A live in-place re-dial was considered and rejected: the Hub's single `pump` goroutine ranges
over `client.Events()`, which `SDKClient.Close` deliberately never closes (to avoid a
send-on-closed-channel panic from in-flight callbacks), and every cookie-keyed `Server` holds
a copy of the client pointer — a runtime swap would leak the pump, strand per-`Server`
copies, and invalidate every live session. The config knobs that affect dialing (e.g. the
OTLP endpoint) already follow an "applies on next launch" discipline.

## Decision

A **Connection page** (nav group Config) built on three pieces:

1. **See — a new read-only seam method.** `copilot.Client` gains
   `AuthStatus(ctx) (AuthStatus, error)` — the normalized
   `{Authenticated, Method, Login, Host, Detail}` mapping of the SDK's `GetAuthStatus`.
   `MockClient` returns a settable canned status (offline default), keeping UI tests on the
   mock per the seam-purity rule. `Method` (the SDK's `AuthType`) is **opaque display text**,
   not an enum — its vocabulary is unverified (spike note). The page renders the live status
   plus the static precedence ladder with the configured rung highlighted.
2. **Choose — config-persisted, applied at next launch.** `config.AuthMethod` ∈ `""` (auto:
   the CLI's own chain — device-flow login, env vars, `gh`), `"token"` (explicit token from
   the env var named by `GitHubTokenEnv`), `"gh"` (force-reuse the `gh` CLI credential).
   Selection persists through `editConfig` (validate + rollback) and **re-dials on the next
   launch** — no live client swap (see Context). At dial, `copilot.ResolveAuthMethod`
   dispatches: `"gh"` resolves the token by running `gh auth token` (bounded, never
   persisted, in-memory only; on failure it falls back to auto and the failure is visible on
   the page's preflight); `"token"`/auto reuse the existing `ResolveAuth`.
3. **No secret at rest — paste lands in the process env only.** The page's "paste a token"
   flow stores the value via a `setEnv` seam (default `os.Setenv`) under the user-chosen
   `${VAR}` name and persists **only the name** (`GitHubTokenEnv`, the ADR-0020 shape). The
   secret is never written to disk, never echoed back into HTML (the page shows only
   set/unset), and evaporates on process exit — persisting it across restarts is the user's
   shell profile's job, and the page says so.

Device flow stays out-of-app by SDK constraint (finding 2): the page links the user to run
`copilot` interactively and re-check status — the dead-end is surfaced honestly rather than
worked around with a bespoke OAuth implementation.

## Consequences

- The stable `copilot.Client` seam grows one read-only method (this ADR is the §1 contract
  change record). No SDK type crosses the seam; consumers keep zero SDK imports.
- Auth state becomes inspectable: the "auth failure vs. runtime missing" ambiguity (spike
  finding) now has a surface — the mock fallback's Connection page shows an unauthenticated
  offline status instead of nothing.
- A method change takes effect on the next launch, like the OTLP endpoint; the page labels
  this. If the SDK later ships a public credential-set RPC, a live-apply child can supersede
  this trade-off with a new ADR.
- A pasted token's lifetime is the process: honest, but means re-pasting (or a shell-profile
  export) after a restart. The masked `${VAR}` indirection stays the single secret pattern
  app-wide (ADR-0020).
- `"gh"` mode adds one bounded exec (`gh auth token`) at dial time, behind a seam injectable
  in tests; `gh` absent degrades to auto with a visible preflight warning, never an error
  loop.
