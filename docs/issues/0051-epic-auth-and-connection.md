---
id: 0051
title: "Epic: Auth & connection — choose how to reach Copilot (device flow / local token / gh reuse) (roadmap v10)"
status: closed
severity: medium
group:
github:
links:
  adr: [0039]
  prs: [108, 109]
  issues: [0067, 0068]
  regression: []
---

> **Closed 2026-06-10 — both children shipped: A0 spike (issue 0067, PR #108) and the A1
> Connection page (issue 0068, PR #109, ADR-0039). SemVer minor.**

## Charter

Today the app reaches Copilot through whatever credential the underlying CLI/SDK happens to resolve —
there is **no surface to see or choose the auth method**. GitHub Copilot supports four, with a defined
precedence:

1. **OAuth device flow** (default/interactive) — token in the OS keychain (`copilot-cli`). *Our
   current "auto-login."*
2. **Env-var token** — `COPILOT_GITHUB_TOKEN` / `GITHUB_TOKEN` / `GH_TOKEN` (CI / headless / "provide
   a token and save it locally").
3. **Fine-grained PAT** with the **"Copilot Requests"** permission (a specific form of #2).
4. **`gh` CLI fallback** — reuse an authenticated GitHub CLI token (lowest priority).

Precedence: `COPILOT_GITHUB_TOKEN → GITHUB_TOKEN → GH_TOKEN → gh CLI → device flow`. The headless
fallback stores a **plaintext** token at `~/.copilot/config.json` — which we want to avoid; we already
have the masked `${VAR}` indirection (ADR-0020, no secret at rest) to reuse.

## Children

- [x] **A0 · Auth spike** (S) → issue [0067](0067-auth-spike-sdkclient-auth-today.md), PR #108.
      Findings: A1 is buildable — explicit-token path exists (GitHubTokenEnv, ADR-0020 shape), the
      CLI inherits our env (precedence applies transparently), public `auth.getStatus` is the read
      seam. Dead-end recorded: no in-app device-flow initiation; method changes are choose-by-re-dial.
- [x] **A1 · Auth-method surface** (L; ADR) → issue [0068](0068-connection-page-auth-method-surface.md),
      PR #109, ADR-0039.
      A Connection page to choose + see the **active** method:
      (a) device flow (current/auto), (b) a pasted token saved locally **masked via `${VAR}`**
      (ADR-0020 reuse — no secret at rest), (c) reuse the `gh` CLI token. Shows the resolved precedence
      and which credential is live; never persists a raw secret.

## Acceptance (epic)

- [x] The active auth method + precedence is visible in the UI.
- [x] A user can supply a token that is stored **only** as a `${VAR}` reference (no plaintext at rest).
- [x] `gh`-reuse and device-flow paths are selectable/inspectable (in-app device-flow *initiation*
      is the SDK dead-end A0 recorded; the page links the user to `copilot` login instead).
- [x] A0 findings are written down before A1 design (0067, PR #108); A1 takes ADR-0039.
- [x] `make lint && make test` + `make e2e` green; born in its PR; SemVer minor.

## Notes

Lower priority than epic 0050 (billing fidelity is correctness in the core differentiator; this is
enablement). Sources for the auth taxonomy are in NEXT_FEATURES.md "Roadmap v10". Gated on the A0
spike — A1's shape depends on what the SDK exposes.
