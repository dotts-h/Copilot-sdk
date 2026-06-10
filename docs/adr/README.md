# Architecture Decision Records

One decision per file (MADR-lite). Superseded records keep their file; their status points forward.

| # | Decision | Status |
|---|----------|--------|
| 0001 | [Render markdown server-side for committed agent turns](0001-render-markdown-server-side-for-committed-agent-turns.md) | accepted |
| 0002 | [Restore SDK session resume for session pick/start/continue](0002-restore-sdk-session-resume-for-session-pick-start-continue.md) | accepted |
| 0003 | [Claude-CLI-style agents: built-in chat agent and per-agent tool allowlist](0003-claude-cli-style-agents-built-in-chat-agent-and-per-agent-tool-allowlist.md) | accepted |
| 0004 | [Fold supporting docs into the feature branch](0004-fold-supporting-docs-into-the-feature-branch.md) | accepted |
| 0005 | [Relicense to BSL 1.1](0005-relicense-to-bsl-1-1.md) | accepted |
| 0006 | [Desktop shell via Wails v3 wrapping the local HTTP server](0006-desktop-shell-via-wails-v3-localhost-window.md) | accepted |
| 0007 | [Pre-flight turn cost estimate prices the live context as fresh input](0007-pre-flight-turn-cost-estimate-prices-context-as-fresh-input.md) | accepted |
| 0008 | [Budget guardrails: a soft warn and a hard-cap turn gate](0008-budget-guardrails-soft-warn-and-hard-cap-gate.md) | accepted |
| 0009 | [Persisted spend history: an append-only, atomically-written ledger](0009-persisted-spend-history-append-only-ledger.md) | accepted |
| 0010 | [MCP server management page: curated defaults, disabled-by-default, with a PATH preflight](0010-mcp-server-management-page-curated-defaults-disabled-with-preflight.md) | accepted |
| 0011 | [Per-session telemetry meter for the statusline](0011-per-session-telemetry-meter-for-the-statusline.md) | accepted |
| 0012 | [Diff review lane for file-write permissions](0012-diff-review-lane-for-file-write-permissions.md) | accepted |
| 0013 | [Multi-agent workflow run / handoff surface](0013-multi-agent-workflow-run-handoff-surface.md) | accepted |
| 0014 | [Keybinding surface: a config-backed keymap with minimal JS dispatch](0014-keybinding-surface-config-backed-keymap-with-minimal-js-dispatch.md) | accepted |
| 0015 | [Prompt/snippet library: forge-backed composer insertion](0015-prompt-snippet-library-forge-backed-composer-insertion.md) | accepted |
| 0016 | [The persisted ledger is the source of truth for account-wide budget accounting](0016-ledger-is-source-of-truth-for-account-wide-budget-accounting.md) | proposed |
| 0017 | [Per-lane tool + permission surface for parallel workflow lanes](0017-per-lane-tool-and-permission-surface-for-parallel-workflow-lanes.md) | accepted |
| 0018 | [Additive agent/workflow attribution tags on spend records](0018-additive-attribution-tags-on-spend-records.md) | accepted |
| 0019 | [Budget burn-rate forecast: a trailing-window average over the daily ledger](0019-budget-burn-rate-forecast-trailing-window-average.md) | accepted |
| 0020 | [MCP secrets via `${VAR}` env-reference indirection](0020-mcp-secrets-via-env-var-reference-indirection.md) | accepted |
| 0021 | [Conditional / branching workflow steps — a declarative step predicate](0021-conditional-branching-workflow-steps-declarative-predicate.md) | accepted |
| 0022 | [Workflow run history: a sibling append-only run store](0022-workflow-run-history-sibling-append-only-run-store.md) | accepted |
| 0023 | [Rerun a recorded run re-executes the current workflow definition](0023-rerun-a-recorded-run-re-executes-the-current-workflow-definition.md) | accepted |
| 0024 | [Abort an in-flight run — settle it as failed and abort its lane sessions](0024-abort-an-in-flight-run-settles-it-as-failed-and-aborts-its-lane-sessions.md) | accepted |
| 0025 | [Design-token foundation and light/dark theming via `light-dark()` + `color-scheme`](0025-design-token-foundation-and-light-dark-theming.md) | accepted |
| 0026 | [Grouped left-sidebar navigation and a ⌘K command palette](0026-grouped-sidebar-navigation-and-command-palette.md) | accepted |
| 0027 | [Telemetry KPI dashboard: pure readers + pure Go SVG builders, server-rendered inline charts](0027-telemetry-kpi-dashboard-server-rendered-inline-svg.md) | accepted |
| 0028 | [Motion & polish: htmx per-navigation View Transitions (NOT global), plus a token-driven component pass](0028-motion-and-polish-htmx-per-navigation-view-transitions.md) | accepted |
| 0029 | [Hooks: a forge-backed Pre/PostToolUse entity, bridge-enforced allow/deny/ask, deny-wins, safe-read defaults](0029-hooks-forge-entity-bridge-enforced-allow-deny-ask-safe-read-defaults.md) | accepted |
| 0030 | [Dangerous-action deny + mandatory HITL, unbypassable by config](0030-dangerous-action-deny-and-mandatory-hitl-unbypassable-by-config.md) | accepted |
| 0031 | [Hooks management UI + mode binding + the timeline "why" annotation](0031-hooks-management-ui-mode-binding-and-timeline-why.md) | accepted |
| 0032 | [PostToolUse hook command execution: a bounded local command with untrusted, display-only output](0032-posttooluse-hook-command-execution-untrusted-output.md) | accepted |
| 0033 | [ReportedAIU is the source of truth for actual spend; the price book is the estimate/fallback](0033-reportedaiu-is-source-of-truth-for-actual-spend-price-book-is-the-estimate.md) | accepted |
| 0034 | [Price cache-write tokens (additive, 1.25× input); reasoning is a subset of output, not a second charge](0034-price-cache-write-additive-reasoning-is-output-subset.md) | accepted |
| 0035 | [No live price-book refresh — the models catalog carries no pricing; the static book + ReportedAIU already self-heal](0035-no-live-price-book-refresh-catalog-has-no-pricing.md) | accepted |
| 0036 | [OKLCH palette re-derivation (terracotta kept), the `@layer` ordering contract, vendored Open Props subset](0036-oklch-palette-rederivation-layer-contract-vendored-open-props.md) | accepted |
