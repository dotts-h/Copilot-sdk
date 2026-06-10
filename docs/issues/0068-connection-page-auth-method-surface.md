---
id: 0068
title: Connection page — see + choose the active auth method: device flow, masked ${VAR} token, gh reuse (roadmap v10, A1)
status: open
severity: medium
group: 0051
depends_on: [0067]
github:
links:
  adr: [0039]
  prs: []
  issues: [0051, 0067]
  regression:
assets: []
---

## Summary

A1 of epic [0051](0051-epic-auth-and-connection.md): a **Connection page** that makes the auth
method visible and choosable, built on the seam the A0 spike
([0067](0067-auth-spike-sdkclient-auth-today.md)) established. Today the credential is whatever
the CLI resolves, invisible to the user, and an auth failure is indistinguishable from "runtime
missing". The page shows the **live credential** (`auth.getStatus`: authenticated?, type, host,
login) plus the resolved precedence chain, and lets the user switch between: (a) the `copilot`
CLI login / device flow (current default), (b) a pasted token stored **only** as a masked
`${VAR}` env-var reference (ADR-0020 reuse — no secret at rest), (c) `gh` CLI reuse.

## Scope / Touches

- `internal/copilot` — thread the SDK's public `GetAuthStatus` through the `copilot.Client`
  seam as a normalized `AuthStatus`; `MockClient` returns a canned status (UI tested via mock,
  per the seam-purity rule). New runtime behavior goes in `SDKClient` only.
- `internal/config` — auth-method selection persists as config (`GitHubTokenEnv` already
  exists); never a raw token. Atomic validate-before-save as usual.
- `internal/web` — the Connection page: live status, precedence list with the active rung
  highlighted, method chooser; switching re-dials the client (A0 finding: no runtime auth
  write seam — choose-by-re-dial).
- **ADR-0039** — choose-by-re-dial design, the in-app device-flow dead-end, and the
  no-secret-at-rest invariant. Folded into the same PR.

## Acceptance

- Active auth method + precedence visible in the UI; live credential from `auth.getStatus`.
- A pasted token is stored **only** as a `${VAR}` reference — no plaintext at rest, masked in
  the UI (ADR-0020 reuse).
- `gh`-reuse and device-flow/CLI-login paths selectable and inspectable.
- Failing test first; `make lint && make test` + `make e2e` green; born in its PR; ADR-0039;
  closes epic 0051 (SemVer minor).

## Notes

Gated on 0067 (done first, same epic). Sources: NEXT_FEATURES "Roadmap v10" (A1).
