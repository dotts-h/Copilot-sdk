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

## Roadmap v7 — cost⋈run reconciliation (code re-read 2026-06-07)

> Fresh pass appended this session. Roadmaps **v1–v6** are shipped and closed (epics
> 0001/0005/0007/0013/0022/0024/0030/0031); their picks are summarized above + in the
> appendices. This section grounds the **next** candidates after epic 0031 brought the
> Runs / orchestration surface to **cost-surface parity** (windowed, exportable, total &
> per-workflow & per-lane roll-ups). The v7 epic takes **0038**; its first child takes
> issue **0039** (next free after 0037). No ADR is consumed (the first pick is a pure
> cross-record reader returning ids — no cross-package seam).

### Where the product is now (v7 framing)

Both differentiators are deep, surfaced, *and* — after v6 — **at parity** across their two
persisted surfaces. The fresh leverage is no longer *within* either surface but in
**converging them.** The cost ledger (`SpendStore`) and the run history (`RunStore`) are
still **two separate stores answering overlapping questions**: a workflow's spend lives in
**both** — as `telemetry.WorkflowShares` over metered turns **and** as
`telemetry.RunAggregates.TotalCredits` over recorded runs — **reconciled nowhere**. The two
figures can **diverge** (a turn metered outside a recorded run; a run whose lanes metered
under a different attribution) and a user has no way to see — or trust — that they agree. So
orchestrated spend is *accountable* on each surface but not *reconcilable* across them. The
convergence is one pure cross-store reader away.

### Tier K — converge the two stores (pure cross-record readers / UI composition)

Ranked by value × fit; all are pure cross-record readers / presentation-layer
compositions over the **existing v1/v2 records** — no schema change, no new store. The
reader takes two record slices and returns ids; the web layer resolves labels under
`forgeMu` — **no cross-package seam, no ADR**.

#### V15 — cost⋈run reconciliation reader + Telemetry "Ledger vs runs" — **M** · **BUILD FIRST** · *first child of epic 0038*
- **What:** `telemetry.WorkflowReconcile(spend []SpendRecord, runs []RunRecord)
  []WorkflowRecon{WorkflowID, LedgerCredits, RunCredits, Delta}` joins the two roll-ups
  per workflow — `LedgerCredits` from workflow-attributed spend (the `WorkflowShares`
  grain, chat bucket excluded), `RunCredits` from each workflow's recorded runs
  (`RunRecord.Credits`, the `RunAggregates.TotalCredits` grain), `Delta = LedgerCredits −
  RunCredits` — sorted by **absolute delta descending** (the biggest discrepancy first;
  ties → ledger credits desc, then workflow id asc, a total deterministic order). A
  workflow present in one store but not the other appears with the other side zero. The
  Telemetry page renders a **"Ledger vs runs"** per-workflow comparison table below "Cost
  by workflow", resolving ids → labels under `forgeMu` and **ambering** a non-trivial
  delta.
- **Why now:** with both persisted surfaces mature *and* at parity, the next leverage is
  converging them; this is the lowest-risk first step — a pure cross-record reader
  mirroring the `*Shares` / `RunAggregates` patterns, no schema change. Makes orchestrated
  spend *reconcilable*, not just accountable. Highest value × fit of the convergence gaps.
- **Touches:** `internal/telemetry` (`reconcile.go`: `WorkflowReconcile`, `WorkflowRecon`),
  `internal/web` (`pages.go` `workflowReconcile`/`reconcileRow`, `telemetryPartial`,
  `templates/fragments.html` `telemetryPage`, `static/app.css` `.recon`).
- **No ADR** (pure cross-record reader returning ids, no cross-package seam — pre-blessed
  by the same convergence rationale as ADR-0022 / the `*Shares` readers). **Issue
  [0039](issues/0039-cost-run-reconciliation.md); epic
  [0038](issues/0038-epic-cost-run-reconciliation.md).**

#### V16 — per-lane / per-session reconciliation — **M** · candidate
- **What:** extend the join below the per-workflow grain — reconcile the ledger's
  `(workflow, lane)`-tagged spend (ADR-0018) against `LaneShares` over the run history, or
  reconcile per copilot session (`SessionShares`) where a session spans recorded runs — so
  a divergence is locatable at the finer grain, not just the workflow total. **Touches:**
  `internal/telemetry` (a finer-grain reconcile reader), `internal/web` (Telemetry or Runs
  section). **No schema change**; an ADR only if a cross-package seam appears (it
  shouldn't).

#### V17 — surface the reconciliation delta in the export / forecast — **S** · candidate
- **What:** carry the per-workflow `Delta` into the CSV export (a reconciliation column or
  a sibling `WriteReconcileCSV`) so the divergence leaves the tool like the spend/run
  ledgers do, and/or annotate the burn-rate forecast when the ledger and run sides
  disagree materially. **Touches:** `internal/telemetry`, `internal/web`. Presentation /
  pure-reader; **no schema change, no ADR.**

### Recommended sequencing (v7)

1. **V15 — cost⋈run reconciliation reader + Telemetry "Ledger vs runs"** *(BUILD FIRST)*.
   Opens the convergence story; M; pure cross-record reader + UI composition; mirrors the
   `*Shares` / `RunAggregates` patterns. → issue **0039**, epic **0038**.
2. **V16 → V17** — finer-grain reconciliation (per-lane / per-session), then surface the
   delta in the export / forecast: once the per-workflow join exists, extend the grain and
   the reach.
3. **TECH_DEBT #8** only when its volume trigger actually fires.

> **v7 update (after V15):** **V15 shipped** (PR #66, issue 0039) — `telemetry.WorkflowReconcile(spend
> []SpendRecord, runs []RunRecord) []WorkflowRecon{WorkflowID, LedgerCredits, RunCredits, Delta}`
> joins the two persisted stores' per-workflow roll-ups (the `WorkflowShares` ledger grain vs
> the `RunAggregates.TotalCredits` run grain) and surfaces the **delta**, sorted by absolute
> delta descending (a workflow present in one store but not the other appears with the other
> side zero). The Telemetry page renders a **"Ledger vs runs"** comparison table below "Cost by
> workflow", resolving ids → labels under `forgeMu` and **ambering** a non-trivial delta — so
> orchestrated spend is *reconcilable* across the two stores, not just *accountable* on each. A
> pure cross-record reader returning ids (no schema change, no new store, no cross-package seam,
> no ADR). **Epic 0038 stays OPEN** — V15 is its first child. **Remaining, re-ranked:** **V16 —
> per-lane / per-session reconciliation** (M, the finer-grain join, the natural next child) →
> then **V17 — surface the delta in the export / forecast** (S). TECH_DEBT #8 stays deferred to
> its (unmet) volume trigger.

> **v7 update (after V16):** **V16 shipped** (issue 0040) — `telemetry.LaneReconcile(spend
> []SpendRecord, runs []RunRecord) []LaneRecon{WorkflowID, LaneIndex, LedgerCredits, RunCredits,
> Delta}` joins the **same** two persisted stores one grain **finer** — per `(workflow, lane)` —
> so a divergence the per-workflow row only totals is locatable at the exact step. Ledger side
> groups lane-tagged spend (`SpendRecord` by `WorkflowID + LaneIndex`, ADR-0018) and run side
> sums per-lane credits (`RunLane.Credits`, the `LaneShares` grain), keyed by `(workflow, lane)`,
> sorted by absolute delta descending (ties → ledger credits desc, then workflow id asc, then
> lane index asc; a lane zero on both sides — a skipped run lane with no ledger spend — is
> omitted). The Telemetry page renders a **"Ledger vs runs by lane"** table below the
> per-workflow "Ledger vs runs", resolving ids → labels under `forgeMu` and ambering a
> non-trivial delta. A pure cross-record reader returning ids (no schema change, no new store,
> no cross-package seam, no ADR). **Epic 0038 stays OPEN** — V16 is its second child. **A
> per-SESSION reconciliation was considered and is not well-supported** (`RunRecord` carries no
> session id, unlike `SpendRecord.SessionID`, so there's no key to join runs on per session) —
> the per-lane join is the natural finer grain. **Remaining:** **V17 — surface the delta in the
> export / forecast** (S, the last convergence slice — carry the per-workflow/per-lane `Delta`
> into the CSV export and/or annotate the burn-rate forecast when the two sides disagree
> materially) → then **close epic 0038** (the reconciliation surface exhausted at the workflow +
> lane grain). TECH_DEBT #8 stays deferred to its (unmet) volume trigger.

> **v7 update (after V17) — roadmap v7 CLOSED, epic 0038 CLOSED:** **V17 shipped** (issue 0041) —
> `telemetry.WriteReconcileCSV(w io.Writer, spend []SpendRecord, runs []RunRecord) error` serializes
> the cross-store reconciliation to CSV — the **export sibling** of `WriteCSV` (spend) and
> `WriteRunsCSV` (runs) — so the ledger-vs-runs **divergence leaves the tool** the way spend and runs
> already do. One file carries **both grains**: the per-workflow rows (`WorkflowReconcile`, V15) first,
> then the per-`(workflow, lane)` rows (`LaneReconcile`, V16), each labelled by a leading `grain` column
> (`workflow` | `lane`) so a consumer never double-counts a total against its breakdown — header
> `grain,workflow,lane,ledgerCredits,runCredits,delta`, the readers' own deterministic order
> (biggest |delta| first within each grain), header-only on empty/chat-only input. Streamed by a new
> `GET /telemetry/reconcile.csv` route (mirroring `handleSpendExport`/`handleRunsExport`), surfaced as
> a "Export CSV" link beside the "Ledger vs runs" heading (a DISJOINT `reconcile-export` marker class so
> it can't collide with the spend export's `a.export` selector — the V16 strict-mode lesson). A pure
> writer (the `io.Writer` the caller owns is the only IO; no schema change, no cross-package seam, **no
> ADR** — pre-blessed by the ADR-0009 export precedent). **The forecast-annotation alternative was
> weighed and dropped** as an altitude mismatch — the burn-rate forecast answers *"when does the budget
> run out"*, not *"do the two stores agree"*; bolting a reconciliation note onto it would mix two
> concerns. **Epic 0038 CLOSES** — the reconciliation surface is **exhausted** from a fresh value×fit
> pass: orchestrated spend is now reconcilable at the **workflow** grain (V15) and the **lane** grain
> (V16) on-page, and **exportable** (V17) for outside-the-tool analysis; the per-session grain is
> unsupported (`RunRecord` carries no session id). **Roadmap v7 is done.** TECH_DEBT #8 stays deferred
> to its (still-unmet) volume trigger. **→ Next: roadmap v8** — scope a fresh epic from a value×fit pass
> against the two differentiators (cost-awareness ⋈ orchestration).

---

## Roadmap v8 — interactive orchestration (code re-read 2026-06-08)

> Fresh pass appended this session. Roadmaps **v1–v7** are shipped and closed (epics
> 0001/0005/0007/0013/0022/0024/0030/0031/0038); their picks are summarized above + in the
> appendices. This section grounds the **next** candidates after epic 0038 exhausted the
> cost⋈run **reconciliation** surface (workflow + lane grain, on-page and exportable). The
> v8 epic takes **0042**; its first child takes issue **0043** (next free after 0041), and
> — being the first **action** child — consumes **ADR-0023** (the v7 reader epic consumed
> none).

### Where the product is now (v8 framing)

The product can now **meter**, **attribute** (agent/workflow/lane/session), **forecast**,
**budget-cap**, run workflows (sequential/parallel/branching), and — after v6/v7 — fully
**observe** the orchestration surface: run history, per-workflow + per-lane aggregates,
ledger⋈runs reconciliation, and CSV export of all three. Re-reading against the two
differentiators, the leverage is no longer *more depth* or *more readers* (v4–v7 were all
pure-reader + presentation cadence, and both surfaces are mature). The standout gap is that
**the orchestration surface is entirely READ-ONLY**: a user can *see* a run failed or cost
too much but cannot **ACT** on it — every control (the sole run-trigger,
`POST /workflows/{id}/run`) lives only on the Workflows page; the rich Runs / Telemetry
history has no action at all. (Verified against the code: slash commands, the MCP
secrets/Env editor, the price-override editor, and keybinding live-apply are **shipped** —
not re-proposed.) So v8's theme is **the orchestration surface goes interactive.**

### Tier L — make the orchestration surface actionable

Ranked by value × fit. Unlike v4–v7, the lead pick is an **action with side effects** (it
spawns live orchestration via the `copilot.Client` seam), so it takes an ADR.

#### V18 — rerun a recorded run from the Runs page — **M** · **BUILD FIRST** · *first child of epic 0042*
- **What:** a `↻ rerun` control on each recorded run (Runs page) that re-executes that
  run's workflow — looked up by `WorkflowID` and run as its **current** definition (a
  re-execution, *not* a historical replay; `RunRecord` carries no step definitions) —
  through a single shared `launchWorkflow` trigger extracted from `handleWorkflowRun`. The
  new run carries the **same `WorkflowID`**, so its spend rolls up under the same
  per-workflow totals / aggregates / reconciliation (coherent with V13/V15). Gated on the
  workflow still existing (`CanRerun` — an orphan run shows no control) and refused while
  the server is busy; lands the user on the Chat page where the lanes stream.
- **Why now:** the orchestration surface has been read-only through all of v4–v7; this is
  the first action on it — the highest-value gap — at lower risk than feared because the
  run-trigger action **already exists** (rerun is a second entry point to it, not a new
  mechanism). Highest value × fit of the interactive candidates.
- **Touches:** `internal/web` (`workflow.go` `launchWorkflow`/`handleRunRerun`, `runs.go`
  `runRow` `CanRerun`, `hub.go` route, `templates/fragments.html` `runRecord` button,
  `static/app.css` `.rerun`).
- **Takes [ADR-0023](adr/0023-rerun-a-recorded-run-re-executes-the-current-workflow-definition.md)**
  (written first, ADR-0004) — the first **action** child: the rerun semantics
  (re-execute current definition, not replay) + the shared trigger seam (no new runtime
  seam). **Issue [0043](issues/0043-rerun-workflow-from-runs-page.md); epic
  [0042](issues/0042-epic-interactive-orchestration.md).**

#### V19 — abort / cancel an in-flight run from the surface — **M** · candidate
- **What:** the dual of rerun — a control to **stop** a running workflow (the seam already
  has `Abort`; `handleAbort` aborts the chat turn but not a multi-lane run cleanly). Lets a
  user halt a run that's clearly going wrong (cost runaway, wrong path). **Touches:**
  `internal/web` (`workflow.go` run-abort path, the lanes panel). An action child — likely
  reuses the V18 trigger/seam discipline; ADR only if a genuinely new seam appears.

#### V20 — rerun a single failed lane — **L** · candidate
- **What:** finer than V18 — re-execute just the failed lane of a run rather than the whole
  workflow. Higher value for long workflows, but needs care (a lane's input is its
  predecessor's handoff output, which a historical record may not fully carry). **Touches:**
  `internal/web`, possibly `internal/telemetry` (lane lineage). An action child; likely an
  ADR for the partial-rerun semantics.

#### B (deferred from this pass) — cost-spike / anomaly reader — **M** · candidate
- **What:** a pure `DetectAnomalies` reader (trailing burn-rate shift > N%, or a workflow's
  per-turn cost jump > M%) ambered on the Telemetry page — the **active** cost surface
  (passive cost-awareness predicts/warns/caps but never flags anomalies). Mirrors the
  V15/V16/F3 pure-reader + presentation pattern exactly (no ADR, pre-blessed by ADR-0019's
  forecast math). **Deferred** behind V18: lower marginal value (the cost surface is
  already deep) than opening the interactive theme, but the natural pick if the interactive
  surface is judged exhausted. **Touches:** `internal/telemetry` (`forecast.go`/`history.go`
  readers), `internal/web` (`telemetry_render.go`).

### Recommended sequencing (v8)

1. **V18 — rerun from the Runs page** *(BUILD FIRST)*. Opens the interactive theme; M;
   reuses the existing run-trigger behind one shared `launchWorkflow`; takes ADR-0023. →
   issue **0043**, epic **0042**.
2. **V19 → V20** — abort an in-flight run, then per-lane rerun: extend the interactive
   surface once the rerun trigger discipline exists.
3. **B (anomaly reader)** if the interactive surface is exhausted — a pure reader, no ADR.
4. **TECH_DEBT #8** only when its volume trigger actually fires.

### v8 update (after V19) — second child shipped

> Appended after V19. **V18/0043 (rerun from the Runs page, PR #76, ADR-0023) shipped +
> merged** — the first action on the orchestration surface. Epic 0042 stayed **open**; a
> fresh value×fit pass for the **second** child re-read the code against the two
> differentiators (cost-awareness ⋈ orchestration).

**The pass.** The interactive theme V18 opened is **not** exhausted: rerun is *start
again*; the obvious gaps were **stop** (V19) and **finer-grain re-execution** (V20). Of the
candidates, **V19 (abort an in-flight run)** ranked highest on value × fit — it is the
**dual of rerun** and **completes the basic interactive control set** (start → rerun →
stop), and it **reuses an existing seam**: `copilot.Client.Abort` is already on the seam
(driven by the chat-turn `handleAbort`) and recorded by `MockClient`, so the only new work
is aborting a *multi-lane run* cleanly. **V20 (per-lane rerun)** is higher-grain but harder
(a historical `RunRecord` doesn't carry a lane's predecessor handoff input) and reads as
**L** with a likely ADR — correctly **teed behind V19**. **B (cost-anomaly reader)** stays
a strong low-risk alternate but the interactive surface still has the obvious *stop* gap, so
it stays deferred.

**Chosen second child: V19 — abort an in-flight run** (issue **0044**, epic **0042**,
**ADR-0024**). A `⏹ stop run` control on the Chat lanes panel stops the running workflow:
each still-running lane's backing session is aborted over `Abort`, the unsettled lanes
settle **failed** (detail `⏹ aborted`) and the run records as a **failed** outcome — *a
stopped run is a failed run*, **no new lane status / schema change**, so it rolls up under
the same aggregates / reconciliation as any failed run. It took **ADR-0024** (an *action*
child, like V18) for three decisions: reuse `failed` (vs. a new terminal status), reuse the
`Abort` seam per-lane (vs. a bespoke cancel path), and **make the single completion path
`runFrags` idempotent** (`run.recorded`) — because `laneError` is called from a lane
goroutine that **bypasses** the reducer's `!s.run.done` guard, so a lane that errors *after*
the abort already settled the run would otherwise **double-record** it. New route `POST
/run/abort`; the `stop-run` class is **disjoint** from `.abort` / `button.run` /
`button.rerun` / `a.export`.

**Re-ranked remainder (after V19).** 1) **V20 — per-lane rerun** (L, likely an ADR for the
partial-rerun semantics + lane lineage) — the next interactive child if epic 0042 stays
open. 2) **B — cost-anomaly reader** (M, pure reader, no ADR) — the pivot if the interactive
surface is judged exhausted. 3) **TECH_DEBT #8** only when its volume trigger fires. The
basic control set (start → rerun → stop) is now complete; whether the interactive surface is
"exhausted" or has one more high-value child (V20) is the **next session's** fresh-pass call.

---

## Roadmap v9 — UI/UX refresh (presentation pass; code re-read 2026-06-08)

> Appended after epic 0045 was scoped + driven to completion. Roadmaps **v1–v8** are shipped
> (epics 0001/0005/0007/0013/0022/0024/0030/0031/0038/0042); their picks are summarized above +
> in the appendices. v9 is the first **presentation** epic — every prior roadmap deepened or
> surfaced the two differentiators (cost-awareness ⋈ orchestration); a dedicated UX/front-end
> research pass found the standout gap was no longer a missing reader or action but the
> **presentation layer** itself. The epic took **0045**; its children took issues **0046–0049**
> and **ADR-0025–0028**.

### Where the product was (v9 framing)

The *functional* surface (v4–v8) was mature: meter, attribute, forecast, budget-cap, run
workflows, fully observe + reconcile + export the orchestration surface, and (v8) act on it. The
research re-read the web UI against modern front-end practice and found three presentation gaps:
**no theme system** (dark-only, raw color literals blocking a light theme, a carried WCAG-AA
contrast shortfall), **navigation overload** (~13 flat top-bar items, past where a top bar scans),
and **a flat telemetry surface** (plain tables where the data could read like a BI report). The
hard constraints — **no build chain, single committed CSS file, htmx + server templates, minimal
JS / no framework** — were confirmed *not* limiting: modern vanilla CSS (`light-dark()`,
custom-property tokens, View Transitions) + server-rendered inline SVG cover theming, charting, and
polish with no framework and no build step.

### Tier M — refresh the presentation layer (CSS + templates, no build chain)

All four children shipped; **on V24's merge epic 0045 is exhausted.**

#### V21 — design-token foundation + light/dark theme — **M** · **SHIPPED** (issue 0046, ADR-0025, PR #44)
- A semantic design-token layer expressing both palettes via `light-dark()` keyed on
  `color-scheme`; an OS-default, persisted, no-FOUC theme toggle; a palette retune that **deleted**
  the destructive-control contrast allowlist so the axe scan runs over **both** themes; a global
  `:focus-visible` ring + `prefers-reduced-motion` reset. The foundation every later child builds on.

#### V22 — navigation → grouped sidebar + ⌘K command palette — **M/L** · **SHIPPED** (issue 0047, ADR-0026, PR #79)
- The 13-item top bar → a left sidebar grouping pages into *Primary · Build · Observe · Config ·
  Help* (config/help pinned to the bottom, progressive disclosure) + a ⌘/Ctrl-K command palette
  (minimal vanilla JS reusing the existing keymap dispatch) so grouping never blocks a power user.

#### V23 — telemetry dashboard: KPI cards + server-rendered SVG sparklines — **L** · **SHIPPED** (issue 0048, ADR-0027, PR #81)
- A top row of big-number KPI cards (each a period-over-period Δ + a sparkline, per-metric
  higher-is-worse coloring), a cumulative trend band (+ dashed burn-rate forecast), and a
  spend-vs-budget bullet — **server-rendered inline `<svg>` from pure Go builders**, zero JS, no
  charting library, re-rendering through the existing `?window=` swap. Bonus: REGRESSIONS #20 (a
  latent light-theme contrast bug on the chat elicit form, surfaced by the both-theme axe scan).

#### V24 — motion & polish: View-Transition page swaps + component pass — **S/M** · **SHIPPED** (issue 0049, ADR-0028)
- Opts the sidebar nav links into the browser View Transitions API with per-swap `transition:true`
  (one `{{range .Nav}}` loop; the ⌘K palette inherits it), scoped to `#main` with a
  `view-transition-name` so navigation cross-fades (degrades to instant); an explicit
  `::view-transition-*` guard silences it under `prefers-reduced-motion`. **`globalViewTransitions`
  was tried and rejected** — it wraps the `hx-swap-oob` streaming updates a `/send`/run response
  pushes mid-stream and **dropped run/turn completion swaps** (REGRESSIONS); per-nav opt-in touches no
  streaming swap. A settle-aware `navTo` (waits for `htmx:afterSettle`) keeps the now-async nav
  deterministic. A token-driven component pass (new `--speed`/`--ease`/`--shadow`/`--shadow-lg`
  tokens, eased interactive controls, a 1px button press, resting card elevation) that changes no
  color pairing, so the both-theme axe scan is unaffected. **No build step, no framework, no new JS,
  no route, no schema.** The deferred **Open Props** primitives + CSS **`@layer`** stay
  deferred-additive (a conscious trade-off in ADR-0028). **Last child — its merge closes epic 0045.**

### Recommended sequencing (v9) — done

V21 (foundation) → V22 (IA) → V23 (dashboard) → V24 (motion/polish), each born in its PR, the epic
re-ranked on each merge. All four shipped.

> **→ Next: roadmap v10 (scoped below).** A real-world audit (2026-06-08) surfaced that our meter
> has drifted from **GitHub Copilot's June-2026 usage-based token billing**, *and* that the product can
> run on **autopilot with no governance layer** to make it safe — so v10 has **two co-lead pillars**:
> **billing fidelity** (cost-awareness correctness) and **safe autopilot** (a tool-governance policy +
> hooks), plus an **auth/connection** surface and the carried V20 / B / Open-Props paydown. A
> **dedicated tech-debt + code-quality + architecture audit** follows the pricing work. **TECH_DEBT #8**
> stays deferred to its (unmet) volume trigger.

---

## Roadmap v10 — billing fidelity + connection (code + live-billing re-read 2026-06-08)

> **Trigger.** Two findings from using the shipped UI: (1) the Telemetry **"per-model breakdown"
> reads empty next to "11.20 cr spent"** — three deliberately-separate sources (ledger / month-to-date
> / **live in-process meter**), and the demo seeds the ledger but never the live meter, so the
> token table reads zero until a real turn streams; the ledger records *do* carry the token counts,
> so the table can be computed from history instead. (2) **Copilot moved to usage-based token billing
> (2026-06-01)** — input/output/cached priced per model rate → AI Credits (1 cr = $0.01, our model
> exactly), **plus a billed cache-write cost (Anthropic ~1.25× input) and reasoning tokens billed at
> the output rate** — *neither of which we price* (both sit in display-only `ExtraTokens`,
> REGRESSIONS #3). Our meter therefore **under-counts real spend**. This is a correctness gap in the
> product's core differentiator. Live sources exist to keep us honest:
> `models.github.ai/catalog/models` (per-model multipliers) and `/rest/billing/usage` (GitHub's
> authoritative billed `usageItems` with `pricePerUnit`), and the SDK already reports per-turn
> `ReportedAIU`. — sources in the v10 research log (issue 0050).

**Consistency design (the spine).** Don't try to *replicate* GitHub's billing perfectly offline — that
drifts on every multiplier change. Instead, a **three-tier source hierarchy**:
1. **Per-turn truth** = the SDK's **`ReportedAIU`** (GitHub's authoritative cost, already captured) —
   no network. The static price book is demoted to an **estimate** (pre-flight composer + forecast),
   never the source of truth for *actual* spend.
2. **Rate freshness (optional)** = poll `catalog/models` for current multipliers, **cached + fail-open
   to the static book**, so a stale hard-coded rate self-heals without breaking the offline-single-
   binary doctrine (no CDN dependency — the fetch is opt-in, cached, and degradable).
3. **Reconciliation (optional)** = pull `/rest/billing/usage` to reconcile our ledger against GitHub's
   billed records — the cost cousin of the V15/V16 ledger⋈runs reconciliation.

### Epic — Billing fidelity (cost-awareness) → issue [0050](issues/0050-epic-billing-fidelity.md)

- **P0 · Authoritative-cost-first metering** — **M** · ADR. Make `ReportedAIU` the source of truth for
  *actual* turn spend when present; keep the price book as the *estimate* for pre-flight/forecast and
  the offline fallback. Surface estimate-vs-reported so drift is visible. Re-frames `Meter`/`SpendRecord`
  around "estimated vs reported." *The answer to "how do we stay consistent."*
- **P1 · Price cache-write + reasoning tokens** — **L** · ADR (money math + price-book migration).
  Add `CacheWritePerMTok` + reasoning pricing to `ModelRate`; promote the cache-write + reasoning
  counts out of display-only `ExtraTokens` into priced `Usage`; show them in the statusline split and
  the per-model breakdown. **Default rule (confirmed): cache-write = 1.25× input, reasoning = output
  rate**, overridable per-model via the existing Settings price editor (G1). Table-tested; the price
  book stays deterministic.
- **P2 · Per-model breakdown from the ledger** — **M** · no ADR. Compute the per-model token table
  (in / cached / **cache-write** / out / **reasoning** + credits/usd) from the **persisted ledger**
  (records already carry the counts) so it's populated and restart-surviving; relabel the live-meter
  table "this session" vs the ledger table "all-time." **Closes the empty-table finding.** Pure reader
  + render; adds the integration coverage that was missing.
- **P3 · Estimate-vs-reported reconciliation + drift** — **M** · no ADR. A Telemetry row joining our
  computed credits to `ReportedAIU` (and optionally `/billing/usage`), ambered past an epsilon — the
  cost-side cousin of ledger⋈runs reconciliation (V15).
- **P4 · Live price-book refresh (optional, opt-in)** — **L** · ADR (network in an offline-first tool).
  Fetch current per-model multipliers from `catalog/models` on a cadence, cached to disk, **fail-open**
  to the static `DefaultPriceBook`. A spike first: confirm the catalog payload shape + auth/network
  policy. Strictly additive — the binary still runs fully offline.

### Epic — Safe autopilot: tool-governance policy + hooks (the third pillar) → issue [0052](issues/0052-epic-safe-autopilot-governance.md)

> **Why (maybe the highest-value pillar).** The product can run agents on **autopilot** (auto mode) and
> orchestrate multi-lane workflows — but there is **no governance layer** that makes that *safe*. Today
> `permissionHandler` (`internal/copilot/handlers.go:17`) **always** blocks for an interactive decision;
> there is only a flat `AutoApproveTools` allowlist (`sdkclient.go:144`) and **no hooks at all** (the
> repo's only "hooks" are the CI workflow-guard scripts). So a user either click-approves everything or
> turns approvals off — neither is safe. Industry practice (Claude Code's allow/deny/ask + PreToolUse
> hooks + auto-mode risk classifier) is the model: **auto-approve read-only ops, hard-deny destructive
> patterns, and force a mandatory human-in-the-loop gate for the risky-but-legitimate** — enforced in
> the bridge, not just config (deny-rules-alone have known bypass bugs; combine with a hook).

- **G0 · Policy model + seam** — **M** · ADR. A forge-backed **permission policy** of `allow / deny /
  ask` rules, matched on tool **kind** (read/write/shell/MCP) and **bash patterns**, evaluated **inside
  `permissionHandler` before the gate emits**: allow → auto-approve, deny → `PermissionDecisionReject`
  with a reason, ask → the existing HITL gate. Generalizes the flat `AutoApproveTools`. Deny wins.
- **G1 · Default safe policy (auto-approve reads)** — **M**. Ship a **safe-by-default** policy: read-only
  tools (file read, search, navigation, plan transitions) auto-approved; writes/exec fall to the gate.
  *The "pre-permission with auto-approve read stuff" — the default build is safe out of the box.*
- **G2 · Dangerous-action deny + mandatory HITL** — **M/L** · ADR. A built-in **deny/gate** ruleset for
  destructive patterns — `rm -rf` on `$HOME`/root, `curl|sh` / pipe-a-download-into-an-editor or shell,
  writes outside the workspace, `sudo`, secret exfiltration — **hard-denied or forced through a
  mandatory gate even in auto mode** (unbypassable, enforced in the bridge). The "security stuff that
  makes autopilot safe."
- **G3 · Hooks as a first-class forge entity (the headline feature)** — **L** · ADR. A new
  `ctxforge.Hook` `{id, event: pre/post-tool-use, match, action: command | built-in allow|deny|ask,
  enabled}`, **persisted in `forge.json` like skills/agents/MCP/workflows** and fired by the bridge.
  *This is the "bake hooks into the app" ask* — the built-in safe policy (G0–G2) rides the same
  evaluator. Reuses `${VAR}` + preflight; hook command output is untrusted (sanitize).
- **G4 · Hooks/policy editor UI + mode binding** — **M**. Full **CRUD in the app** — a Hooks page to
  add / edit / enable-disable / remove hooks, exactly like the MCP/workflow forms; bind a hook set to
  **agent modes** (auto mode → strict defaults; ask mode → fully interactive). Surfaces *why* a call was
  auto-approved/denied in the timeline.

### Epic — Auth & connection (enablement) → issue [0051](issues/0051-epic-auth-and-connection.md)

- **A0 · Auth spike** — **S**. Document how `SDKClient` authenticates **today** (which of the four
  Copilot methods the underlying CLI/SDK resolves: device-flow keychain, env-var token, fine-grained
  PAT with "Copilot Requests", or `gh` reuse — precedence
  `COPILOT_GITHUB_TOKEN → GITHUB_TOKEN → GH_TOKEN → gh → device flow`). Feeds A1.
- **A1 · Auth-method surface** — **L** · ADR. A Connection page to choose + see the active method:
  **(a)** device flow (current/auto), **(b)** a pasted token saved locally **masked via the `${VAR}`
  indirection** (ADR-0020 reuse — *no secret at rest in plaintext*, unlike `~/.copilot/config.json`),
  **(c)** reuse the `gh` CLI token. Shows the resolved precedence + which credential is live.

### Carried from v9 (re-ranked into v10, lower than the correctness work)

- **V20 — per-lane rerun** (L, ADR) — the last child of the still-open **epic 0042**; re-run only a
  failed lane. Carries the partial-rerun/lane-lineage ADR question.
- **B — cost-anomaly reader** (M, no ADR) — pure `DetectAnomalies`, ambered on Telemetry; pairs
  naturally with P3 (both make the cost surface *active*).
- **Open Props / `@layer` + literal-cleanup** — the deferred additive CSS structuring (ADR-0025).

### Then — dedicated quality audit (after P1/P2, confirmed)

A focused **tech-debt + code-quality + architecture + workflow** review with its own findings doc
(retro/ADR as warranted): re-scan `TECH_DEBT.md`, the web layer's growing render modules, the
`Meter`/price-book seams after the P0/P1 reshape, and the e2e/demo-seeding gap this audit already
exposed. Scoped as its own pass so the pricing correctness lands first on the current base, then the
base is cleaned deliberately rather than mid-feature.

### Known artifact — modal over an in-flight View Transition (verified 2026-06-08)

The capture that showed *"Workflows rendered under the ⌘K palette"* (over the Chat page) is a **transient
cross-fade artifact, not a stuck-DOM bug** — **verified**: navigating to Chat, letting the ~140ms
transition finish, then opening the palette shows Chat behind it (`#main #composer` present, no Workflows
`h2`). The cause: `::view-transition-old(main)` snapshots render in the browser **top layer**, *above*
the `.overlay` (`z-index:50`), so a modal opened **within ~140ms of navigating** is briefly covered by
the old-page snapshot. Real-world impact is near-zero (the fade completes in 140ms), but it's a genuine
interaction. **Small fix item (v10 polish):** make ⌘K/help open in the **top layer** (a real
`<dialog>`/`:modal`, or raise above the transition), or have the palette opener await the transition;
add a guard. Recorded in REGRESSIONS as a known interaction.

### Recommended sequencing (v10)

Two co-lead pillars, interleaved by value: **billing** P0 (consistency spine) → P1 (price the two token
types) → P2 (fix the empty table); **governance** G0 (policy seam) → G1 (auto-approve reads) → G2
(dangerous-action deny + HITL) → **quality audit** → G3/G4 / P3 / P4 / A0→A1 / V20 / B, re-ranked on each
merge. The small View-Transition modal guard is pulled opportunistically. Numbering: next issues
**0050+**, next ADRs **0029+**. Each item born in its PR; SemVer per CONVENTIONS (features → minor: this
epic burst → `v0.3.0`).

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

---

## Roadmap v11 — Playful-polished visual + motion overhaul (design-research pass 2026-06-09)

> The **second presentation epic** (after v9/epic 0045), run from the **deferred** Open-Props +
> motion paydown that ADR-0028 named. v9 modernized the UI's *structure* (tokens, sidebar+palette,
> dashboard, a restrained motion pass) but the result reads structural, not designed. A five-leg,
> cited design-research pass (below) confirms modern vanilla CSS now covers a full **playful-polished**
> overhaul (Raycast discipline × Arc delight) — re-derived color, real depth, and a spring motion
> system — inside the hard constraints (one CSS file, htmx + server templates, no build chain, no
> framework, minimal vanilla JS). Filed as **epic 0062**; children **0063–0066**; reserves **ADR-0036**
> (palette/token + `@layer` + Open Props; keep-or-drop the terracotta/blue identity) and **ADR-0037**
> (motion system — `linear()` springs + view-transition policy).

### Direction (see epic 0062 charter for the full synthesis)
- **Aesthetic:** Raycast's token discipline (constrained radius/space scales, saturated accents used
  sparingly, tight type) × Arc's delight (bold gradients, depth, springy motion). Depth = surface
  **luminance ladder** in dark + **hue-tinted layered shadows** in light + optional radial-glow.
- **Palette/tokens:** re-derive in **OKLCH** (contrast tracks lightness, chroma-independent → raise
  saturation for "playful" without breaking AA by holding per-role `L` bands); three-tier tokens;
  `light-dark()`, `color-mix(in oklch)`, `@property`, **`@layer tokens,base,components,utilities`**;
  **Open Props** vendored offline (no CDN) for primitives/easings, semantic layer on top.
- **Motion:** **`linear()` spring easings** (gated `@supports` + cubic-bezier fallback) + motion/
  duration tokens; CSS-only catalogue (hover/press/focus, skeleton shimmer, list enter/exit + toast +
  palette via `@starting-style`/`allow-discrete`/`popover`, optional scroll-driven reveals gated for
  no-Firefox); **per-nav (never global) view-transitions** — global wrapping aborts transitions and
  drops htmx OOB / SSE swaps (the ADR-0028 dead-end, re-confirmed); reduced-motion guard extended.
- **Sequencing:** W1 tokens/palette (0063) → W2 surface/elevation (0064) → W3 motion (0065) → W4 hero
  polish Chat+Telemetry (0066). Each born in its PR, axe both-theme scan green per slice. Palette
  re-derivation lands first behind the gate (highest AA risk, fully reversible).
- **Risk/trade-offs (no build chain needed):** scroll-driven animations have no Firefox →
  progressive-enhancement only; P3-gamut accents wrap in `@media (color-gamut: p3)` + sRGB fallback.

> **v11 update (after W1):** **W1 shipped** (issue 0063, ADR-0036) — the four-layer `@layer`
> contract, the OKLCH primitive ramp behind the unchanged semantic names (terracotta identity
> kept, chroma raised inside AA-proved L bands), `color-mix(in oklch)` state tints replacing the
> rgba tint literals, Open Props v1.7.23 easings+animations vendored into the tokens layer, and
> the `css_tokens_test.go` structure/contrast guard (it caught two playful-chroma targets sitting
> outside the sRGB gamut — shipped at C 0.125/0.103; P3 variants stay W2 material). **Remaining,
> re-ranked:** **W2 (0064)** and **W3 (0065)** are both unblocked and parallelizable (disjoint
> seams: surface/elevation restyle vs. the motion system — W3 writes ADR-0037 first); then
> **W4 (0066)** closes the epic on the two hero surfaces.

### Sources (cited research)
- Raycast design vocabulary: github.com/VoltAgent/awesome-design-md (design-md/raycast/DESIGN.md);
  styles.refero.design (Raycast style teardown — *auto-extracted, indicative*); raycast.com/blog,
  developers.raycast.com, manual.raycast.com/themes.
- Arc + spring motion: blakecrosley.com/guides/design/arc; blog.logrocket.com (Arc UX analysis);
  joshwcomeau.com/animation/linear-timing-function; developer.chrome.com/docs/css-ui/css-linear-easing-function;
  carmenansio.com/articles/spring-physics-css; web.dev/articles/choosing-the-right-easing;
  linear-easing-generator.netlify.app (Archibald/Argyle generator).
- Dev-tool token systems: linear.app/now/how-we-redesigned-the-linear-ui; vercel.com/geist +
  seedflip.co/blog/vercel-design-system; joshwcomeau.com/css/designing-shadows;
  penpot.app + muz.li (token hierarchy, dark-mode elevation).
- Vanilla-CSS motion toolkit: web.dev/blog/same-document-view-transitions-are-now-baseline-newly-available;
  developer.chrome.com/blog/view-transitions-misconceptions + .../entry-exit-animations + .../scroll-driven-animations;
  htmx.org/essays/view-transitions; web.dev/blog/at-property-baseline; caniuse.com (support).
- Color/tokens: evilmartians.com/chronicles/oklch-in-css-why-quit-rgb-hsl; blog.logrocket.com/oklch-css-…;
  open-props.style + css-tricks.com/open-props-and-custom-properties-as-a-system; MDN (`@layer`,
  `color-mix`, `light-dark`); oklch.com; huetone.ardov.me (APCA/AA validation).

> **Confidence:** platform facts (browser-support baselines, View-Transitions "one per document",
> global-VT breaking OOB/SSE) are from primary sources (MDN/web.dev/Chrome/caniuse) **and**
> independently corroborated by this repo's own ADR-0028 / REGRESSIONS — high. Raycast exact
> hex/timings come from community/auto-extracted teardowns — treat as *indicative reference*, not gospel.
