# Issues index

Source of truth for tracked issues. Markdown files here are canonical; mirror to
GitHub via `scripts/sync-github.sh` (requires `gh`). Regenerate this index when
issues are added or closed.

## Epics

| id | title | status | children |
|----|-------|--------|----------|
| [0001](0001-epic-cost-awareness.md) | Epic: make cost active (Tier 1) | in-progress | 0002, 0003 |

## Issues

| id | title | status | severity | group | links |
|----|-------|--------|----------|-------|-------|
| [0002](0002-pre-flight-turn-cost-estimate.md) | Pre-flight turn cost estimate (item 1.2) | closed | high | 0001 | ADR-0007 |
| [0003](0003-budget-guardrails.md) | Budget guardrails — soft warn + hard cap (item 1.1) | closed | high | 0001 | ADR-0008 |
