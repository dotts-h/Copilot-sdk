---
id: 0019
title: MCP secrets / Env editor (roadmap v3, item C1)
status: open
severity: high
group: 0022
github:
links:
  adr: ../adr/0020-mcp-secrets-via-env-var-reference-indirection.md
  prs: []
  issues: [0022]
  regression:
assets: []
---

## Summary

The MCP-server page (ADR-0010) ships curated servers **key-free** and a form that
edits `command`/`args`/`enabled` but **not** `Env`: an existing `MCPServer.Env` is
preserved across edits (`internal/web/mcp.go` `handleMCPServerUpdate`) yet is only
settable by hand-editing `forge.json`. That blocks the highest-value MCP servers
(GitHub, web search), which need an API key — and MCP is how a user grows the agent's
tools, so this is the gate to the product's **extensibility** story. The key decision
(where secrets live, not plaintext in `forge.json`) is settled first in
**[ADR-0020](../adr/0020-mcp-secrets-via-env-var-reference-indirection.md)**:
**env-var-reference indirection**, no secret at rest, following the existing
`config.GitHubTokenEnv` → `config.GitHubToken()` precedent. Source:
`docs/NEXT_FEATURES.md` item C1; TECH_DEBT #10.

## Repro
1. Open the MCP page, add or edit a server that needs an API key (e.g. a GitHub MCP
   server).
   - **Expected:** a masked Env editor lets you map the key to a `${VAR}` reference; an
     unresolved reference is flagged in the page preflight before a session starts.
   - **Actual:** the form has no `Env` field at all; the only way to set a key is to
     hand-edit `forge.json`, and a key set there would sit in cleartext in the forge doc.

## Proposed resolution (per ADR-0020 — not yet built)

- **`MCPServer.Env` value semantics (additive, no schema change):** a value is a
  literal unless it matches `${VAR_NAME}` (`[A-Z_][A-Z0-9_]*`), which is a reference
  resolved from the process environment at session start. A pre-C1 forge (all literal
  values) loads and behaves identically.
- **Resolution behind a seam:** `web.MCPServerSpecs` (the single forge→seam
  translation) expands `${VAR_NAME}` via a lookup seam (default `os.Getenv`, injectable
  for tests); a reference that resolves empty is left **unset**, never sent as the
  literal `${VAR_NAME}`.
- **Form:** a repeatable key/value Env row plus a *secret* checkbox in
  `renderMCPServerForm` (`internal/web/mcp.go`) / a masked field helper
  (`internal/web/forms.go`); a secret row persists only `${VAR_NAME}`, the value input
  is masked and never round-trips to disk. Mirrors the forge-CRUD validated-builder +
  rollback-on-invalid pattern.
- **Preflight:** extend `mcpServersPartial` (already PATH-preflighting `command` behind
  `s.lookPath`) to report an enabled server's `${VAR_NAME}` that resolves empty.
- **Tests:** unit — a secret row persists only the reference (never the value); an
  unresolved reference is left unset, not sent literally; a literal Env value still
  round-trips. Contract — CONTRACTS §4 `MCPServer.Env` note updated; REGRESSIONS guard
  "a secret reference is never persisted as a literal / never sent unexpanded." e2e —
  the Env editor renders, accepts a masked secret row, and the forge doc shows the
  reference (structure, never a real key).

## Notes
- **Decision:** env-var-reference indirection over (a) a dedicated 0600 `secrets.json`
  store (puts a secret at rest for marginal in-UI-entry UX in a single-user tool;
  documented follow-up), (b) an OS keychain (CGO/platform dep, breaks the pure-Go
  invariant), or (c) plaintext in `forge.json` (the thing the roadmap forbids). Recorded
  in ADR-0020.
- **Numbering:** this claims the **reserved** issue **0019** and **ADR-0020**, held for
  C1 since the B3 pass (`docs/issues/0021` notes, ADR-0022) precisely so the numbers
  don't collide when C1 lands.
- **Differentiator:** extensibility (the gate to key-requiring MCP servers); pays down
  TECH_DEBT #10.
