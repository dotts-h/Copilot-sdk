# 0003. Claude-CLI-style agents: built-in chat agent and per-agent tool allowlist

- Status: accepted
- Date: 2026-06-04
- Deciders: Horia
- Related: backlog item #9 (roadmap memory), `internal/ctxforge`, `internal/copilot`, `internal/web`

## Context

Backlog item #9 ("Claude-CLI-style skills/agents + a default chat agent") bundles
four loosely-defined parts: a default chat agent, per-agent tool allowlists +
auto-delegation descriptions, importing well-known instruction files
(`.github/copilot-instructions.md`, `AGENTS.md`, `CLAUDE.md`), and extending
Skill toward Claude's on-disk folder/`SKILL.md` model (resources, allowed-tools).
A fresh forge has no agents, so the Agents page is empty and there is no baseline
persona. Agents already carry model/effort/system-message/skills but cannot
restrict which tools a session may call, even though the SDK's session config
exposes `AvailableTools`.

## Considered options

- **Full scope incl. SKILL.md folders** — also load skills from on-disk folders
  with per-skill resources. Rejected for now: the most speculative part, needs
  runtime support for skill resources the SDK may not expose, and is separable.
- **Default agent + import only** — smallest; skips the tool allowlist, leaving
  the SDK's `AvailableTools` unused.
- **Focused slice** — built-in chat agent + per-agent tool allowlist (wired to
  the SDK) + auto-delegation description + import instruction files.

## Decision

We chose the **focused slice**. (1) A built-in `chat` agent
(`ctxforge.DefaultChatAgent`) is always resolvable and listed when the forge has
no `chat` agent, so chat has a baseline persona with zero config. (2)
`ctxforge.Agent` gains `AllowedTools []string`; it threads through
`copilot.SessionSpec.AllowedTools` to the SDK session config's `AvailableTools`,
and is applied when an agent is activated (shared by `/agent` and the Agents-page
select). The existing `Description` is the auto-delegation hint. (3) An "import
project instructions" action scans the working directory for the three
well-known files and creates/updates `Instruction`s. The Claude folder/`SKILL.md`
model is **deferred** and tracked as tech-debt.

## Consequences

- Positive: chat works with no forge config; agents can scope tools (least
  privilege) via the SDK; projects bootstrap their guidance from existing
  instruction files. All testable through the seam/forge without a live SDK.
- Negative / cost we accept: the built-in agent is virtual (not persisted), so it
  cannot be edited/deleted; `AllowedTools` is a flat name list (no per-tool
  config); import is a one-shot copy, not a live link to the files.
- Follow-ups: tech-debt entry for the SKILL.md folder/resources model; a guard
  test that an agent's `AllowedTools` reaches the session spec.
