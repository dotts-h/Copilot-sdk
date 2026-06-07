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
| [0024](0024-epic-convergence-dashboards-cost-surface.md) | Epic: convergence dashboards & cost-surface completion (roadmap v4) | closed | 0025, 0026, 0027, 0028, 0029 |
| [0030](0030-epic-orchestration-visibility-polish.md) | Epic: orchestration visibility & polish (roadmap v5) | closed | 0031, 0032, 0033 |
| [0031](0031-epic-orchestration-accountability.md) | Epic: orchestration accountability — Runs surface parity (roadmap v6) | open | 0034 |

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
| [0025](0025-workflow-last-run-cost-badges.md) | Workflow list "last run" + cost badges (item V4) | closed | medium | 0024 | ADR-0022 |
| [0026](0026-bucketed-burn-rate-forecast.md) | Per-workflow / per-agent bucketed burn-rate forecast (item F3) | closed | medium | 0024 | ADR-0018, ADR-0019 |
| [0027](0027-settings-price-override-editor.md) | Settings price-override editor (item G1/V2) | closed | medium | 0024 | REGRESSIONS (price-override reprice) |
| [0028](0028-per-session-cost-sessions-page.md) | Per-session cost on the Sessions page (item G2/V5) | closed | medium | 0024 | CONTRACTS §3/§4 |
| [0029](0029-telemetry-spend-window-selector.md) | Telemetry spend-window selector (item G3/V9) | closed | medium | 0024 | CONTRACTS §3 |
| [0031](0031-subagent-description-activity-strip.md) | Surface SubagentInfo.Description on the sub-agent activity strip (item V3) | closed | medium | 0030 | CONTRACTS §2, PR #56 |
| [0032](0032-keybinding-live-apply.md) | Keybinding live-apply — rebind without a full page reload (item V10) | closed | low | 0030 | ADR-0014, TECH_DEBT #13, REGRESSIONS #18, PR #57 |
| [0033](0033-generic-append-only-store.md) | Generic telemetry.AppendOnlyStore[T] — collapse the SpendStore/RunStore machinery (item H1) | closed | medium | 0030 | CONTRACTS §4, TECH_DEBT #14, PR #58 |
| [0034](0034-runs-csv-export.md) | Runs CSV export — the orchestration sibling of the spend ledger export (item V11) | closed | medium | 0031 | CONTRACTS §3/§4 |
