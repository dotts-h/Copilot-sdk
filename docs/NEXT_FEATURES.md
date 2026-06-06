# Next-features research — my-orchestra (roadmap v2)

> Research deliverable, not a commitment. Fresh pass — code re-read **2026-06-06**.
> Roadmap **v1** (items 1.1–3.4) is shipped and exhausted (epics 0001/0005/0007 all
> closed); this supersedes it. Grounds the *next* candidates in the current
> codebase, `docs/TECH_DEBT.md`, and the product's two differentiators:
> **cost-awareness** (the meter) and **orchestration** (the name). Each item names
> the seam/files it touches, a rough effort (S/M/L), and a "why now." The
> build-first picks are promoted to `docs/issues/` under epic **0013**; the rest
> stay candidates here until promoted. Architectural choices are recorded as ADRs.

## Where the product is now

Chat is at parity and beyond (streaming text/reasoning split, tool timeline, inline
permissions, ask_user, plan review, elicitation, subagents, compaction, attachments,
slash commands, forge CRUD for skills/instructions/agents/**MCP**/**workflows**/
**snippets**, SDK-backed session resume, Settings, a live statusline + Telemetry
page, keybindings, a Wails desktop shell, a full Go + Playwright pyramid). Both
differentiators are **active** but **shallow**:

- **Cost (the meter)** is active end to end — pre-flight estimate (ADR-0007),
  soft-warn + hard-cap guardrails (ADR-0008), a persisted append-only ledger with
  trends (ADR-0009), a per-session statusline meter (ADR-0011). **But** the
  account-wide accounting rows still read the **live in-process meter**, so
  "remaining this month" resets on restart (TECH_DEBT #9, confirmed at
  `pages.go:183` `telemetryPartial` → `s.meter.Totals()`), and spend is not
  attributed to the agent/workflow that incurred it.
- **Orchestration (the name)** exists — a forge `Workflow` runs as lanes,
  sequential handoff or parallel fan-out (ADR-0013). **But** only the *sequential*
  path is end-to-end; *parallel* is model/engine-only and unobserved offline
  (TECH_DEBT #12, `MockClient` returns one session id), and a lane surfaces only
  message + usage (no per-lane tool timeline / inline permission).

So v2 is not about new surfaces — it's about making the two differentiators **deep
and accountable**: cost that survives time and attributes to *who spent it*, and
orchestration whose parallel half is actually observable. Candidates below are
ranked by value × fit; tech-debt paydown that's now worth promoting is folded in
where it advances a differentiator.

---

## Tier A — close the cost-accountability loop (the meter's promise)

v1 made cost *active*; v2 makes it *accountable across time* and *attributable
across agents*. The ledger (`SpendStore`) already records every turn with `At` +
`SessionID` and survives restart — the account-wide reads just still point at the
wrong (in-process) source.

### A1 — Ledger-derived budget rows  — **M**  ·  *promotes TECH_DEBT #9*  ·  **BUILD FIRST**
- **What:** the Telemetry "Total cost / Monthly budget / Remaining" rows, the cost
  footer, and the hard-cap projection baseline read **month-to-date from
  `SpendStore`** (a new pure `telemetry.MonthToDate(records, now)`, UTC calendar
  month) instead of the live `Meter`, so they survive restart. The per-session
  statusline (`sessionMeter`, ADR-0011) and the live token split stay on the
  in-process meter — one source per surface.
- **Why now:** the last restart-amnesiac gap in the headline promise ("never
  surprises you on the bill"); ADR-0009 explicitly deferred exactly this. The
  ledger already exists, is atomic, and tags every record — only the *read* moves.
- **Touches:** `internal/telemetry` (`history.go`: `MonthToDate`, pure + tested),
  `internal/web` (`pages.go` `telemetryPartial`, `render.go` `renderCostFooter`,
  `server.go` `budget()` / `overCap`), CONTRACTS §4 + ARCHITECTURE note.
- **Decision → ADR-0016** (ledger is the source of truth for account-wide
  accounting; written first per ADR-0004). **Issue [0014](issues/0014-ledger-derived-budget-rows.md).**

### A2 — Cost attribution: per-agent / per-workflow / per-session rollups  — **M/L**  ·  **SHIPPED (#TBD)**
- **What:** `SpendRecord` already carries `SessionID`; tag it additively with the
  active **agent id** (and workflow/lane id when a run owns the turn) and add a
  "Cost by agent / workflow" breakdown on Telemetry (a pure aggregation like
  `ModelShares`). Answers *"which agent is burning my budget?"* — orchestration-
  aware cost, where the two differentiators meet.
- **Why now:** builds directly on A1's ledger queries; workflows already meter
  per-lane cost (ADR-0013) but don't *persist* the attribution. Schema bump is
  additive (CONTRACTS §4 migration rules).
- **Touches:** `internal/telemetry` (`SpendRecord` `+agentId`/`+workflowId`,
  versioned-additive; an `AgentShares` aggregation), `internal/web` (`session.go`
  `EvUsage` record-tagging, `workflow.go` `handleRunEvent`, `telemetryPartial`).
- **Shipped:** schema v2 additive `agent`/`workflow`/`lane` tags + pure
  `AgentShares`/`WorkflowShares` (over a shared `shareBy`); `recordUsage` takes a
  `spendTag`; "Cost by agent / workflow" on Telemetry; CSV columns appended.
  **Decision → ADR-0018. Issue [0016](issues/0016-cost-attribution-rollups.md).**

### A3 — Budget burn-rate projection / forecast  — **S/M**  ·  **SHIPPED (#TBD)**
- **What:** from `DailyTotals` + the allowance, project *"at this rate you reach
  your cap in ~N turns / by <date>"* — on Telemetry and (compact) in the
  statusline. Turns cost from **reactive** (warn at 80%) to **predictive**.
- **Why now:** A1 puts month-to-date on a real time series and `DailyTotals`
  already gives the slope; a pure function, no new IO.
- **Touches:** `internal/telemetry` (a pure `Forecast` over `DailyTotals` +
  `Budget`), `internal/web` (`telemetryPartial`, optional statusline cell).
- **Shipped:** pure `telemetry.Forecast` → `Projection` (trailing-7-day-average
  burn rate; days/turns-to-cap + exhaustion date; degenerate cases explicit in
  `Status`: no-budget / idle / exhausted / ok); a Telemetry-page forecast line +
  a compact `cap ~Nd` statusline cell (ambers when on track to exceed the budget
  before month-end). **Decision → ADR-0019. Issue [0017](issues/0017-budget-burn-rate-forecast.md).**

## Tier B — deepen orchestration (the name)

The differentiated half of "orchestra" is still mostly on paper: parallel fan-out
is unobserved and lanes are thin.

### B1 — Real parallel workflow lanes  — **M/L**  ·  *promotes TECH_DEBT #12*  ·  **BUILD-FIRST (orchestration)**
- **What:** give `MockClient` distinct session ids + `SessionID`-tagged demo events
  so a browser-driven **parallel** run drives concurrent lanes; surface per-lane
  **tool cards + inline permissions** in each lane (today a lane folds only message
  + usage).
- **Why now:** parallel fan-out is the orchestration payoff and is currently
  undrivable offline (no demo/e2e). The run state machine + `SessionID` routing
  already exist (`workflow.go` `laneFor`); this makes them *exercised*, not just
  unit-tested. Clears TECH_DEBT #12.
- **Touches:** `internal/copilot` (`mock.go`: distinct ids per `CreateSession`),
  `internal/web` (`workflow.go` `handleRunEvent` per-lane tool/permission render,
  `demo.go` `SessionID`-tagged parallel demo, lanes templates/CSS), `e2e/` parallel
  spec. **Extends ADR-0013. Issue [0015](issues/0015-real-parallel-workflow-lanes.md).**

### B2 — Conditional / branching workflow steps  — **L**  ·  **SHIPPED (#TBD)**
- **What:** a step gated on the prior lane's outcome (e.g. *"if the review lane
  flags issues, run the fix agent"*). Moves `Workflow` from a fixed pipe to real
  control flow — a `When`/predicate on `WorkflowStep`, pure additions to
  `workflowRun` + `CompileWorkflow`.
- **Why now:** sequential + parallel are the primitives; branching is the first
  genuinely *orchestration*-shaped capability beyond fan-out/handoff. **Needs an
  ADR** (declarative predicate vs free-form condition model).
- **Touches:** `internal/ctxforge` (`workflow.go`: `WorkflowStep.When` +
  validation), `internal/web` (`workflow.go` state machine), an ADR.
- **Shipped:** a declarative `WorkflowStep.When` (`StepCondition{Step, Condition,
  Value}`; `succeeded`/`failed`/`output-contains`/`always`, gating on a strictly-prior
  step → acyclic by construction) — pure, `Validate`-able, additive (`omitempty`,
  nil = always). The pure `workflowRun` engine gained a `laneSkipped` status and
  `evalWhen`/`evalPending`/`advance`: an unsatisfied step is **skipped** (rendered
  distinctly), not failed, and a skipped lane still terminates the run; works in both
  sequential (walk-and-skip) and parallel (launch-when-dependency-settles) modes. A
  seeded branching demo + e2e drive a real branch (a skipped lane). **Decision →
  ADR-0021. Issue [0020](issues/0020-conditional-branching-workflow-steps.md).**

### B3 — Workflow run history  — **M**
- **What:** persist each run (workflow id, agents, per-lane cost, outcome,
  timestamps) and a "Runs" view. A run is the natural unit of orchestrated spend.
- **Why now:** runs are ephemeral today; once A2 tags ledger records with
  workflow/lane, a run-history view is mostly a query. Pairs A2 ↔ B.
- **Touches:** `internal/telemetry` (or a sibling run-store), `internal/web`
  (`workflow.go`, a Runs partial/page — minds the `pages.length` e2e coupling).

## Tier C — extensibility & composer polish (promoted debt)

### C1 — MCP secrets / Env editor  — **M**  ·  *promotes TECH_DEBT #10*
- **What:** a masked secrets store + `Env` editing on the MCP form, unblocking
  **key-requiring** curated servers (GitHub, web search). `MCPServer.Env` already
  exists and is preserved across edits but is not UI-editable (`mcp.go:33`).
- **Why now:** the MCP page (ADR-0010) shipped key-free *because* there's no
  secrets surface; this is the gate to the highest-value MCP servers — and MCP is
  how users grow the agent's tools. **Needs an ADR** (where secrets live — not
  plaintext in `forge.json`).
- **Touches:** `internal/ctxforge` (`MCPServer.Env`), `internal/web` (`mcp.go`
  form, `forms.go` masked field), a new secrets store, an ADR.

### C2 — Textarea composer  — **S**  ·  *promotes TECH_DEBT #15*
- **What:** switch the composer `<input>` to a `<textarea>` (Enter-to-send,
  Shift-Enter newline), fixing the multi-line snippet flatten on insert
  (`fillSnippet`) and enabling multi-line prompts generally.
- **Why now:** snippets (ADR-0015) shipped with a known flatten limit; small,
  self-contained, compounding UX win the snippet library already wants.
- **Touches:** `internal/web` (`templates/index.html` composer, `fillSnippet`, the
  `keydown` handler's Enter/Shift-Enter), `e2e/` composer spec. Clears TECH_DEBT #15.

## Tier D — platform / distribution (carried, unchanged)

Paydown, not product — pull when demand appears:
- **Desktop installers** (.dmg/.msi/.deb/AppImage) via `wails3 package` — TECH_DEBT #5.
- **Wails v3 stable migration** when it lands — TECH_DEBT #6 (pinned to `alpha.98`).
- **On-disk `SKILL.md` folder model** — TECH_DEBT #1; deferred pending real demand.
- **First-party embedded MCP server** (zero external runtime) — TECH_DEBT #11.

---

## Recommended sequencing

1. **A1 — ledger-derived budget rows** *(BUILD FIRST)*. Completes the cost
   differentiator's headline promise; M; reuses `SpendStore`; grounded in ADR-0016;
   unblocks A2/A3. → issue **0014**.
2. **B1 — real parallel lanes**. Completes the orchestration differentiator's
   unobserved half; M/L; engine already exists. → issue **0015**.
3. **A2 → A3** — cost attribution, then forecast: per-agent/per-workflow spend, then
   predictive burn-rate. The two differentiators meet in A2. **Both shipped**
   (A2 ADR-0018 #41; A3 ADR-0019) — Tier A's cost-accountability loop is now closed.
4. **C2 (textarea composer)** — small, compounding; then **C1 (MCP secrets**, lead
   with an ADR for the secrets store).
5. **B2 / B3** — branching + run history: the bigger orchestration bets; lead each
   with an ADR. **B2 shipped** (branching workflow steps; ADR-0021, issue 0020) —
   `Workflow` is now real control flow. **B3 (workflow run history) is the last v2
   item.**
6. **Tier D** when distribution demand appears.

Epic **[0013](issues/0013-epic-deepen-differentiators.md)** ("deepen the
differentiators") carries A1 (0014) + B1 (0015) as the promoted build-first picks;
everything else stays a candidate here until promoted.

Each item: write the failing test first, keep domain logic pure
(`telemetry`/`ctxforge`/`config` dependency-free), run `make lint && make test`
(coverage floor 65%) + `make e2e` for UI, and fold its ADR/CONTRACTS/REGRESSIONS
updates into the same feature branch (ADR-0004).

---

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
