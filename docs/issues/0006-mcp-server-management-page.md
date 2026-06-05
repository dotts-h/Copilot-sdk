---
id: 0006
title: MCP server management page + curated defaults (Tier 2, item 2.2)
status: closed
severity: high
group: 0005
github:
links:
  adr: ../adr/0010-mcp-server-management-page-curated-defaults-disabled-with-preflight.md
  prs: []
  issues: [0005]
  regression:
assets: []
---

## Summary

Skills/Instructions/Agents each had a full CRUD page; **MCP servers did not**,
even though `ctxforge.MCPServer` is a first-class forge entity that `Compile`
already wires into the session. It was the one forge entity editable only by
hand-writing `forge.json`. Add the nav page + add/edit/toggle/delete mirroring the
forge-CRUD pattern, and seed a curated set of well-known stdio servers disabled by
default with a PATH preflight. Source: `docs/NEXT_FEATURES.md` item 2.2.

## Repro
1. Open the app and look for a way to add/manage MCP servers in the UI.
2. Expected: a nav page to add/edit/enable/delete MCP servers, with curated
   defaults to start from.
3. Actual (before): no UI at all; `settings.go` said outright they "are not
   exposed here." Only hand-editing `forge.json` worked.

## Resolution

- **`internal/ctxforge`**: validated builders `AddMCPServer`/`UpdateMCPServer`/
  `ToggleMCPServer`/`RemoveMCPServer` + `MCPServer(id)` lookup, rolling back on an
  invalid whole-forge result (rename collisions, empty command) like the other
  entities. Domain stays pure.
- **`internal/web`** (`mcp.go`): an **MCP** nav page (after Agents) with
  add/edit/toggle/delete; routes `/mcp…` mirror the skills/agents group. A PATH
  **preflight** (`exec.LookPath`, behind the `s.lookPath` seam) badges servers
  whose command isn't installed as **unavailable**. The form does not edit `Env`
  (no secrets surface yet); an existing `Env` is preserved across edits.
- **`internal/bootstrap`** (`SeedForge`/`curatedMCPServers`): six well-known,
  key-free stdio servers (filesystem, git, fetch, memory, sequential-thinking,
  time), seeded **disabled** and backfilled independently of the other kinds.
- **Closed a wiring gap found in review**: the compiled forge `SessionSpec`'s MCP
  servers were dropped when translating to the `copilot.SessionSpec`, so enabling a
  server had no runtime effect. Threaded them through `compiledSpec`/`applyAgentSpec`
  + `bootstrap.Build` via `web.MCPServerSpecs`, and added `copilot.MCPServer.ID`/
  `Key()` so the SDK config map keys on the unique id (a non-unique `Name` no longer
  collides). See REGRESSIONS #15.
- Design recorded in **ADR-0010** (disabled-by-default + preflight bake-in model;
  key-free curated set; the `lookPath` seam isolating the one impurity; the
  compile→spec wiring + id-keying).

## Notes

Guarding tests: `internal/ctxforge` `TestAddMCPServer`, `TestMCPServerLookup`,
`TestUpdateMCPServer`, `TestUpdateMCPServerRenameCollisionRollsBack`,
`TestToggleMCPServer`, `TestRemoveMCPServer`; `internal/web` `TestMCPNewForm`,
`TestMCPCreate`, `TestMCPCreateInvalidReshowsForm`, `TestMCPEditFormPrefilled`,
`TestMCPUpdatePreservesEnv`, `TestMCPToggleViaHandler`, `TestMCPDeleteViaHandler`,
`TestMCPPagePreflightMarksUnavailable`, `TestNewServerDefaultsLookPath`;
`internal/bootstrap` `TestSeedForgeSeedsMCPServersDisabled`,
`TestSeedForgePreservesExistingMCPServers`; browser: `e2e/tests/e2e.spec.ts`
"MCP server management" (lists/toggles, adds via form), plus the nav-count and
every-nav-link specs (the MCP page is in `pages.length`). Routes + schema in
CONTRACTS §3/§4; preflight-seam gotcha in REGRESSIONS; follow-ups (secrets/`Env`
editor, embedded first-party server) in TECH_DEBT. Closes item 2.2 of epic 0005.
