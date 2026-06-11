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
| [0031](0031-epic-orchestration-accountability.md) | Epic: orchestration accountability — Runs surface parity (roadmap v6) | closed | 0034, 0035, 0036, 0037 |
| [0038](0038-epic-cost-run-reconciliation.md) | Epic: cost⋈run reconciliation — converge the two persisted stores (roadmap v7) | closed | 0039, 0040, 0041 |
| [0042](0042-epic-interactive-orchestration.md) | Epic: interactive orchestration — the Runs surface goes actionable (roadmap v8) | closed | 0043, 0044 |
| [0045](0045-epic-ui-ux-refresh.md) | Epic: UI/UX refresh — token foundation, theming, navigation IA, telemetry dashboard, motion (roadmap v9) | closed | 0046, 0047, 0048, 0049 |
| [0069](0069-epic-first-class-subagents.md) | Epic: First-class sub-agents — live view, per-subagent cost, HITL pause/continue/cancel (roadmap v12) | closed | 0070, 0071, 0072, 0073, 0074, 0075 |
| [0076](0076-epic-designed-agent-output.md) | Epic: Designed agent output — rich, token-styled components from agent markdown (roadmap v13) | open | 0077, 0078, 0079, 0080, 0081, 0082 |
| [0083](0083-epic-orchestration-robustness-backpressure-replayability-considered-and-rejected-event-bus.md) | Epic: Orchestration robustness — backpressure + replayability (considered-and-rejected event bus) (roadmap v14) | open | 0084, 0085 |

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
| [0034](0034-runs-csv-export.md) | Runs CSV export — the orchestration sibling of the spend ledger export (item V11) | closed | medium | 0031 | CONTRACTS §3/§4, PR #59 |
| [0035](0035-runs-summary-total-cost.md) | Total cost on the per-workflow Runs summary (item V13) | closed | low | 0031 | CONTRACTS §3/§4, PR #61 |
| [0036](0036-runs-time-window-selector.md) | Runs time-window selector (item V12) | closed | low | 0031 | CONTRACTS §3, PR #62 |
| [0037](0037-runs-per-lane-cost-rollup.md) | Per-lane cost roll-up — Cost by lane on the Runs page (item V14) | closed | low | 0031 | CONTRACTS §3/§4, PR #64 |
| [0039](0039-cost-run-reconciliation.md) | Cost⋈run reconciliation — Ledger vs runs on the Telemetry page (item V15) | closed | medium | 0038 | CONTRACTS §3/§4, PR #66 |
| [0040](0040-per-lane-cost-run-reconciliation.md) | Per-lane cost⋈run reconciliation — Ledger vs runs by lane on the Telemetry page (item V16) | closed | medium | 0038 | CONTRACTS §3/§4, PR #74 |
| [0041](0041-reconciliation-csv-export.md) | Reconciliation CSV export — WriteReconcileCSV + GET /telemetry/reconcile.csv (item V17) | closed | low | 0038 | CONTRACTS §3/§4, PR #75 |
| [0043](0043-rerun-workflow-from-runs-page.md) | Rerun a recorded run from the Runs page — re-execute its workflow's current definition (item V18) | closed | medium | 0042 | ADR-0023, PR #76 |
| [0044](0044-abort-in-flight-run.md) | Abort an in-flight run from the Chat lanes panel — the dual of rerun (item V19) | closed | medium | 0042 | ADR-0024, PR #77 |
| [0046](0046-design-token-foundation-light-dark-theme.md) | Design-token foundation + light/dark theme — the UI/UX refresh foundation (item V21) | closed | medium | 0045 | ADR-0025, PR #44 |
| [0047](0047-grouped-sidebar-command-palette.md) | Navigation → grouped left sidebar + ⌘K command palette (item V22) | closed | medium | 0045 | ADR-0026, PR #79 |
| [0048](0048-telemetry-kpi-dashboard.md) | Telemetry dashboard — KPI cards + server-rendered inline-SVG sparklines (item V23) | closed | medium | 0045 | ADR-0027, REGRESSIONS #20, PR #81 |
| [0049](0049-motion-and-polish.md) | Motion & polish — htmx per-navigation View-Transition page swaps + a token-driven component pass (item V24) | closed | low | 0045 | ADR-0028, REGRESSIONS (global-VT dead-end), PR #82 |
| [0050](0050-epic-billing-fidelity.md) | Epic: Billing fidelity — price cache-write + reasoning tokens; authoritative-cost-first source hierarchy (roadmap v10, P0–P4) | closed | high | — | 0057, 0058, 0059, 0060, 0061, ADR-0033/0034/0035, PRs #95 #96 #98 #103, NEXT_FEATURES v10 |
| [0051](0051-epic-auth-and-connection.md) | Epic: Auth & connection — device flow / local ${VAR} token / gh reuse (roadmap v10, A0–A1) | closed | medium | — | 0067, 0068, ADR-0039, PRs #108 #109, NEXT_FEATURES v10 |
| [0052](0052-epic-safe-autopilot-governance.md) | Epic: Hooks & safe autopilot — first-class forge-managed Pre/PostToolUse hooks + safe-by-default governance policy (roadmap v10, G0–G5) | closed | high | — | 0053, 0054, 0055, 0056, ADR-0029, ADR-0030, ADR-0031, ADR-0032, NEXT_FEATURES v10 |
| [0053](0053-hooks-foundation-forge-entity-bridge-evaluator.md) | Hooks foundation — Hook forge entity + bridge evaluator + safe-read defaults (V25, G0+G3-mechanism+G1) | closed | high | 0052 | ADR-0029, PR #84 |
| [0054](0054-dangerous-action-deny-mandatory-hitl.md) | Dangerous-action deny + mandatory HITL — built-in unbypassable ruleset enforced on the auto path (V26, G2) | closed | high | 0052 | ADR-0030, PR #83 #85 |
| [0055](0055-hooks-ui-mode-binding-timeline-why.md) | Hooks management UI + mode binding + timeline why — in-app CRUD over governance hooks (V27, G4) | closed | high | 0052 | ADR-0031, PR #86 |
| [0056](0056-posttooluse-hook-command-execution.md) | PostToolUse hook command execution — bounded local command, untrusted display-only output (V28, G5) | closed | high | 0052 | ADR-0032, PR #87 |
| [0057](0057-authoritative-cost-first-metering.md) | Authoritative-cost-first metering — ReportedAIU is actual spend, price book is the estimate (P0) | closed | high | 0050 | depends_on: —, ADR-0033, REGRESSIONS #21, PR #95 |
| [0058](0058-per-model-breakdown-from-ledger.md) | Per-model breakdown from the ledger + the missing integration test (P2-core) | closed | high | 0050 | depends_on: —, REGRESSIONS #3, PR #96 |
| [0059](0059-price-cache-write-and-reasoning-tokens.md) | Price cache-write + reasoning tokens — promote out of display-only ExtraTokens into priced Usage (P1) | closed | high | 0050 | depends_on: 0057 0058, ADR-0034, PR #98 |
| [0060](0060-estimate-vs-reported-reconciliation-drift.md) | Estimate-vs-reported reconciliation + drift on the Telemetry page (P3) | closed | medium | 0050 | depends_on: 0057, PR #103 |
| [0061](0061-live-price-book-refresh.md) | Live price-book refresh — opt-in, cached, fail-open fetch of per-model multipliers (P4) | closed | low | 0050 | depends_on: 0059, ADR-0035 (refuted by spike — catalog has no pricing) |
| [0062](0062-epic-playful-polished-ui-motion-overhaul.md) | Epic: Playful-polished visual + motion overhaul — re-derived OKLCH palette, depth, spring motion system (roadmap v11) | closed | medium | — | 0063, 0064, 0065, 0066, PRs #101/#105/#106/#107, NEXT_FEATURES v11 |
| [0063](0063-token-palette-foundation-oklch-layer-openprops.md) | Token & palette foundation — re-derive palette in OKLCH, @layer, Open Props (W1) | closed | medium | 0062 | depends_on: —, ADR-0036 |
| [0064](0064-elevation-surface-component-restyle.md) | Elevation, surface & component restyle — luminance ladder + hue-tinted shadows, radius/space/type scales (W2) | closed | medium | 0062 | depends_on: 0063, ADR-0038, PR #105, REGRESSIONS #20 |
| [0065](0065-motion-microinteraction-system.md) | Motion & micro-interaction system — linear() springs, motion tokens, CSS-only interaction catalogue (W3) | closed | medium | 0062 | depends_on: 0063, ADR-0037, PR #106 |
| [0066](0066-hero-surface-polish-chat-telemetry.md) | Hero-surface polish — apply the full system to Chat + Telemetry (W4) | closed | low | 0062 | depends_on: 0064 0065, PR #107 — closes the epic |
| [0067](0067-auth-spike-sdkclient-auth-today.md) | Auth spike — how SDKClient authenticates today + the seam a Connection page needs (A0) | closed | medium | 0051 | depends_on: —, PR #108 |
| [0068](0068-connection-page-auth-method-surface.md) | Connection page — see + choose the active auth method: device flow / masked ${VAR} token / gh reuse (A1) | closed | medium | 0051 | depends_on: 0067, ADR-0039, PR #109 — closes the epic |
| [0070](0070-agentid-attribution-seam.md) | AgentID attribution through the seam — every normalized event knows root vs sub-agent (S1) | closed | high | 0069 | depends_on: —, SUBAGENTS_RESEARCH §0/§3, ADR-0040, PR #118 |
| [0071](0071-subagent-registry-live-list.md) | Sub-agent registry + live list — status, current activity, live credits beside the chat (S2) | closed | high | 0069 | depends_on: 0070, supersedes 0031's strip, SUBAGENTS_RESEARCH §5, ADR-0041, PR #120 |
| [0072](0072-per-subagent-cost-budget-leash.md) | Per-subagent cost + budget leash — live metering, ledger attribution, pre-Send cap (S3) | closed | high | 0069 | depends_on: 0070, ADR-0018 pattern, SUBAGENTS_RESEARCH §4, ADR-0042 |
| [0073](0073-pause-continue-cancel-escalate.md) | Pause / continue / cancel — typed pause records + the orchestrator escalate tool (S4) | closed | high | 0069 | depends_on: 0070, permBridge/ADR-0008/ADR-0024, SUBAGENTS_RESEARCH §1–2, ADR-0043 |
| [0074](0074-subagent-chat-overlay.md) | Per-subagent chat overlay — popup transcript with live stream, pause form, and steer (S5) | closed | medium | 0069 | depends_on: 0071, see also 0073, SUBAGENTS_RESEARCH §3/§5, ADR-0044 |
| [0075](0075-subagent-attention-surface.md) | Attention surface — needs-you badging, title/favicon dot, Runs integration (S6) | closed | medium | 0069 | depends_on: 0071+0073, ADR-0022 run store — closes the epic |
| [0076](0076-epic-designed-agent-output.md) | Epic: Designed agent output — block-AST + token-styled components from markdown (roadmap v13) | open | medium | | children 0077–0082; NEXT_FEATURES roadmap-v13; ADR-0001 doctrine |
| [0077](0077-block-ast-seam.md) | Block-AST seam — parse markdown to a typed []Block, render byte-identically (R1) | closed | medium | 0076 | depends_on: —, keystone; ADR-0045, builds on ADR-0001 |
| [0078](0078-callouts-admonitions.md) | Callouts / admonitions — GitHub-alert blockquotes as designed callout components (R2) | closed | medium | 0076 | depends_on: 0077, highest visual ROI; ADR-0046 |
| [0079](0079-designed-code-blocks.md) | Designed code blocks — language label + copy affordance + token-styled frame (R3) | closed | medium | 0076 | depends_on: 0077, no highlighting lib |
| [0080](0080-tables.md) | Tables — GFM pipe tables as token-styled table components (R4) | closed | medium | 0076 | depends_on: 0077, the missing common block |
| [0081](0081-container-directives.md) | Container directives — :::card / :::details, allowlisted model-authorable blocks (R5) | closed | medium | 0076 | depends_on: 0077+0078, ADR-0047 grammar+allowlist |
| [0082](0082-citation-cards.md) | Citation cards — inline [n] markers + server-rendered source cards (R6, stretch) | open | low | 0076 | depends_on: 0077, Perplexity pattern; needs a source-bearing surface |
| [0083](0083-epic-orchestration-robustness-backpressure-replayability-considered-and-rejected-event-bus.md) | Epic: Orchestration robustness — backpressure + replayability (considered-and-rejected event bus) | open | medium | | children 0084–0085; NEXT_FEATURES roadmap-v14; event-bus evaluation |
| [0084](0084-bounded-lane-worker-pool-backpressure-for-wide-parallel-workflows.md) | Bounded lane worker pool — backpressure for wide parallel workflows | closed | medium | 0083 | depends_on: —, bounds launchLanes fan-out; no new seam; PR #148 |
| [0085](0085-per-run-event-log-for-replay-audit-on-appendonlystore.md) | Per-run event log for replay/audit on AppendOnlyStore | closed | medium | 0083 | depends_on: —, ADR-0048, builds on AppendOnlyStore (0033); replay vs summary |
