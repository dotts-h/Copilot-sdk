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
| [0013](0013-epic-deepen-differentiators.md) | Epic: deepen the differentiators (roadmap v2) | closed | 0014, 0015, 0016, 0017, 0018, 0020, 0021 |
| [0022](0022-epic-extensibility-and-convergence.md) | Epic: extensibility & convergence (roadmap v3) | closed | 0019, 0023 |

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
| [0014](0014-ledger-derived-budget-rows.md) | Ledger-derived budget rows (item A1) | closed | high | 0013 | ADR-0016, TECH_DEBT #9 |
| [0015](0015-real-parallel-workflow-lanes.md) | Real parallel workflow lanes (item B1) | closed | medium | 0013 | ADR-0013, TECH_DEBT #12 |
| [0016](0016-cost-attribution-rollups.md) | Cost attribution — per-agent/per-workflow rollups (item A2) | closed | high | 0013 | ADR-0018 |
| [0017](0017-budget-burn-rate-forecast.md) | Budget burn-rate projection / forecast (item A3) | closed | medium | 0013 | ADR-0019 |
| [0018](0018-textarea-composer.md) | Textarea composer — Enter sends, Shift-Enter newline (item C2) | closed | low | 0013 | TECH_DEBT #15 |
| [0019](0019-mcp-secrets-env-editor.md) | MCP secrets / Env editor (item C1) | closed | high | 0022 | ADR-0020, TECH_DEBT #10 |
| [0020](0020-conditional-branching-workflow-steps.md) | Conditional / branching workflow steps (item B2) | closed | medium | 0013 | ADR-0021 |
| [0021](0021-workflow-run-history.md) | Workflow run history (item B3) | closed | medium | 0013 | ADR-0022 |
| [0023](0023-workflow-run-aggregations.md) | Workflow run-history aggregations + Runs duration (item V1) | closed | medium | 0022 | ADR-0022 |
