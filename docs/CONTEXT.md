# CONTEXT.md — the domain glossary (ubiquitous language)

> The **single source of truth for what each domain term means.** Define a term here
> **once**; code comments, ADRs, CONTRACTS, and issues use it without re-defining. If a
> term's meaning changes, change it here and let the pointers follow. This is the working
> memory the codebase is built around — read it before naming a new type or writing a doc.
>
> Format: **term** — one-line meaning · *where it lives* (package/symbol). Stability of the
> names themselves is governed by [CONTRACTS.md](CONTRACTS.md); the *why* by the linked ADR.

## Product framing

- **my-orchestra** — the app: a cost-aware coding web UI over the GitHub Copilot CLI/SDK.
  Two differentiators: **cost-awareness** (the meter) and **orchestration** (workflows).
- **differentiator** — one of the two product axes every feature is ranked against:
  *cost-awareness* and *orchestration*. The convergence of the two is the current frontier
  (see *reconcile*).

## The forge — context composition (`internal/ctxforge`)

- **forge** — the context composer: the user's library of skills, instructions, agents, MCP
  servers, workflows, and snippets that **compile into a session** (`Forge.Compile` →
  `copilot.SessionSpec`). Persisted as `forge/forge.json`. — ADR-0003
- **skill** — a reusable block of prompt context toggled **into** the system message
  (`ctxforge.Skill`). Always-on skills can be pinned to an agent.
- **instruction** — a system-message rule, **priority-ordered** (`ctxforge.Instruction`).
- **agent** (persona) — a named `{model, reasoningEffort, pinned skills, tool allowlist}`
  (`ctxforge.Agent`). The **active** agent tags each turn's spend (cost attribution). The
  built-in **chat** agent is the empty-persona default. — ADR-0003, ADR-0018
- **MCP server** — an external stdio tool provider (`ctxforge.MCPServer`); `env` holds masked
  key/value rows where a **secret** persists only a `${VAR}` reference, resolved at session
  start (no secret at rest). — ADR-0010, ADR-0020
- **workflow** — a multi-agent **run definition**: `{mode, steps}`, `mode ∈
  {sequential, parallel}` (`ctxforge.Workflow`). — ADR-0013
- **step** — one stage of a workflow `{agentId, prompt, when?}`; `when` is a **branching**
  predicate over a strictly-prior step's settled outcome (`ctxforge.StepCondition`). When
  referenced by a condition, steps are **1-based**. — ADR-0021
- **snippet** — a saved one-shot composer prompt (`ctxforge.Snippet`), inserted via a
  `/trigger`. Not system-message context (never compiled). — ADR-0015

## Orchestration — running workflows (`internal/web/workflow.go`, `internal/telemetry/runs.go`)

- **run** — one completed execution of a workflow, recorded once on finish
  (`telemetry.RunRecord`). The product's unit of orchestration.
- **lane** — one execution track within a run: a step instantiated. Settled **status ∈
  {done, failed, skipped}** with metered **credits** (`telemetry.RunLane` /
  `web.lane`). A **skipped** lane (unsatisfied `when`) incurs **zero** cost — the data a
  spend record can't express. — ADR-0017, ADR-0021, ADR-0022
- **outcome** — the run-level result: **finished | failed** (a skipped lane is normal, not a
  failure).
- **mode** — `sequential` (each lane's output feeds the next) or `parallel` (concurrent
  lanes, disambiguated by SessionID).
- **rerun** — re-executing a recorded run's workflow from the Runs page: its **current**
  definition (looked up by `WorkflowID`), run as a fresh run under the **same** `WorkflowID`
  so its spend rolls up with the original's — a **re-execution, not a historical replay**
  (`web.handleRunRerun` via the shared `launchWorkflow` trigger). The first **action** on
  the orchestration history surface. — ADR-0023
- **abort** (a run) — **stopping an in-flight run** from the Chat lanes panel (`⏹ stop run`
  → `web.handleRunAbort`): each still-running lane's backing session is aborted over the
  `copilot.Client.Abort` seam, the unsettled lanes settle as **failed** (detail `⏹
  aborted`) and the run records as a **failed** outcome — a stopped run is a failed run, no
  new terminal status. The **dual of rerun**, completing the interactive control set (start
  → rerun → stop). Distinct from the **chat-turn** abort (`web.handleAbort`, `POST /abort`),
  which stops the chat session, not a run. — ADR-0024

## Cost — the meter and the ledger (`internal/telemetry`)

- **credit** — a GitHub AI Credit. **1 credit = $0.01** (`USDPerCredit`). All UI cost reads
  in credits (`FormatCredits`).
- **AIU** — GitHub's **authoritative** cost in AI units, when the runtime reports it (0 for
  the offline mock).
- **meter** — the **live, in-memory** cost gauge (`telemetry.Meter`). Two scopes share one
  price book: the **account-wide** meter (topbar/budget) and the **per-session** meter
  (statusline). A gauge — not the source of truth across restarts. — ADR-0011, TECH_DEBT #2
- **price book** — the model→token-rate table (`telemetry.PriceBook`). Built from
  `DefaultPriceBook()` + `config` overrides via `BuildPriceBook`; live-repriced by
  `Replace`. — ADR-0008 (G1)
- **turn** — one assistant exchange; the unit a `SpendRecord` meters.
- **ledger** (spend ledger) — the **persisted, append-only** spend history
  (`telemetry.SpendStore`, `spend.json`): one `SpendRecord` per metered turn, tagged with
  agent/workflow/lane/session. The **source of truth** for account-wide accounting across
  restarts. — ADR-0009, ADR-0016, ADR-0018
- **budget** — the monthly credit **allowance**; a **warn fraction** (soft warning) and a
  **hard cap** (the gate) sit against it. — ADR-0008
- **gate** (budget gate) — a turn **paused before `Send`** when it would breach the hard cap;
  resolved by `proceed | raise | cancel`. An app-level gate, not an SDK permission.
  — ADR-0008
- **forecast** — a trailing-window **burn-rate projection** to the allowance
  (`telemetry.Forecast`); the **bucketed** variant projects per agent/workflow
  (`BucketForecasts`). — ADR-0019

## Reading the persisted data — the pure readers (`internal/telemetry`)

- **share** — a per-group **slice of total spend** with its fraction (`ModelShares` /
  `AgentShares` / `WorkflowShares` / `SessionShares`, all on `shareBy`). The "Cost by …"
  bars. — ADR-0018
- **aggregate / roll-up** — a per-workflow **run** summary (`RunAggregates` →
  `RunAggregate{Runs, Failures, Total/AvgCredits, Duration, LastOutcome}`); **`LaneShares`**
  is the per-(workflow, lane) cousin. — ADR-0022
- **reconcile** — the cross-store **join** of the ledger's per-workflow spend (`WorkflowShares`
  grain) against the run history's (`RunAggregates` grain), per workflow
  (`telemetry.WorkflowReconcile`). The convergence of the two differentiators. The per-lane
  cousin (`telemetry.LaneReconcile`) joins the **same** stores one grain finer — per
  `(workflow, lane)`, the lane-tagged ledger vs the `LaneShares` grain — so a divergence is
  locatable at the exact step, not just the workflow total.
- **delta** — `LedgerCredits − RunCredits`: how far the two stores **disagree** for a
  workflow. Ambered in the UI when non-trivial.
- **window** — the **14/30/90-day** slice selector shared by the spend trend and the Runs
  page (`clampWindow`, `spendWindows`). — G3/V12
- **AppendOnlyStore[T]** — the **generic** persisted store both `SpendStore` and `RunStore`
  embed: atomic temp-file+rename, missing=empty, invalid=error, `dir==""`-ephemeral.
  — ADR-0009/0022, H1

## The web layer — serving and rendering (`internal/web`)

- **seam** — the **`copilot.Client` interface**, the single boundary to the runtime
  (`SDKClient` live, `MockClient` offline/tests). **No SDK import crosses it.** — ADR (backfill)
- **hub** — the process-wide **multi-session router**: holds shared forge/config/stores,
  routes each cookie to its `Server`, and runs the **event pump**.
- **server** — **one session's** state + HTTP handlers (per cookie). Dense but not a
  god-object; mutates shared forge/config only under `forgeMu`.
- **session** — a conversation, SDK-**resumable** (`copilot.SessionMeta`). — ADR-0002
- **partial / fragment** — an `html/template`-rendered HTML chunk, swapped via **htmx**
  (`hx-get` into `#main`) or streamed over **SSE** (`/events`). Model-originated text is
  escaped (ADR-0001).
- **lock order** — **`forgeMu → s.mu`**, never inverted (shared forge/config before
  per-session state). Race-tested. — see `internal/web/sessions.go`
- **demo / mock mode** — the offline path (`-demo`): a scripted `MockClient` + seeded
  forge/ledger/runs, so every page renders with no CLI/token. — `internal/bootstrap`

## Presentation — tokens & theming (`internal/web/static/app.css`)

- **design token** — a **semantic** CSS custom property (`--bg`, `--fg`, `--accent`, `--good`,
  …) that names a **role**, not a raw color; the single home for a color value (new UI uses the
  token, never a literal hex / `rgba(255,255,255,…)`). `--on-bright` is the text color for **any**
  solid accent/good/warn/bad fill; `--hover`/`--raised`/`--sunken` are theme-aware neutral
  elevations. — ADR-0025
- **theme** — the **light** or **dark** color scheme. Tokens carry both values in one declaration
  via the CSS `light-dark()` function, resolved by `color-scheme`; the toggle flips
  `<html data-theme>` (→ `color-scheme`), persisted **client-side** in `localStorage`, with the
  **OS preference** as the default. A synchronous `<head>` script sets the attribute before first
  paint (no flash). Client-only — no server route, no schema. — ADR-0025
- **sidebar** — the shell's left **navigation column**: the single `<header>` banner, laid out as a
  left column on wide viewports and reflowed to a compact wrapping top bar on narrow ones (CSS-only,
  no JS router). It contains the grouped `<nav>`, the theme toggle, and the `#cost-footer`. — ADR-0026
- **nav group** — one labelled cluster of the sidebar's pages, by user intent: **Primary** (Chat,
  Sessions) · **Build** (Agents, Workflows, Skills, Instructions, Snippets) · **Observe** (Runs,
  Telemetry) · **Config** (Models, MCP, Settings) · **Help**. The grouping lives on the `group` field
  of `pageNames` (one source); Config + Help are pinned to the bottom (progressive disclosure).
  — ADR-0026
- **command palette** — the ⌘/Ctrl-K modal (`web.commandPalette`, mirroring the help overlay): a
  filter input over a server-rendered `{slug,label,group}` list, filtered client-side and navigating
  the match via the existing keymap `navClick` seam (no new route). ⌘K is a **fixed** modifier chord,
  outside the single-key configurable keymap (ADR-0014). — ADR-0026
- **KPI card** — one "big number" tile in the Telemetry dashboard's top row: a current-window value
  (total spend, turns, avg cost/turn, or burn rate) with a **period-over-period delta** badge (▲/▼/→ a
  signed %) and a **sparkline**. The delta color is per-metric: a rise in spend/avg/burn is `--warn`
  (higher-is-worse), a rise in turns is `--good` — never a blanket green ▲. A metric with no prior
  baseline reads "new". `telemetry.Dashboard`/`ChangePct` compute it; `web.dashboardView` renders it.
  — ADR-0027
- **sparkline** — a small, axis-free inline-SVG trend line (a normalized polyline over a metric's
  zero-filled daily series) inside a KPI card, server-rendered from a pure Go builder (`web.sparklineSVG`).
  — ADR-0027
- **trend band** — the Telemetry dashboard's cumulative-spend chart (`web.trendBandSVG`): a filled
  **actuals** area (solid) plus a **dashed burn-rate forecast** continuing at the window's daily rate
  over the days left this month. Inline SVG, zero JS. — ADR-0027
- **bullet graph** — the spend-vs-budget chart (Few's bullet; `web.bulletSVG`): a budget **track**, a
  month-to-date **measure bar**, and a **target marker** at the projected month-end spend at the
  current pace (flipped to `--bad` when over budget). Renders only when a monthly budget is set.
  — ADR-0027

## Process vocabulary

- **ADR** — Architecture Decision Record (`docs/adr/`): the **single home of a decision's
  why**. Highest is the reserved next number.
- **epic / child** — a tracked body of work (`docs/issues/`); an epic is **born in its first
  child's PR**. Issues mirror to GitHub. — `tracking-issues` skill
- **gate** (CI) — a required check: lint · race+coverage (floor 65%) · fuzz · build matrix ·
  e2e. Green before merge.
- **leash** — running an agent change at **lower autonomy**: plan-first, human-approves-plan,
  then execute. Used for refactors that touch a seam (vs. the default build-and-merge flow).

---
*Seeded 2026-06-07. A term belongs here the moment it appears in two files' comments. When
you add a domain concept, add its one-liner here first, then write the code.*
