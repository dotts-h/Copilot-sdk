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
