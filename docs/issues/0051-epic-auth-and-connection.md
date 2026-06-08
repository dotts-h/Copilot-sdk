---
id: 0051
title: "Epic: Auth & connection — choose how to reach Copilot (device flow / local token / gh reuse) (roadmap v10)"
status: open
severity: medium
group:
github:
links:
  adr: []
  prs: []
  issues: []
  regression: []
---

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

- [ ] **A0 · Auth spike** (S). Document how `SDKClient` authenticates **today** — which method the
      underlying CLI/SDK resolves, and what (if anything) we can influence. Output: a short findings
      note + the seam A1 needs. (Recorded as a dead-end/decision input if a path proves closed.)
- [ ] **A1 · Auth-method surface** (L; ADR). A Connection page to choose + see the **active** method:
      (a) device flow (current/auto), (b) a pasted token saved locally **masked via `${VAR}`**
      (ADR-0020 reuse — no secret at rest), (c) reuse the `gh` CLI token. Shows the resolved precedence
      and which credential is live; never persists a raw secret.

## Acceptance (epic)

- [ ] The active auth method + precedence is visible in the UI.
- [ ] A user can supply a token that is stored **only** as a `${VAR}` reference (no plaintext at rest).
- [ ] `gh`-reuse and device-flow paths are selectable/inspectable.
- [ ] A0 findings are written down before A1 design; A1 takes an ADR.
- [ ] `make lint && make test` + `make e2e` green; born in its PR; SemVer minor.

## Notes

Lower priority than epic 0050 (billing fidelity is correctness in the core differentiator; this is
enablement). Sources for the auth taxonomy are in NEXT_FEATURES.md "Roadmap v10". Gated on the A0
spike — A1's shape depends on what the SDK exposes.
