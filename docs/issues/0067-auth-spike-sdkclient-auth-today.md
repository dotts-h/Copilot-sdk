---
id: 0067
title: Auth spike — how SDKClient authenticates today and the seam a Connection page needs (roadmap v10, A0)
status: open
severity: medium
group: 0051
depends_on: []
github:
links:
  adr:
  prs: []
  issues: [0051]
  regression:
assets: []
---

## Summary

Spike (A0 of epic [0051](0051-epic-auth-and-connection.md)): document which of the four Copilot
auth methods our `SDKClient` actually resolves today, and what the app can influence — the
decision input for A1's Connection page. **Verdict: the seam exists and A1 is buildable.** The
app already has an explicit-token path (masked env-var indirection, no secret at rest), the
spawned CLI inherits our environment (so the documented env-var precedence applies transparently),
and the SDK exposes a public read-only `auth.getStatus` RPC — the live-credential surface A1
needs. The one closed path: **device flow cannot be initiated in-app** (no login/logout RPC in
the SDK; the CLI runs `--headless`).

## Findings — how auth resolves today

Citations: app = this repo; SDK = `github.com/github/copilot-sdk/go@v1.0.0` module source.

**The app side (what we control today).**

1. `config.Config.GitHubTokenEnv` (internal/config/config.go:29) stores the **name** of an env
   var, never a token; `Config.GitHubToken()` resolves it via `os.Getenv` at dial time
   (config.go:188). Default `""` → use the logged-in `copilot` CLI session. This is already the
   ADR-0020 masked-`${VAR}` shape; a Settings text field edits it (internal/web/settings.go:77).
2. `bootstrap.dialClient` (internal/bootstrap/bootstrap.go:221) → `copilot.ResolveAuth(token)`
   (internal/copilot/sdkclient.go:80): empty token → `UseLoggedInUser=true`; non-empty token →
   explicit override (`UseLoggedInUser` unset). Passed to `sdk.ClientOptions{GitHubToken,
   UseLoggedInUser}` (sdkclient.go:95).
3. On SDK start failure the app silently falls back to `MockClient` (bootstrap.go:227) — today
   an auth failure is indistinguishable from "runtime missing" in the UI.

**The SDK side (what happens underneath).**

4. `sdk.Client.Start` spawns the Copilot CLI as a subprocess (SDK client.go:1607): binary from
   `COPILOT_CLI_PATH` → the SDK-embedded binary (extracted to `~/.cache/copilot-sdk/copilot`) →
   `copilot` on `$PATH`; flags always include `--headless --no-auto-update --stdio`.
5. `GitHubToken` is **not** written anywhere; it is injected into the child env as
   `COPILOT_SDK_AUTH_TOKEN` plus the flag `--auth-token-env COPILOT_SDK_AUTH_TOKEN`
   (client.go:1636, 1677) — token-by-env-reference end to end, consistent with ADR-0020.
6. `UseLoggedInUser=false` adds `--no-auto-login`; `true` (our default) lets the CLI resolve its
   own credential chain.
7. The child process env defaults to the **full parent `os.Environ()`** (client.go:211–213,
   1675). So `COPILOT_GITHUB_TOKEN` / `GITHUB_TOKEN` / `GH_TOKEN` set in our process reach the
   CLI untouched and participate in its documented precedence
   (`COPILOT_GITHUB_TOKEN → GITHUB_TOKEN → GH_TOKEN → gh CLI → device-flow keychain token`).
   SDK-managed vars are appended last and win — i.e. an explicit `GitHubTokenEnv` token
   outranks everything else.
8. **Read seam:** public `Client.GetAuthStatus(ctx)` → JSON-RPC `auth.getStatus` →
   `{IsAuthenticated, AuthType, Host, Login, StatusMessage}` (SDK client.go:1480,
   types.go:1920). This is exactly the "which credential is live" surface A1 needs.
9. **No write seam at runtime:** there is no `signIn`/`signOut`/login RPC on the public Go API.
   `session.auth.setCredentials` exists only in the generated RPC layer (rpc/zrpc.go), not as a
   supported `Session` method — using it raw would bypass the SDK's public surface. Changing
   method ⇒ **restart the SDK client with different `Options`** (the app already constructs it
   in one place, `bootstrap.dialClient`).

## The seam A1 needs (decision input)

- **See:** thread `GetAuthStatus` through the `copilot.Client` interface (normalized
  `AuthStatus` type; `MockClient` returns a canned status) and render it on a Connection page
  with the resolved precedence list (item 7 order, explicit token first).
- **Choose:** a method switch = persist config + **re-dial** the SDK client:
  - *device flow / CLI session* (current default): `GitHubTokenEnv=""` → `UseLoggedInUser=true`.
    In-app device-flow initiation is a **dead-end** (item 9 + `--headless`); the page links the
    user to run `copilot` login interactively, then re-dial + re-check status.
  - *pasted token:* store **only** the env-var name (ADR-0020 reuse); the paste flow writes the
    value to the process env (or instructs the user to export it) — never to disk.
  - *gh reuse:* no token + the CLI's own `gh` fallback (item 7); requires `gh` on PATH;
    verifiable via `AuthType` in `GetAuthStatus`.
- Surfacing auth state also fixes finding 3 (auth failure vs. runtime-missing ambiguity).
- A1's ADR (reserved **0039**): the choose-by-re-dial design, the device-flow dead-end, and the
  no-secret-at-rest invariant.

## Notes

- Probe limitation: this sandbox has neither `copilot` nor `gh` installed, so findings are from
  SDK v1.0.0 source + our tests, not a live credential probe. The `AuthType` string vocabulary
  of `auth.getStatus` is unverified — A1 should treat it as opaque display text, not an enum.
- Sources: epic [0051](0051-epic-auth-and-connection.md), NEXT_FEATURES "Roadmap v10" (A0/A1).
