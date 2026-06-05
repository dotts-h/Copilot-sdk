# 0010. MCP server management page: curated defaults, disabled-by-default, with a PATH preflight

- Status: accepted
- Date: 2026-06-05
- Deciders: Horia
- Related: `internal/ctxforge` (`MCPServer`, `AddMCPServer`/`UpdateMCPServer`/
  `ToggleMCPServer`/`RemoveMCPServer`, `Compile` MCP wiring), `internal/web`
  (`mcp.go` page + form + handlers + the `lookPath` preflight seam, `hub.go`
  routes, `pages.go` nav), `internal/bootstrap` (`SeedForge`, `curatedMCPServers`),
  `internal/copilot/sdkclient.go` (stdio MCP wiring), `docs/NEXT_FEATURES.md`
  item 2.2, [ADR-0003](0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md)

## Context

Skills, Instructions, and Agents each have a full CRUD page; **MCP servers did
not**, even though `ctxforge.MCPServer` is a first-class forge entity that
`Forge.Compile` already wires into the session and `sdkclient.go` already maps to
the SDK's `MCPStdioServerConfig`. It was the one forge entity editable only by
hand-writing `forge.json`. MCP is how a user extends the agent's tools, so this
gap blocked real customization. Item 2.2 asks to close it *and* ship a curated
set of well-known servers so the page isn't empty on first run.

Two questions beyond the mechanical CRUD mirror needed deciding: **how curated
defaults are baked in** (and whether they're enabled), and **how to keep the
otherwise-pure page rendering honest about whether a server can actually run**.

## Considered options

- **Bake-in model.** MCP servers here are **stdio = external processes**
  (`MCPStdioServerConfig`: a `Command` + `Args` the runtime execs). A seeded
  `npx …` / `uvx …` entry is only runnable if that command is present on the host.
  - *Seed enabled* (rejected): auto-enabling a server whose binary is absent
    surprise-fails at session start, and clashes with the project's offline
    single-binary value (htmx is vendored precisely to avoid runtime fetches).
  - *Seed disabled + PATH preflight* (chosen): seed a handful of key-free stdio
    servers **disabled by default**, and on the page run `exec.LookPath` against
    each server's `Command`, badging the ones not on `PATH` as **unavailable**.
    Seeding *config* is cheap; the user opts in only after the preflight confirms
    the dependency is installed.
- **Secrets.** The highest-value servers (web search, GitHub) need API keys.
  `MCPServer.Env` exists but there is **no secrets UI/handling**. Decision: the
  curated defaults are **key-free only** (filesystem, git, fetch, memory,
  sequential-thinking, time), the form **does not edit `Env`**, and an existing
  `Env` set by hand in `forge.json` is **preserved across edits** (the update
  handler copies it forward) rather than silently wiped. A real secrets story is
  its own scoped item before shipping key-requiring servers.
- **Where the preflight impurity lives.** The forge and page rendering are pure
  and unit-tested; `exec.LookPath` touches the host. Decision: isolate it behind a
  **`lookPath func(string)(string,error)` seam** on the `Hub`/`Server` (defaults to
  `exec.LookPath`, injected as a fake in tests), so the renderer stays
  deterministic and the one impurity is a single, mockable function.
- **Actually wiring enabled servers into the session (gap found in review).** The
  premise was that `Compile` "already wires MCP into the session." It does populate
  `ctxforge.SessionSpec.MCPServers`, but the web/bootstrap translation to the
  `copilot.SessionSpec` copied system-message/tools/model/effort and **dropped MCP
  servers**, so enabling one had no runtime effect — a cosmetic toggle. Decision:
  thread the compiled servers through the same two translation points the other
  compiled fields use (`compiledSpec`→`applyAgentSpec` for agent restarts;
  `bootstrap.Build` for the initial session) via one shared `web.MCPServerSpecs`
  converter. And because the SDK config map was keyed by the **non-unique,
  non-required `Name`** (so a clash silently overwrote an entry), add
  `copilot.MCPServer.ID`/`Key()` and key the map by the unique id (fallback to Name
  for legacy callers). Tightening `MCPServer.Validate` to require/unique `Name` was
  rejected — it would break `Load` on existing `forge.json` files (backward-read
  invariant).

## Decision

`ctxforge` gains the standard validated builders for MCP servers
(`AddMCPServer`/`UpdateMCPServer`/`ToggleMCPServer`/`RemoveMCPServer` +
`MCPServer(id)` lookup), each rolling back on an invalid whole-forge result,
exactly like skills/instructions/agents. The web layer adds an **MCP** nav page
(in nav order after Agents) with add/edit/toggle/delete, routed under `/mcp…`
mirroring the existing forge-CRUD routes. The page runs the `lookPath` preflight
per row and badges unavailable commands. `bootstrap.SeedForge` backfills the
curated set **only when no MCP servers exist** (independent of the other kinds),
all **disabled**, all **key-free**.

The new routes are `internal` stability (CONTRACTS §3, the forge-CRUD group); the
`MCPServer` schema was already listed in CONTRACTS §4 and is unchanged.

## Consequences

- Positive: the last forge entity now has a UI; users extend the agent's tools
  without editing JSON. Curated defaults make MCP discoverable on first run, and
  the preflight makes the host dependency **visible** instead of a session-start
  failure. The CRUD reuses the proven rollback-on-invalid-save discipline; the
  page stays unit-testable behind the `lookPath` seam.
- Trade-off we accept: a seeded server is inert until the user installs its
  command and enables it — capability is *not* baked in, only config. Baking in a
  true zero-dependency baseline (an embedded first-party Go MCP server, sidecar or
  in-process) is a larger, separate move (TECH_DEBT follow-up), not this item.
- Trade-off: the preflight is a point-in-time `LookPath` at render, not a live
  watch; a command installed after the page renders shows stale until reload. For
  a localhost tool this is fine and keeps the seam trivial.
- Follow-ups (tracked, not built): a secrets/`Env` editor to unlock key-requiring
  servers (GitHub, web search), and the embedded first-party server for an
  out-of-the-box baseline with no external runtime.
