# Tech-debt register

> Tracked, prioritized shortcuts and gaps. Severity = impact if it bites. Effort = cost to fix.
> Interest = ongoing cost of leaving it. Rank by interest × likelihood.

| # | Item | Location | Sev | Effort | Interest | Links | Trigger to pay down |
|---|------|----------|-----|--------|----------|-------|---------------------|
| 1 | Claude folder/`SKILL.md` skill model — per-skill resources + allowed-tools, loading skills from on-disk folders. Deferred from backlog #9 (the focused slice shipped instead). Needs runtime support for skill resources the SDK may not expose. | internal/ctxforge | low | L | low | [ADR 0003](adr/0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md) | when on-disk skill folders or per-skill resources are actually requested |
| 2 | Statusline token/credit totals are meter-global, not per-session. | internal/web (renderStatline) | low | M | low | — | when multi-session accounting matters |
| 3 | Docs say "single in-memory session"; the Hub is cookie-keyed multi-session. | docs/ARCHITECTURE.md, README | low | S | med | — | next docs pass |
| 4 | Web agent activation doesn't compile the agent's full system message (instructions + skill prompts) into the session — only model/effort/tools are applied. | internal/web (applyAgentSpec) | med | M | med | [ADR 0003](adr/0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md) | when agent personas/skills must affect chat behavior |
