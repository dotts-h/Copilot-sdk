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

### G2 / V5 — Per-session cost on the Sessions page  — **M**  ·  **SHIPPED** (epic 0024, issue [0028](issues/0028-per-session-cost-sessions-page.md))
- **What:** a pure `SessionShares(records) []SessionShare{SessionID, Credits, Turns}`
  aggregation (parallel to `AgentShares`/`WorkflowShares`, **excluding** the empty-`SessionID`
  bucket like `WorkflowShares`) so the Sessions picker (was title + age only, `sessions.go`)
  shows total credits + turn count per session — `SpendRecord` already carries `SessionID`.
  A cost-aware session picker.
- **Shipped:** `sessionRows` joins `SessionShares(s.spend.Records())` onto each
  `copilot.SessionMeta` row by id (off the spend store's leaf mutex — no `forgeMu → s.mu`
  inversion), showing *"N turns · X cr"* per row; a no-spend session shows *"no cost yet"*
  (not dropped), a since-deleted bucket is not shown, no spend store → prior shape. The
  turn count rides a new per-group `Count` on `shareBy` (one pass). No ADR (pure-reader
  composition pre-blessed by ADR-0018 ⋈ the `*Shares` pattern; CONTRACTS §3/§4). **Fourth
  build of epic [0024](issues/0024-epic-convergence-dashboards-cost-surface.md) (roadmap
  v4); issue [0028](issues/0028-per-session-cost-sessions-page.md); PR #53.**

### G3 / V9 — Telemetry spend-window selector  — **S**  ·  **SHIPPED** (epic 0024, issue [0029](issues/0029-telemetry-spend-window-selector.md))
- **What:** the trend view hardcoded a 14-day window (`pages.go` `spendTrend`); a
  **14/30/90-day** selector (three buttons, active one marked) threaded through
  `DailyTotals` truncation + re-scale. Users with months of history couldn't see the full
  picture before. **Touches:** `internal/web` (`pages.go` `spendTrend`/`telemetryPartial`/
  `renderPage`, `server.go` `handlePage`, `templates/fragments.html`).
- **Shipped:** `spendTrend(window int)` takes the window; `handlePage` reads `?window=`
  (default 14, clamp to {14,30,90} via `clampWindow`, garbage/out-of-range → 14) and threads
  it through `renderPage` → `telemetryPartial` → `spendTrend`; the `maxUSD` bar-scaling stays
  **after** the window slice (the REGRESSIONS #14 invariant, now asserted per window via
  `TestTelemetryTrendScalesToVisibleMaxPerWindow`). The `telemetryPage` template gained a
  window-selector control that re-fetches `GET /page/telemetry?window=N` into `#main`,
  mirroring the Models-page effort row. No schema change, no new store (a presentation-layer
  slice over the existing pure reader). No ADR. **Fifth and final build of epic
  [0024](issues/0024-epic-convergence-dashboards-cost-surface.md) (roadmap v4) — its last
  child; on merge epic 0024 closes. Issue
  [0029](issues/0029-telemetry-spend-window-selector.md).**

## Tier H — paydown that advances the architecture

### H1 — Generic `telemetry.AppendOnlyStore[T]`  ✅ **shipped** (roadmap v5, epic 0030, issue [0033](issues/0033-generic-append-only-store.md))
- **Built:** the duplicated `SpendStore`/`RunStore` machinery (versioned envelope, atomic
  temp-file+rename, missing=empty / invalid=error / empty-dir=ephemeral,
  `Append`/`Records`/`Count`) collapsed into one generic `telemetry.AppendOnlyStore[T any]`
  (`store.go`); the two stores are now thin `struct{ *AppendOnlyStore[…] }` embeddings that
  preserve their **exact** public API and a **byte-identical** on-disk shape — a hand-built
  `envelope[T]` marshaler reproduces the `{"version":N,"<key>":[…]}` output so `MarshalIndent`
  re-indents it the same. The on-disk JSON tags (`records`/`runs`/`version`) are the unchanged
  stable contract; the spend v1→v2 read needs no migration code (additive tags read back empty).
  Pure, dependency-free. Refactor-only paydown guarded by the existing
  round-trip/atomic/migration/ephemeral tests + a direct generic-store test + on-disk-tag pins
  (`TestSpendStoreOnDiskTagsAreStable` / `TestRunStoreOnDiskTagsAreStable`). Resolves TECH_DEBT
  #14; no new ADR (preserves ADR-0009 / ADR-0022, referenced from CONTRACTS §4). **Third and
  final child of epic 0030 — its merge closed the epic** (V3, V10, H1 all shipped).

## Tier I — small surface polish (pull opportunistically)

- **V3 — surface `SubagentInfo.Description`** ✅ **shipped** (roadmap v5, epic 0030, issue
  [0031](issues/0031-subagent-description-activity-strip.md)): the SDK populated it
  (`normalize.go`) but `renderSubagents` (`render.go`) dropped it — now the `subagentChip`
  surfaces it as a `title=` tooltip (escaped per ADR-0001) so concurrent sub-agents during
  a parallel run say *what* they're doing; an empty description renders the prior chip.
- ~~**V10 — keybinding live-apply** (S, polish)~~ — **shipped** (issue
  [0032](issues/0032-keybinding-live-apply.md), epic 0030): TECH_DEBT #13 — a rebind took
  effect only on the next full page load; the Settings keybinding POST now appends an
  `hx-swap-oob` re-render of the help overlay + an `applyKeymap(…)` script that updates
  `<body data-keymap>` and rebuilds the JS dispatcher's map, so a rebind applies without a
  reload (reads back the persisted keymap so a no-op/rolled-back save can't desync; escaped
  per ADR-0001). Completes the ADR-0014 mechanism (no new ADR); resolves TECH_DEBT #13,
  guards REGRESSIONS #18.

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

## Roadmap v6 — orchestration accountability (code re-read 2026-06-07)

> Fresh pass appended this session. Roadmaps **v1–v5** are shipped and closed (epics
> 0001/0005/0007/0013/0022/0024/0030); their picks are summarized above + in the
> appendices. This section grounds the **next** candidates in the current code, after H1
> collapsed both stores onto one `telemetry.AppendOnlyStore[T]` (epic 0030). The v6 epic
> takes **0031**; its first child takes issue **0034** (next free after 0033). No ADR is
> consumed (the first pick is a pure additive reader + route).

### The teed-up paydown (TECH_DEBT #8) — validated, then superseded

The carried lead-in for this pass was TECH_DEBT #8: now that both stores share one
`AppendOnlyStore[T]`, switch the persistence to an **append-only JSONL log** for O(1)
appends behind the same generic API. The research **rejects** it as the v6 build, on the
evidence:

- **ADR-0009 already considered and rejected JSON Lines**, explicitly, with reasoning
  that still holds: JSONL with `O_APPEND` is genuinely O(1) on disk, *but* it abandons
  the **temp-file+rename atomicity the codebase standardises on** across config / forge /
  spend, and a torn final line needs bespoke recovery. "For this localhost single-user
  tool the per-turn volume is tiny, so the O(n) full rewrite is a non-issue and buys one
  consistent persistence pattern."
- The **#8 trigger is unmet.** Its own row says pay it down "when turn volume or session
  count makes the per-turn rewrite visible" — at one record per turn on a localhost tool,
  the write is sub-millisecond and the rewrite is invisible. Severity *low*, interest
  *low*.
- Reversing a sound, accepted ADR (changing a persisted on-disk contract, adding
  torn-line recovery + a migration) **to fix a non-problem** is negative-value. #8 stays
  a candidate, deferred to its trigger.

So v6 is a **product** epic, not this paydown.

### Where the product is now (v6 framing)

Both differentiators are deep *and* surfaced. The fresh leverage is **not more depth**
within either — it's the **parity gap between the two persisted stores' surfaces.** The
**cost** ledger (`SpendStore`) is a mature accountable surface: a windowed trend,
per-model/agent/workflow shares, a burn-rate forecast, **and a CSV export**
(`WriteCSV` → `/telemetry/export.csv`). Its orchestration sibling, the **run history**
(`RunStore`, ADR-0022), has a Runs view with a per-workflow roll-up + per-lane breakdown
— but **can't be exported**, has **no window selector**, and surfaces only *average*
cost. A run records **skipped** branches that leave no spend record (the reason the store
exists), so it holds data the spend export can't — yet that data can't leave the tool.

### Tier J — bring the Runs surface to cost-surface parity (pure readers / UI composition)

Ranked by value × fit; all are pure readers / presentation-layer compositions over the
**existing v1 run records** — no schema change, no new store.

#### V11 — Runs CSV export — **S** · **BUILD FIRST** · *first child of epic 0031*
- **What:** `telemetry.WriteRunsCSV(w, records)` (the sibling of `WriteCSV`) flattening
  the run history to **one row per lane** (run-level columns repeated) so a branched
  run's **skipped** lane is first-class; a `GET /runs/export.csv` route
  (`handleRunsExport`, sibling of `handleSpendExport`); an "Export CSV" link on the Runs
  page. Columns: `run, workflow, name, mode, startedAt, finishedAt, durationSeconds,
  outcome, lane, agent, status, credits`.
- **Why now:** completes the **accountable** half of the orchestration story — the run
  history becomes exportable like the cost ledger, and the export carries the
  skipped-branch data unique to the run store. Lowest-risk: mirrors the proven, tested
  spend-export pattern end-to-end. Highest value × fit of the parity gaps.
- **Touches:** `internal/telemetry` (`runs.go`: `WriteRunsCSV`, `csvTime`),
  `internal/web` (`pages.go` `handleRunsExport`, `hub.go` route,
  `templates/fragments.html` `runsPage` link).
- **No ADR** (pure additive reader + route, pre-blessed by the ADR-0009 export
  precedent). **Issue [0034](issues/0034-runs-csv-export.md); epic
  [0031](issues/0031-epic-orchestration-accountability.md).**

#### V12 — Runs time-window selector — **S** · candidate
- **What:** mirror the Telemetry trend's 14/30/90-day selector on the Runs page,
  threading a clamped `?window=` (reuse `clampWindow`) so a long run history can be
  sliced. **Touches:** `internal/web` (`runs.go` `runsPartial`, `handlePage`,
  `templates`). Presentation-layer slice; **no schema change, no ADR.**

#### V13 — Total cost on the per-workflow Runs summary — **S** · candidate
- **What:** the Runs summary table shows `AvgCredits` but not `TotalCredits` (already on
  `RunAggregate`); add the column so a workflow's *cumulative* orchestrated spend reads
  beside its average. **Touches:** `internal/web` (`runs.go` `runSummaryRow`,
  `templates`). **No schema change, no ADR.**

#### V14 — Per-lane cost roll-up — **M** · candidate
- **What:** a `LaneShares`-style pure reader keyed by (workflow, lane) over the run
  history (or the `WorkflowID`+`LaneIndex`-tagged spend records, ADR-0018), surfacing
  *"which lane in a workflow costs / fails most?"* — the finest orchestration-attribution
  grain, currently computed nowhere. **Touches:** `internal/telemetry` (new reader),
  `internal/web` (Telemetry or Runs section). **No schema change**; an ADR only if it
  introduces a cross-package seam (it shouldn't).

### Recommended sequencing (v6)

1. **V11 — Runs CSV export** *(BUILD FIRST)*. Closes the export parity gap; S; pure
   reader + route; mirrors the spend export. → issue **0034**, epic **0031**.
2. **V13 → V12** — total-cost column, then the window selector: smallest parity gaps,
   compounding, both presentation-layer.
3. **V14** — per-lane roll-up: the one new *analytical* reader, once the cheaper parity
   gaps are closed.
4. **TECH_DEBT #8** only when its volume trigger actually fires.

> **v6 update (after V13):** **V11 shipped** (PR #59, issue 0034) and **V13 shipped**
> (PR #61, issue 0035) — the per-workflow Runs summary now surfaces `RunAggregate.TotalCredits`
> as a "Total cost" column beside the average (a pure presentation-layer slice, no
> telemetry/schema change, no ADR). **Remaining, re-ranked:** **V12 — Runs time-window
> selector** (S, next up: thread a clamped `?window=` through `runsPartial` like
> `spendTrend`, reusing `clampWindow`) → then **V14 — per-lane cost roll-up** (M, the
> one new analytical reader). TECH_DEBT #8 stays deferred to its (unmet) volume trigger.

> **v6 update (after V12):** **V12 shipped** (PR #62, issue 0036) — the Runs page now
> carries the same 14/30/90-day window selector as the Telemetry trend. A clamped
> `?window=` is threaded `handlePage → renderPage → runsPartial` (reusing `clampWindow`,
> no re-implementation), and a pure `windowRuns` slices the run history to the records
> within `window` days of the **most recent run** (tail-relative like `spendTrend`, so a
> long-idle history still shows its latest window) **before** both the per-workflow
> summary roll-up and the history list — an out-of-window run is dropped from both. A
> presentation-layer slice over the existing v1 run records: no telemetry/schema change,
> no new store, no ADR. **Remaining:** **V14 — per-lane cost roll-up** (M, the one new
> analytical reader keyed by (workflow, lane)) is the last open child of epic 0031;
> TECH_DEBT #8 stays deferred to its (unmet) volume trigger. On V14's merge epic 0031
> closes — then scope epic 0032 from a fresh value×fit pass.

> **v6 update (after V14):** **V14 shipped** (PR #64, issue 0037) — `telemetry.LaneShares`
> rolls the run history up **per (workflow, lane)** to `LaneShare{WorkflowID, LaneIndex,
> AgentID, Runs, Failures, Credits, Fraction}` (a skipped lane adds zero cost, a failed
> lane counts as a failure), sorted by credits descending (ties → workflow id asc, then
> lane index asc — a deterministic total order); the Runs page renders a **"Cost by
> lane"** share list below the per-workflow summary, resolving ids → labels under
> `forgeMu`. The per-lane cousin of `RunAggregates` — the finest
> orchestration-attribution grain — a pure telemetry reader (returns ids; the web layer
> resolves labels), **no schema change, no new store, no ADR**. **Epic 0031 is now
> EXHAUSTED — all four children (V11/V13/V12/V14) shipped — and is CLOSED:** the Runs /
> orchestration surface has reached cost-surface parity (windowed, exportable, total &
> per-workflow & **per-lane** roll-ups). TECH_DEBT #8 stays deferred to its (unmet)
> volume trigger.
>
> **→ Next: epic 0032 (roadmap v7).** With **both** persisted surfaces now mature *and*
> at parity, a fresh value×fit pass finds the leverage is no longer *within* either
> surface but in **converging them** — the cost ledger (`SpendStore`) and the run history
> (`RunStore`) are still **two separate stores answering overlapping questions** (a
> workflow's spend lives in *both*: as `WorkflowShares` over metered turns and as
> `RunAggregates.TotalCredits` over recorded runs), reconciled **nowhere**. The two
> figures can **diverge** (a turn metered outside a recorded run; a run whose lanes
> metered under a different attribution) and a user has no way to see — or trust — that
> they agree. **Epic 0032 — cost⋈run reconciliation:** a pure cross-store reader that
> joins the two roll-ups per workflow and surfaces the **delta** (ledger spend vs.
> recorded-run spend), so orchestrated spend is not just *accountable* on each surface but
> *reconcilable* across them — the natural convergence of the now-mature cost +
> orchestration surfaces. First child: a `telemetry.WorkflowReconcile`-style reader over
> *both* record slices, surfaced on the Telemetry or Runs page as a per-workflow
> ledger-vs-runs comparison. (A demand-gated Tier-D pick supersedes this only if concrete
> demand has appeared; none has, so the convergence pick leads.)

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
