# Issues index

Source of truth for tracked issues. Markdown files here are canonical; mirror to
GitHub via `scripts/sync-github.sh` (requires `gh`). Regenerate this index when
issues are added or closed.

## Epics

| id | title | status | children |
|----|-------|--------|----------|
| [0001](0001-epic-cost-awareness.md) | Epic: make cost active (Tier 1) | closed | 0002, 0003, 0004 |
| [0005](0005-epic-orchestration.md) | Epic: make it an orchestra (Tier 2) | closed | 0006, 0010 |
| [0007](0007-epic-polish.md) | Epic: polish that compounds (Tier 3) | closed | 0008, 0009, 0011, 0012 |

## Issues

| id | title | status | severity | group | links |
|----|-------|--------|----------|-------|-------|
| [0002](0002-pre-flight-turn-cost-estimate.md) | Pre-flight turn cost estimate (item 1.2) | closed | high | 0001 | ADR-0007 |
| [0003](0003-budget-guardrails.md) | Budget guardrails — soft warn + hard cap (item 1.1) | closed | high | 0001 | ADR-0008 |
| [0004](0004-persisted-spend-history.md) | Persisted spend history + trends (item 1.3) | closed | high | 0001 | ADR-0009 |
| [0006](0006-mcp-server-management-page.md) | MCP server management page + curated defaults (item 2.2) | closed | high | 0005 | ADR-0010 |
| [0008](0008-per-session-telemetry-totals.md) | Per-session telemetry totals (item 3.2) | closed | medium | 0007 | ADR-0011 |
| [0009](0009-diff-review-lane.md) | Diff review lane (item 3.1) | closed | medium | 0007 | ADR-0012 |
| [0010](0010-multi-agent-run-handoff.md) | Multi-agent run / handoff surface (item 2.1) | closed | high | 0005 | ADR-0013 |
| [0011](0011-keybinding-surface.md) | Keybinding surface (item 3.3) | closed | low | 0007 | ADR-0014 |
| [0012](0012-prompt-snippet-library.md) | Prompt/snippet library (item 3.4) | closed | low | 0007 | ADR-0015 |
