# Next-features research — my-orchestra (roadmap v3)

> Research deliverable, not a commitment. Fresh pass — code re-read **2026-06-06**.
> Roadmap **v1** (items 1.1–3.4, epics 0001/0005/0007) and **v2** (epic 0013:
> A1/B1/A2/A3/C2/B2/B3) are both shipped and closed; this supersedes them (their
> items are summarized in the appendices below). Grounds the *next* candidates in the
> current code, `docs/TECH_DEBT.md`, and the product's two differentiators —
> **cost-awareness** (the meter) and **orchestration** (the name). Each item names the
> seam/files it touches, a rough effort (S/M/L), and a "why now." The build-first picks
> are promoted to `docs/issues/` under epic **0022**; the rest stay candidates here
> until promoted. Architectural choices are recorded as ADRs.

## Where the product is now

Both differentiators are now **deep**, not shallow. **Cost** is accountable *across
time* (the ledger is the source of truth for account-wide rows — they survive restart,
ADR-0016), *attributable* per agent/workflow/lane (ADR-0018), and *predictive* (a
trailing-window burn-rate forecast, ADR-0019). **Orchestration** has *sequential* +
*parallel* + *branching* lanes (ADR-0013/0017/0021), each lane surfaces its own tool
timeline + inline permissions, and every run is *persisted history* in a sibling
`RunStore` with a Runs view (ADR-0022). The chat loop, forge CRUD (skills/instructions/
agents/MCP/workflows/snippets), session resume, keybindings, a desktop shell, and a full
Go + Playwright pyramid are all in place.

So v3 is not about deepening *within* a differentiator — it's about **reach and
convergence**:

- **Extensibility is gated.** MCP is how a user grows the agent's tools, but the MCP
  page is **key-free**: no secrets/`Env` editor, so the highest-value servers (GitHub,
  web search) can't be set up from the UI — only by hand-editing `forge.json`
  (TECH_DEBT #10; confirmed `mcp.go` `renderMCPServerForm` has no `Env` field,
  `handleMCPServerUpdate:129` only *preserves* it). The blocker was always *where
  secrets live*; now decided in **ADR-0020** (env-var-reference indirection, no secret
  at rest, following the `config.GitHubTokenEnv` precedent).
- **The two stores never meet.** B3 persists runs (`RunStore`) and A2 tags spend per
  workflow (`SpendStore`), but no view **joins** them: Runs is a flat log with no
  duration (`runs.go` `runRow` renders only `StartedAt`, never `FinishedAt`) and no
  roll-up; `WorkflowShares` shows spend but not run count / avg duration / failure rate.
  The cost ⋈ orchestration convergence is one pure reader away — exactly the
  aggregations ADR-0022 deferred.

Candidates below are ranked by value × fit; tech-debt paydown that advances a
differentiator is folded in where it earns its place.

---

## Tier E — open extensibility (the gate to key-requiring MCP servers)

### E1 / C1 — MCP secrets / Env editor  — **M**  ·  *promotes TECH_DEBT #10*  ·  **BUILD FIRST**
- **What:** a masked `Env` editor on the MCP form. A secret row persists only a
  `${VAR}` **reference**, resolved from the environment at session start in the single
  forge→seam translation (`web.MCPServerSpecs`) behind a lookup seam — **no secret at
  rest**. Unblocks key-requiring curated servers (GitHub, web search). The page
  preflight (already PATH-probing `command` behind `s.lookPath`) also flags an
  unresolved reference.
- **Why now:** MCP is the product's extensibility surface and this is its one gate; the
  key decision (where secrets live) is now settled in ADR-0020, so it's build-ready.
  Highest-value carried candidate across v1→v3.
- **Touches:** `internal/ctxforge` (`MCPServer.Env` value semantics — additive, no
  schema change), `internal/web` (`mcp.go` form + `MCPServerSpecs` resolution,
  `forms.go` masked field), CONTRACTS §4 (lands with the build).
- **Decision → [ADR-0020](adr/0020-mcp-secrets-via-env-var-reference-indirection.md)**
  (written first, ADR-0004). **Issue [0019](issues/0019-mcp-secrets-env-editor.md)**
  (claims the reserved 0019/ADR-0020).

## Tier F — converge cost ⋈ orchestration (join the two stores)

### F1 / V1 — Run-history aggregations + Runs duration  — **S**  ·  **BUILD FIRST**
- **What:** a pure `RunAggregates(records) []RunAggregate` (run count, avg cost, avg
  duration, failure rate per workflow) + a `RunRecord.Duration()` helper over the
  existing `RunStore` records (**no schema change**); a duration column + a per-workflow
  summary on the Runs page, joining the run and spend stores.
- **Why now:** the exact pure-reader follow-on ADR-0022 deferred ("records carry enough
  to compute them later without a schema change"); the convergence of the two
  differentiators; lowest-risk build on B3.
- **Touches:** `internal/telemetry` (`runs.go`: `RunAggregates`, `Duration` — pure +
  tested, a cousin of `*Shares`), `internal/web` (`runs.go` `runRow`/`runsPartial`).
- **No ADR** (decision pre-blessed by ADR-0022). **Issue
  [0023](issues/0023-workflow-run-aggregations.md).**

### F2 / V4 — Workflow list "last run" + cost badges  — **M**  ·  **SHIPPED** (epic 0024, issue 0025)
- **What:** each row on the Workflows page (was name + step summary only,
  `workflow.go` `workflowsPartial`) gains a last-run outcome glyph + age, a run count,
  and total spend — joining `RunStore.Records()` (via `RunAggregates`, extended with a
  `LastOutcome`/`LastStartedAt` last-run signal) and `WorkflowShares` keyed by workflow
  id. Makes the page diagnostic, not just navigational.
- **Why now:** reuses F1's aggregations + the existing `WorkflowShares`; turns the
  orchestration entry point into a cost-aware dashboard. **Touches:** `internal/web`
  (`workflow.go` `workflowsPartial`), `internal/telemetry` (extends F1's `RunAggregate`).
- **No ADR** (pure-reader composition pre-blessed by ADR-0022). **First build of epic
  [0024](issues/0024-epic-convergence-dashboards-cost-surface.md) (roadmap v4); issue
  [0025](issues/0025-workflow-last-run-cost-badges.md).**

### F3 / V7 — Per-workflow / per-agent burn-rate forecast  — **M**  ·  **SHIPPED** (epic 0024, issue 0026)
- **What:** a bucketed `Forecast` reader (`forecast.go` was account-wide only) that
  projects *"at this pace, the `review` workflow burns ~X cr/day"* from `DailyTotals`
  bucketed by the A2 `agent`/`workflow` tag. Trajectory, not just the historical share
  `AgentShares`/`WorkflowShares` already show.
- **Why now:** where cost prediction (A3) and attribution (A2) compound; pure reader,
  no schema change. **Touches:** `internal/telemetry` (`bucketforecast.go`:
  `DailyTotalsBy` + `BucketForecasts`, reusing `Forecast` unchanged per bucket),
  `internal/web` (`pages.go` `spendShares`).
- **No ADR** (pure-reader composition pre-blessed by ADR-0019 ⋈ ADR-0018). **Second
  build of epic [0024](issues/0024-epic-convergence-dashboards-cost-surface.md)
  (roadmap v4); issue [0026](issues/0026-bucketed-burn-rate-forecast.md).**

## Tier G — complete the cost surface (small, self-contained)

### G1 / V2 — Settings price-override editor  — **S**  ·  **SHIPPED** (issue [0027](issues/0027-settings-price-override-editor.md))
- **What:** a per-model rate table on the Settings page for
  `config.TelemetryConfig.PriceOverrides` (loaded at startup, applied to the price book,
  but the **only** cost knob with no UI — `settings.go` `renderSettingsForm` omitted it by
  design). Three numeric fields per model; closes the last hand-edit-JSON cost step.
- **Shipped:** a per-model rate table on the Settings page, persisted through `editConfig`
  (rollback-on-invalid, with a new non-negative-rate `config.Validate` hook) and applied
  **live** by rebuilding the price book from `DefaultPriceBook()` + overrides
  (`telemetry.BuildPriceBook`) and `Replace`-ing the shared book in place — repricing the
  account meter and every per-session meter without a restart. No ADR (additive UI;
  CONTRACTS §3/§4 + a REGRESSIONS note). **Third build of epic
  [0024](issues/0024-epic-convergence-dashboards-cost-surface.md) (roadmap v4); issue
  [0027](issues/0027-settings-price-override-editor.md).**

### G2 / V5 — Per-session cost on the Sessions page  — **M**  ·  *candidate*
- **What:** a pure `SessionShares(records)` aggregation (parallel to `AgentShares`) so
  the Sessions picker (today title + age only, `sessions.go`) shows total credits + turn
  count per session — `SpendRecord` already carries `SessionID`. A cost-aware session
  picker.
- **Why now:** the session id is already tagged on every record; pure reader, no schema
  change. **Touches:** `internal/telemetry` (`history.go` `SessionShares`),
  `internal/web` (`sessions.go` `sessionRows`).

### G3 / V9 — Telemetry spend-window selector  — **S**  ·  *candidate*
- **What:** the trend view hardcodes a 14-day window (`pages.go` `spendTrend:249`); add a
  30/90-day selector threaded through `DailyTotals` truncation + re-scale. Users with
  months of history can't see the full picture today. **Touches:** `internal/web`
  (`pages.go` `spendTrend`, `server.go` `handlePage`).

## Tier H — paydown that advances the architecture

### H1 — Generic `telemetry.AppendOnlyStore[T]`  — **M**  ·  *candidate (debt)*
- **What:** extract the duplicated `SpendStore`/`RunStore` machinery (versioned
  envelope, atomic temp-file+rename, missing=empty / invalid=error / empty-dir=ephemeral,
  `Append`/`Records`/`Count`) into one generic store; the two stores become thin typed
  wrappers. Flagged in the B3 review, deferred then as scope creep.
- **Why now:** the duplication is now real (two near-identical files, `history.go` +
  `runs.go`) and a third store (if any future persisted history appears) would triple it;
  paying it down keeps the persistence discipline single-sourced. Pure, dependency-free.
  Best done as a **precursor** to F1/G2 (which add readers, not stores) or standalone.
  **Touches:** `internal/telemetry` (new `store.go` generic; `history.go`/`runs.go`
  rewrap). Needs care: the on-disk JSON tags (`records` vs `runs`) are the stable
  contract and must not change — a refactor-only paydown, guarded by the existing
  round-trip/atomic/migration tests.

## Tier I — small surface polish (pull opportunistically)

- **V3 — surface `SubagentInfo.Description`** (S, orchestration): the SDK populates it
  (`normalize.go:89`) but `renderSubagents` (`render.go:283`) drops it — show it as a
  chip tooltip/subline so concurrent sub-agents during a parallel run say *what* they're
  doing.
- **V10 — keybinding live-apply** (S, polish): TECH_DEBT #13 — a rebind takes effect only
  on the next full page load; an OOB swap of `<body data-keymap>` + the help overlay on
  the Settings POST closes it.

## Tier D — platform / distribution (carried, unchanged)

Paydown, not product — pull when demand appears:
- **Desktop installers** (.dmg/.msi/.deb/AppImage) via `wails3 package` — TECH_DEBT #5.
- **Wails v3 stable migration** when it lands — TECH_DEBT #6 (pinned to `alpha.98`).
- **On-disk `SKILL.md` folder model** — TECH_DEBT #1; deferred pending real demand.
- **First-party embedded MCP server** (zero external runtime) — TECH_DEBT #11. *Note:*
  once C1 (E1) ships, key-requiring stdio servers still need `npx`/`uvx` on PATH — an
  embedded server would be the zero-dependency complement.

---

## Recommended sequencing

1. **C1 / E1 — MCP secrets / Env editor** *(BUILD FIRST)*. Opens the extensibility
   story; M; key decision settled (ADR-0020); claims the reserved 0019/ADR-0020. →
   issue **0019**.
2. **V1 / F1 — run-history aggregations + Runs duration** *(BUILD FIRST)*. The cost ⋈
   orchestration convergence; S; pure readers pre-blessed by ADR-0022; lowest risk. →
   issue **0023**. (Optionally land **H1** `AppendOnlyStore[T]` just before, as a clean
   precursor.)
3. **F2 / F3** — workflow last-run badges, then bucketed forecast: extend the
   convergence once F1's readers exist.
4. **G1 → G2 → G3** — complete the cost surface (price-override editor, per-session cost,
   window selector): small, self-contained, compounding.
5. **I / V3, V10** — opportunistic polish.
6. **Tier D** when distribution demand appears.

Epic **[0022](issues/0022-epic-extensibility-and-convergence.md)** ("extensibility &
convergence") carries C1 (0019) + V1 (0023) as the promoted build-first picks;
everything else stays a candidate here until promoted.

Each item: write the failing test first, keep domain logic pure
(`telemetry`/`ctxforge`/`config` dependency-free), run `make lint && make test`
(coverage floor 65%) + `make e2e` for UI, and fold its ADR/CONTRACTS/REGRESSIONS
updates into the same feature branch (ADR-0004).

---

## Numbering (reconciled 2026-06-06)

Highest on disk before this pass: issues → **0021**, ADRs → **0022** (issue **0019** /
**ADR-0020** were RESERVED for C1 and unused). This pass **claims the reservation**: C1
takes issue **0019** + **ADR-0020**. The v3 epic takes **0022** and the run-aggregations
issue takes **0023** (next free after 0021). No ADR is consumed by the aggregations item
(its decision is pre-blessed by ADR-0022).

---

## Appendix — roadmap v2 (shipped, epic 0013, for context)

| item | feature | ADR | issue |
|------|---------|-----|-------|
| A1 | Ledger-derived budget rows (survive restart) | 0016 | 0014 |
| B1 | Real parallel workflow lanes (per-lane tools/perms) | 0017 | 0015 |
| A2 | Cost attribution — per-agent/workflow/lane rollups | 0018 | 0016 |
| A3 | Budget burn-rate forecast | 0019 | 0017 |
| C2 | Textarea composer (Enter sends, Shift-Enter newline) | — | 0018 |
| B2 | Conditional / branching workflow steps | 0021 | 0020 |
| B3 | Workflow run history (sibling run store + Runs view) | 0022 | 0021 |

Epic 0013 ("deepen the differentiators") — **closed**.

## Appendix — roadmap v1 (shipped, for context)

| item | feature | ADR | issue |
|------|---------|-----|-------|
| 1.2 | Pre-flight turn cost estimate | 0007 | 0002 |
| 1.1 | Budget guardrails (soft warn + hard cap) | 0008 | 0003 |
| 1.3 | Persisted spend history + trends | 0009 | 0004 |
| 2.2 | MCP server management page + curated defaults | 0010 | 0006 |
| 2.1 | Multi-agent workflow run / handoff surface | 0013 | 0010 |
| 3.2 | Per-session telemetry totals | 0011 | 0008 |
| 3.1 | Diff review lane | 0012 | 0009 |
| 3.3 | Keybinding surface | 0014 | 0011 |
| 3.4 | Prompt/snippet library | 0015 | 0012 |

Epics 0001 (cost), 0005 (orchestration), 0007 (polish) — all **closed**.
