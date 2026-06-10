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
- **hook** — a forge-backed **governance rule** fired around a tool call
  (`ctxforge.Hook`): `{event (pre/post-tool-use), match (tool kind + optional command
  pattern), action (allow|deny|ask), reason, enabled}`. `Forge.Compile` folds the
  built-in safe defaults + the enabled user hooks into the session; the bridge
  consults the pure `Evaluate` before the permission gate (**deny > ask > allow**,
  default **ask**). The third governance pillar — enforced in the bridge, not just
  config. — ADR-0029
- **safe-read default** — the built-in `ctxforge.DefaultHooks()` set: auto-approve
  read-only tool kinds, leave writes/shell/MCP to the gate. Built-ins run through the
  **same** `Evaluate` as user hooks, so a user `deny` still wins. Makes the
  out-of-the-box build safe. — ADR-0029
- **mandatory gate** — a built-in hook whose decision is **unbypassable by config**
  (`Hook.Mandatory`): a mandatory `deny` rejects and a mandatory `ask` gates **even when the
  session runs with `AutoApproveTools`**. The dangerous-action ruleset
  (`ctxforge.DangerousHooks()`) is all mandatory — hard-`deny` the clearly-destructive
  (`rm -rf /`, `curl|sh`, exfiltration), force-`ask` the risky-but-legitimate (`sudo`, an
  out-of-workspace write). It does **not** change `deny > ask > allow` (a user `deny`, being
  more restrictive, still wins over a mandatory `ask`); it only forecloses the auto-approve
  escape hatch, enforced on the auto path in the bridge. — ADR-0030
- **mode binding** — scoping the governance policy to the **active agent mode**
  (`autopilot`/`interactive`/`plan`). Two parts (ADR-0031): a per-hook `Hook.Modes` set (empty =
  every mode) threaded into `Evaluate` as the `mode` argument, so a user hook can fire only in one
  mode while the **mandatory** ruleset (empty `Modes`) holds in *every* mode; and the
  `EffectiveAutoApprove(mode, configDefault)` **baseline** the bridge applies to the non-mandatory
  remainder — `autopilot` → on (strict defaults on, unattended), `interactive` → off (more gates),
  else the session's `AutoApproveTools` config. The mode is a runtime fact recorded on the
  per-session policy and updated at `Send`, like the workspace root. — ADR-0031
- **timeline "why"** — the inline annotation that explains a governance decision the bridge made
  **without a gate**: a normalized `copilot.EvToolDecision` event (`ToolDecision{Kind, HookID,
  Reason, Detail}`) reduced into a compact `convo.RoleDecision` turn — "denied: *reason*" (a
  hard-deny has no tool card otherwise) or "auto-approved by *hook*" (a user allow). It is an
  **annotation, not a gate** (the call already proceeded or was blocked). Emitted only where there
  is no other surface and real value (a deny, a user allow); the safe-read/autopilot baseline
  approvals stay silent. A gated **ask** is not a `ToolDecision` — it surfaces as the existing
  `EvPermission` form, now carrying the hook `Reason`. — ADR-0031
- **hook command (PostToolUse executor)** — a local command a **post-tool-use** hook runs *after* a
  matching tool completes (`Hook.Command`/`CommandArgs`; G5, ADR-0032). The seam selects matching
  command hooks (`ctxforge.PostToolUseCommands`) off the tool-completion flow and runs each — the
  program exec'd **directly** (no shell, no chaining), `${VAR}` resolved at execution via the same env
  seam as MCP (ADR-0020; an unset ref → empty, never the literal), a **5s timeout**, ~2KB of bounded
  output, the **workspace** as cwd. It is valid **only** on a post-tool-use hook (a PreToolUse hook
  with a command is rejected). The command's output is **untrusted, display-only telemetry** — bounded,
  **escaped**, never fed back to the agent and **never a gate** (a non-zero exit is annotated, not a
  control). It surfaces as the **hook-run note**. — ADR-0032
- **hook-run note** — the inline timeline annotation for a hook command's execution: a normalized
  `copilot.EvHookRun` event (`HookRun{HookID, Command, Output, ExitCode, TimedOut, Failed}`) reduced
  into a compact `convo.RoleHookRun` turn naming the hook, its resolved command, an exit/timeout
  status, and a bounded, **escaped** snippet of the (untrusted) output. Like the timeline "why", it is
  an **annotation, not a gate**, and is live (not persisted across resume). — ADR-0032
- **workspace fence** — the path-aware match dimension (`HookMatch.OutsideWorkspace`) the glob
  matcher can't express: a built-in mandatory hook that gates a **write whose target resolves
  OUTSIDE the session workspace root**. The root is a runtime fact threaded at the seam
  (`copilot.SessionSpec.Workspace`, set to the process cwd by `bootstrap`) into the pure
  `Evaluate`, which normalizes the path with `filepath.Rel`; an empty root makes the fence
  inert. — ADR-0030

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
  the offline mock). **1 AIU = 1 credit** (same unit) — see *reported spend*.
- **estimate vs. reported (actual) spend** — the **source hierarchy** for what a turn cost
  (`telemetry.ActualCredits`/`HasReported`): the **reported** figure (`ReportedAIU`, GitHub's
  authoritative AIU) is the truth for **actual** spend when present; the **price-book
  estimate** is the fallback (pre-flight composer + forecast + offline). A surface shows the
  actual figure tagged *reported* / *est* / *mixed* (`MonthToDateActual` → `ActualSpend`,
  `renderActualCostFooter`). — ADR-0033
- **meter** — the **live, in-memory** cost gauge (`telemetry.Meter`). Two scopes share one
  price book: the **account-wide** meter (topbar/budget) and the **per-session** meter
  (statusline). A gauge — not the source of truth across restarts. — ADR-0011, TECH_DEBT #2
- **price book** — the model→token-rate table (`telemetry.PriceBook`). Built from
  `DefaultPriceBook()` + `config` overrides via `BuildPriceBook`; live-repriced by
  `Replace`. — ADR-0008 (G1)
- **cache-write** — prompt-cache *write* (cache-creation) tokens, a billed category
  **distinct** from fresh input and cached (read) input and **additive** to the bill. The
  estimate prices them at `CacheWritePerMTok` (default **1.25× input**, overridable). — ADR-0034
- **reasoning** (thinking) tokens — a **subset of output** tokens (the model's
  chain-of-thought), so they are **already** priced at the output rate; surfaced as a
  display-only count, never charged a second time. — ADR-0034
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
- **drift** — the per-model gap between the **price-book estimate** and GitHub's **reported**
  cost over the ledger's *reported turns* (`telemetry.ModelDrifts` → `ModelDrift`): the cost
  cousin of *reconcile*, joining the two figures **within one store** (each `SpendRecord`
  carries both — see *estimate vs. reported*). `Delta = EstimateCredits − ReportedCredits`;
  ambered past epsilon. An unreported turn has no authoritative figure to drift from, so it
  is counted (est-only coverage) but never compared. — issue 0060
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
  elevations. Tokens are **three-tier**: primitive OKLCH ramps (`--p-*`, the only tier holding raw
  color) → semantic roles → component/state tokens (`color-mix()` tints); components never
  reference a primitive. — ADR-0025, ADR-0036
- **layer contract** — `app.css`'s cascade order, `@layer tokens, base, components, utilities`:
  every rule lives in one of the four layers (an un-layered rule would outrank them all), the
  vendored Open Props subset imports into `tokens`, and `css_tokens_test.go` enforces the
  structure plus WCAG AA contrast for every text-role/surface pair in both themes, computed from
  the OKLCH primitives. — ADR-0036
- **elevation (dual-channel)** — how a surface reads as raised, one channel per theme: in **dark**
  the **surface ladder** (`--bg` → `--panel` → `--overlay`) steps *lighter* when raised (shadows are
  invisible on a dark canvas); in **light**, **hue-tinted layered shadows** — one `--shadow-color`
  stacked as `--shadow-1/2/3` (2/4/5 layers, alpha accumulating) — carry depth instead.
  `--border-glass` is the 1px translucent glass border; `.glass`/`.atmosphere` (low-opacity radial
  glow) are the opt-in hero utilities. — ADR-0038
- **scales** — the constrained geometry/type ladders components must consume: radius
  `--radius-1…5/full` (a `border-radius` px literal outside the tokens layer fails the guard), space
  `--space-1…6` (8px rhythm, 4px half-step), type `--text-0…5` + `--tracking-display` (negative at
  display sizes) / `--tracking-caps` on the `--font-sans`/`--font-mono` pair. — ADR-0038
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
- **View Transition** — a same-document page swap that **cross-fades** instead of hard-cutting:
  htmx wraps the `#main` swap in the browser's `document.startViewTransition`, opted in **per
  navigation** with `transition:true` on the sidebar nav links (NOT `globalViewTransitions`, which
  would wrap the `hx-swap-oob` streaming updates and drop run/turn completion swaps), scoped to the
  panel by a `view-transition-name` and silenced under `prefers-reduced-motion`. The swap is async, so
  the e2e `navTo` waits for `htmx:afterSettle`. Templates + CSS, **zero new JS**. — ADR-0028
- **component polish pass** — a contrast-neutral refinement over the V21 tokens: shared
  `--speed`/`--ease` motion + `--shadow`/`--shadow-lg` elevation tokens, an eased transition on the
  interactive controls, a 1px `:active` press on the solid buttons, and a resting shadow on the card
  surfaces. Changes **no color pairing**, so the both-theme axe scan is unaffected. — ADR-0028

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
