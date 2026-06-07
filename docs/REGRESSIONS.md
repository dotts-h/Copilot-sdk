# Regression log

A running record of bugs and rough edges we've found and fixed, so we don't
reintroduce them. **Every entry must name the test that now guards it.** If a fix
lands without a guarding test, it goes in "Known gaps" below until one exists.

Layers: **unit** (`go test ./internal/...`, `./cmd/...`), **api/contract**
(`internal/web/api_test.go`), **browser** (`e2e/` Playwright: e2e · api · a11y ·
ux · perf). See [ARCHITECTURE.md](ARCHITECTURE.md#testing-philosophy-tdd--sdet).

## Fixed

| # | Symptom (what was observed) | Root cause | Fix | Guarding test(s) |
|---|------------------------------|-----------|-----|------------------|
| 1 | "Thinking" rendered **twice** in chat — once while streaming, again below the answer. | The runtime emits a full `AssistantReasoningData` block after a segment that already streamed as deltas; the web layer appended both. | `SDKClient` tracks per-segment streaming (`reasoned[sid]`) and drops the duplicate full block; the flag is also cleared on idle so an interrupted segment can't suppress a later non-streaming block. | unit: `internal/copilot` `TestHandlerDropsDuplicateReasoningBlock`, `TestHandlerResetsReasoningFlagOnIdle` |
| 2 | The **model in use was not shown** in the chat toolbar. | No statusline; the topbar showed only brand/nav/cost. | Live statusline in the composer bar names the active model. | unit: `internal/web` `TestUsageEmitsStatlineWithTokenBreakdown`; browser: `e2e/tests/e2e.spec.ts` "the statusline shows the model and counts the message sent" (asserts `gpt-5`) |
| 3 | **Telemetry was thin** — no cache-write tokens, thinking tokens, message/tool counts, cache-hit %, session time, or USD in view. | `CacheWriteTokens` was unmapped from the SDK; no per-session counters; no statusline. | Map `CacheWriteTokens`; accumulate cache-write + reasoning as display-only totals (`Meter.ExtraTokens`); Server tracks session start / messages / tools; statusline renders the full split + cache-hit % + credits/USD. | unit: `internal/web` `TestUsageEmitsStatlineWithTokenBreakdown`, `TestToolStartCountsTowardStatline`; `internal/telemetry` token tests |
| 4 | A model could be picked but its **reasoning effort could not be set**. | `ReasoningEffort` existed on the spec/config but was only settable via an agent. | `/effort <low\|medium\|high>` command + a reasoning-effort row on the Models page (`POST /effort/{value}/select`), constrained to the model's supported set. | unit: `internal/web` `TestEffortCommandSetsSpec`, `TestModelsPageShowsEffortRowForCurrentModel`, `TestEffortSelectSwitchesAndRestarts` |
| 5 | No **auto / ask** agent modes (only `/plan`). | The seam mapped all four `AgentMode`s but the UI exposed only plan. | `/auto` (autopilot) + `/ask` (interactive); the plan-only flag became one mutually-exclusive `mode` string. | unit: `internal/web` `TestAutoAndAskModesAreExclusive`, `TestPlanCommandTogglesMode` |
| 6 | The context meter read **"ctx"**, not "context". | Label string. | Renamed to "context" in the meter and statusline. | browser: `e2e/tests/e2e.spec.ts` "the cost footer and context meter update after a turn" (matches `/tok\|context/`) |
| 7 | **Remote CI red**: the e2e workflow failed on `toggling a skill` / `selecting an agent`. | Demo mode shipped no forge, so the Skills/Agents pages rendered empty and the tests had nothing to drive (a local `forge.json` masked it). | Extracted `seedForge()`; demo seeds a representative forge in memory and pins to a listed model. | unit: `cmd/my-orchestra` `TestSeedForgePopulatesEmptyForge`, `TestSeedForgePreservesExisting`; browser: `e2e/tests/e2e.spec.ts` forge-management tests |
| 8 | Composer **wiped its input on every keystroke**. | The form's `hx-on::after-request` caught the autocomplete GET's bubbled event. | Guard on `event.target === this`. | browser: `e2e/tests/ux.spec.ts` / `e2e.spec.ts` composer typing |
| 9 | **Topbar overflowed** at tablet width. | No wrap on the nav/cost row. | `flex-wrap` on `.topbar` / `.nav`. | browser: `e2e/tests/ux.spec.ts` responsive layout |
| 10 | Toggle "off" glyph **failed WCAG AA contrast**. | `--subtle` on `--bg` is below 4.5:1. | Use `--dim` for the off glyph. | browser: `e2e/tests/a11y.spec.ts` (axe-core, no A/AA violations) |
| 11 | `my-orchestra -seed` could **fail and write nothing** on a forge that already had skills but no agents. | `seedForge` always pinned the `tdd` skill on the seeded `builder` agent; when skills were pre-populated `tdd` was never seeded, so `Save()` → `Validate()` failed on the dangling agent→skill reference. Found by the code review of #13 (the preserves-existing test set up the trigger state but didn't `Validate()`). | Pin `tdd` only when it exists, so `seedForge` stays valid under any partial state; add the `Validate()` assertion. | unit: `cmd/my-orchestra` `TestSeedForgePreservesExisting` (now asserts `Validate()`; fails without the fix) |
| 12 | Markdown renderer mangled **snake_case identifiers in agent prose** — `my_var_name` rendered as `my<em>var</em>name`. | Underscore emphasis matched intraword; CommonMark deliberately doesn't. Caught by the pre-merge code review of #16. | Word-boundary `\b` anchors on the `_`/`__` alternatives only; `*` emphasis stays intraword-capable. | unit: `internal/web` `TestRenderMarkdown` "intraword underscore stays literal" / "double intraword underscore stays literal" |
| 14 | The Telemetry **spend-over-time bars scaled to the all-time busiest day**, not the busiest day in the visible 14-day window. Caught by the pre-merge code review of the 1.3 diff. | `spendTrend` computed `maxUSD` over the full history *before* slicing to the most-recent 14 days, so once history exceeded 14 days an off-screen peak made every visible bar a sliver. | Slice to the visible window first, then compute `maxUSD` over what's shown, so the window always uses the full bar width. | unit: `internal/web` `TestTelemetryTrendWindowsAndScalesToVisibleMax` (20-day history; off-screen all-time max; asserts the busiest in-window day fills 100%) |
| 15 | **Enabling an MCP server did nothing** — and two enabled servers with the same/empty name silently dropped one. Caught by the pre-merge review of the 2.2 diff. | The forge `Compile`d `SessionSpec.MCPServers`, but the web/bootstrap translation to the `copilot.SessionSpec` copied system-message/tools/model/effort and **dropped MCP servers entirely**, so no enabled server ever reached the runtime. Separately, the SDK config map was keyed by the non-unique, non-required `Name` (`sdkclient.go`), so a name clash overwrote an entry. | Thread compiled MCP servers through `compiledSpec`→`applyAgentSpec` and `bootstrap.Build` via `web.MCPServerSpecs`; add `copilot.MCPServer.ID`/`Key()` and key the SDK map by the unique id (fallback to Name). | unit: `internal/web` `TestEnabledMCPServerReachesSessionSpec`, `TestMCPServerSpecsConverts`; `internal/copilot` `TestMCPServerKey` |
| 13 | **Agent personas didn't affect chat** — selecting an agent applied only its model/effort/tool-allowlist; its system message, the enabled instructions, and skill prompts never reached the session. `Forge.Compile` produced the full system message but was never called in the web path. | `applyAgentSpec` set model/effort/tools directly from the `Agent` struct and dropped `SystemMessage`; startup built the spec from config without compiling. | Both activation sites (`/agent` and Agents-page select) and the clear path route through `compiledSpec` → `Forge.Compile`, applying the compiled system message; `main.go` compiles the default agent into the initial session. | unit: `internal/web` `TestAgentSystemMessageCompiledOnSelect`, `TestAgentClearCompilesGlobalContext` |
| 16 | **An MCP secret must never be persisted as a literal, nor sent to the runtime unexpanded.** The C1 Env editor (ADR-0020) lets a secret value reference an env var; two failure modes would defeat its whole purpose: writing the raw secret into `forge.json`, or — having stored a `${VAR}` reference — forwarding that literal string to the server when the var is unset (so the MCP server receives `"${VAR}"` as its key and fails opaquely). | A secrets surface that round-tripped the raw value to disk, or a forge→seam translation that copied `Env` verbatim without resolving references, would each be a silent leak / silent breakage. | A secret row persists **only** the `${VAR_NAME}` reference (the masked value field carries the var *name*, never the secret); `web.MCPServerSpecs` resolves `${VAR}` via the `lookupEnv` seam and **omits** a reference that resolves empty (never the literal); the form rejects a malformed secret var name so an unexpandable `${…}` is never written; the page preflight flags an enabled server's unresolved `${VAR}`. | unit: `internal/web` `TestMCPUpdateRoundTripsEnvViaEditor`, `TestMCPServerSpecsResolvesEnv`, `TestMCPCreateRejectsMalformedSecretRef`, `TestMCPPreflightFlagsUnresolvedEnv`, `TestMCPEditFormShowsEnvRows`; browser: `e2e/tests/e2e.spec.ts` "Env editor stores a secret as a masked ${VAR} reference" |

## Testing notes (gotchas that bit us)

- **The demo is one shared in-memory session** (`workers: 1`, `fullyParallel:
  false`). Per-session counters (messages, tools) accumulate across the whole
  suite, so browser assertions on them must be **relative** (read → act → assert
  it increased), never a fixed value.
- The demo must be **self-contained**: anything a browser test drives (forge
  rows, models, effort) has to be seeded in `-demo` mode, not assumed on disk.
- **Adding a nav page is coupled to the e2e suite.** `e2e/tests/e2e.spec.ts`
  asserts `nav` link count `== pages.length`, and several specs iterate the
  `pages` array in `e2e/tests/helpers.ts`. A new top-level page (e.g. Sessions)
  must be added to that array in nav order, or CI's e2e job fails on the count.
- **Don't persist session history app-side — the runtime already does.** The
  Copilot CLI persists each session's conversation on disk; `ListSessions`,
  `GetEvents`, and `ResumeSession` expose it. Resume reattaches and `GetEvents`
  rehydrates the transcript; a resumed session's first turn after a gap pays full
  uncached input (the prompt cache is cold). See [ADR 0002](adr/0002-restore-sdk-session-resume-for-session-pick-start-continue.md).
- **`config.Config.Save()` mutates-then-validates without rollback** (unlike
  ctxforge, which rolls back on an invalid result). A handler that edits config
  fields and then `Save()`s leaves the live, in-memory config dirty if validation
  fails. Edit through `Server.editConfig` (`internal/web/settings.go`), which
  snapshots `*config`, applies, saves, and restores the snapshot on error.
- **An e2e test that mutates shared config must reset it in `afterEach`.** The
  demo is one shared session *and* one shared `config.Config`; a test that sets a
  budget hard cap via the Settings form (to exercise the over-cap gate) would gate
  every later test's sends if it left the cap set. The budget-guardrails spec
  resets the cap to `0` in `afterEach` so it can't leak. Same family as the
  shared-session gotcha above.
- **Lock ordering is `forgeMu` → `s.mu`, never the reverse.** `handleAgentSelect`
  takes `forgeMu` then `applyAgentSpec` (`s.mu`); a handler holding `s.mu` must
  **not** call `editConfig`/`editForge` (which take `forgeMu`) or it can deadlock.
  `handleBudget`'s "raise" path releases `s.mu` before persisting the lifted cap
  through `editConfig`, then re-locks only to read for rendering.
- **The demo spend ledger is shared and append-only across the whole suite.**
  Demo mode seeds an *ephemeral* `telemetry.SpendStore` (empty dir → in-memory,
  never touches a real config dir), but it is one store for the one shared demo
  session, and every demo turn appends to it. So the Telemetry **trend view**
  (spend over time, per-model share) grows as the suite runs — assert on
  *structure* (the "Spend over time" / "Per-model share" sections, that a
  `.trend-row` exists, that the CSV header is present), never on exact figures.
  Same family as the shared-session / shared-config gotchas above.
- **Persist spend through the ledger, not by re-reading the meter.** The live
  `Meter` is process-global and resets on restart; the accountable record is
  `SpendStore` (`<configDir>/spend.json`, append-only, atomic temp-file+rename
  like config). The `EvUsage` reducer appends one `SpendRecord` per turn
  best-effort (a disk error is logged, never surfaced). Don't reconstruct history
  from `Meter.ByModel()` — it only knows this process. See [ADR 0009](adr/0009-persisted-spend-history-append-only-ledger.md).
- **Three sources now: session meter (statusline) · process meter (live token
  split) · ledger (account-wide budget accounting).** `Server.sessionMeter` scopes
  the **statusline** to *this* conversation (item 3.2 / ADR-0011); the
  process-global `s.meter` backs the **live token split** (cache-write / reasoning
  display counts, cache-hit rate); and the **persisted ledger** (`SpendStore`, read
  via `s.monthToDate()` → `telemetry.MonthToDate`) is the source of truth for the
  **account-wide accounting** — the topbar cost footer, `/cost`, the **hard-cap
  projection** baseline (`overCap`), and the Telemetry "Total cost / Monthly budget
  / Remaining" rows — so they **survive a restart** (item A1 / ADR-0016). The
  account-wide read **moved off the process meter onto the ledger**; do not "fix" a
  zeroed-after-restart row by pointing it back at `s.meter.Totals()`. The shared
  **`recordUsage`** helper (`session.go`, used by both the chat `EvUsage` reducer
  and `workflow.go handleRunEvent`) records every turn into **both meters AND** the
  ledger — drop the `sessionMeter.Record` and the statusline goes stale; drop the
  ledger append and the budget rows reset on restart; drop the `s.meter.Record` and
  the live token split stops moving. Build the session meter on the **account
  meter's price book** (`telemetry.NewMeter(h.meter.PriceBook())`) or per-session
  credits/estimates drift from the global gauge. Tests that drive the budget past a
  threshold must fold spend into **all three** sources (see `recordSpend` in
  `budget_test.go`, which now also appends to the ledger), since the footer/cap read
  the ledger while the statusline reads the session meter. In the **single shared
  demo session** all three see consistent records, so e2e/relative assertions are
  unaffected — same family as the shared-session / shared-config / shared-ledger
  gotchas above. Guarded by `internal/web` `TestStatlineScopesTotalsToThisSession`,
  `TestTelemetryPageStaysAccountWide`, `TestBudgetRowsSurviveFreshMeter`,
  `TestCapBaselineReadsLedger`, `TestCostFooterReadsLedger`,
  `TestRecordUsageHitsBothMetersAndLedger`; `internal/telemetry` `TestMonthToDate`.
- **Spend attribution rides the same `recordUsage`; the active agent id is mirrored
  under `s.mu`, not read from `config`.** Each turn is tagged additively (schema v2)
  with the agent (and workflow id + lane index when a run owns it) via a `spendTag`
  passed to `recordUsage` (item A2 / ADR-0018). The chat reducer passes
  `s.agentID`; the workflow reducer passes `run.id` + `lane.AgentID`/`lane.Index`.
  **Do not** read `config.DefaultAgent` inside `recordUsage` to get the agent — that
  inverts the established `forgeMu → s.mu` lock order (it runs under `s.mu`) and
  races the shared config; `Server.agentID` is the `s.mu`-guarded mirror, seeded from
  `config.DefaultAgent` at `newSession` and updated in `applyAgentSpec` (the single
  session-restart point). The schema bump is **additive** — keep the fields
  `omitempty` and never rename; a v1 ledger must still load (empty tags) and v1
  readers must tolerate `version:2`. The CSV header appends `agent,workflow,lane`
  at the **end** — never reorder the pre-v2 columns. `WorkflowShares` **excludes**
  non-workflow spend (its fraction is of orchestrated spend, not the grand total);
  `AgentShares` **includes** the empty-agent bucket (built-in chat) — don't
  "normalize" the two to the same denominator. Guarded by `internal/web`
  `TestUsageTagsActiveAgent`, `TestWorkflowUsageTagsWorkflowAndLane`,
  `TestNewSessionSeedsActiveAgentFromConfig`, `TestTelemetryPageShowsAttributionBreakdown`;
  `internal/telemetry` `TestSpendRecordRoundTripsAttributionTags`,
  `TestSpendStoreReadsV1RecordWithoutTags`, `TestAgentShares`, `TestWorkflowSharesExcludeNonWorkflowSpend`.
- **The burn-rate forecast is a fourth pure *reader* over the same ledger — not a
  fourth source.** `telemetry.Forecast(DailyTotals, Budget, now)` projects when the
  monthly allowance is exhausted (item A3 / ADR-0019); it reads the **ledger** like
  the other account-wide rows (via `Server.forecast(now)` → `DailyTotals`), never
  the process/session meter — don't "fix" a forecast by pointing it at
  `s.meter`. Three gotchas it bit on: (1) the slope **divides window credits by
  elapsed observed days** (the 7-day window *clamped to ledger age*), so a
  single-day/new ledger isn't divided by a mostly-absent week (→ near-zero rate)
  while idle days inside an established history still drag it down — don't switch
  the denominator to "days that have records." (2) the displayed "~N days" uses
  `⌈DaysToCap⌉` to **match** `ExhaustionDate = today + ⌈DaysToCap⌉`; rounding the
  count independently prints "~9 days" beside a +10-day date. (3) thread **one
  `now`** per render into both `Forecast` and `forecastSoon` (the amber predicate)
  so the date and the warn can't disagree across a month boundary. Degenerate cases
  are explicit in `Projection.Status` (no-budget / idle / exhausted / ok) — the
  statusline cell shows **only** `ProjectionOK`; the Telemetry page explains the
  rest. Like `MonthToDate`, the read is O(n) on the render path (caching deferred,
  ADR-0016/0019). The shared **demo ledger** is append-only across the suite, so
  e2e asserts the **"Forecast" label**, never figures (same family as the
  shared-ledger gotcha above). Guarded by `internal/telemetry` `TestForecast`,
  `TestForecastDeterministic`; `internal/web` `TestTelemetryPageShowsForecast`,
  `TestTelemetryForecastNoBudgetHint`, `TestStatlineShowsForecastCell`,
  `TestStatlineNoForecastCellWithoutBudget`.
- **The MCP page's PATH preflight is the one impurity — keep it behind the seam.**
  Whether a curated stdio server's `Command` (`npx`/`uvx`) resolves depends on the
  host, so `mcpServersPartial` calls `exec.LookPath` through the `s.lookPath` seam
  (defaulted in `Hub.New`, copied to each `Server`). Unit tests inject a fake
  `s.lookPath` to assert the **unavailable** badge deterministically
  (`internal/web` `TestMCPPagePreflightMarksUnavailable`); never assert on real
  `LookPath` output (it differs across CI runners). Same reason the e2e MCP
  spec asserts on *structure* (rows, toggle state, add), not on badge presence — in
  CI `npx` may be present and `uvx` absent. Curated servers are seeded **disabled**
  precisely so an absent binary can't surprise-fail a session at start.
- **MCP secret resolution rides a second seam, `s.lookupEnv` (ADR-0020).** Whether a
  `${VAR}` reference resolves depends on the host environment, so both the forge→seam
  translation (`web.MCPServerSpecs` → `resolveEnv`) and the secrets preflight
  (`mcpServersPartial` → `missingEnvRefs`) read the env through the `s.lookupEnv` seam
  (defaulted to `os.Getenv` in `Hub.New`, copied to each `Server`; `SeamSpec` takes it
  as a parameter so the bootstrap path passes `os.Getenv` explicitly). Unit tests inject
  a fake `s.lookupEnv` / a lambda to assert resolution and the **missing-key** badge
  deterministically; never assert on the real environment. The `${VAR}` reference shape
  is decoded only in `internal/web` (`envRef`), so `ctxforge`/`config`/`telemetry` stay
  dependency-free and store the reference as opaque data.
- **The diff review lane keys off a hunk header, not any leading `+`/`-`.**
  `parseUnifiedDiff` (`internal/web/diff.go`) reports `OK` only when it finds a
  `@@ … @@` hunk header, so prose that merely starts lines with `-` (a markdown
  bullet) or `+` can't hijack an ordinary permission summary into the review lane.
  `renderPermForm` falls back to the compact form when `OK` is false (including a
  write request with no parseable diff). Untrusted file content in the diff is
  HTML-escaped by `html/template` like all model text (ADR-0001). Guarded by
  `internal/web` `TestParseUnifiedDiffRejectsNonDiff`,
  `TestRenderPermFormReviewLane` (asserts `&lt;old&gt;` escaping),
  `TestRenderPermFormWriteWithoutDiffStaysCompact`.
- **Adding a permission to the demo surfaced the reject button to the a11y scan.**
  The diff review lane (item 3.1) added a file-write `EvPermission` to
  `streamDemoReply`, so the a11y chat scan now sees the reject button (`.no`,
  white-on-red) for the first time — it joins the documented destructive-control
  contrast baseline (`KNOWN_CONTRAST_SELECTORS` in `e2e/tests/a11y.spec.ts`),
  same family as `.abort`/`.plan-reject`/`.elicit-no`. The lane's diff body is
  fully AA (per-line tint + `+`/`-` marker; foregrounds on AA-safe tokens, never
  `--bad` red as text). The chat scan waits for `#perms .perm-review` so the lane
  is deterministically covered, not raced. The demo perm has a fixed id, so (like
  the demo ask/plan/elicit forms) it accumulates across the shared session —
  browser assertions use `.last()`.
- **A workflow run owns the turn; its sub-runs' events route to lanes, not the
  chat transcript.** While `s.run != nil && !s.run.done`, `handleEvent` dispatches
  to `handleRunEvent`, which attributes each event to a lane **by copilot
  `SessionID`, falling back to the sole running lane** when the id is empty (the
  case where a sequential run has exactly one lane active). The `MockClient` now
  hands out **distinct** ids per `CreateSession` (`mock-session-N`) and the demo
  lane (`streamDemoLane`) tags its events with the lane's id, so a **parallel** run
  drives concurrent lanes offline too (B1/ADR-0017) — the empty-id fallback is no
  longer the only offline path. **Don't revert `CreateSession` to a constant id**:
  two concurrent lanes would then share one id and `laneFor` could not disambiguate
  them. Normal `/send` is **refused with a note** during a run (it doesn't queue
  behind the lanes), and `/clear` resets `s.run`. Adding the Workflows nav page also
  touched the `pageNames`/e2e-`pages` count coupling (below). Guarded by
  `internal/web` `TestRun*` (pure engine, both modes), `TestWorkflowRunReducerSequential`,
  `TestWorkflowRunReducerParallelRoutesBySessionID`, `TestParallelDemoRunDrivesConcurrentLanes`,
  `TestWorkflowLanesEscapeModelText`, `TestSendBlockedDuringWorkflowRun`;
  `internal/copilot` `TestCreateSessionReturnsDistinctIDs`; `internal/ctxforge`
  `TestCompileWorkflow`, `TestForgeValidateWorkflowAgentReference`.
- **A lane surfaces its own tool timeline + inline permissions, not just output.**
  `handleRunEvent` reduces a lane's `EvToolStart/Progress/End` + `EvPermission` onto
  per-lane state (`lane.tools`/`lane.perms`); `renderLanes` reuses the chat
  `renderToolCard`/`renderPermForm` so a sub-run's tools and a file-write diff
  review render identically inside the lane card. A lane permission is answered over
  the **shared `/perm/{id}` route**: `handlePerm` is lane-aware — it drops the
  request from whichever lane holds it (`dropLanePerm`) and refreshes `#lanes`
  out-of-band instead of the timeline. Lane permissions are **per-lane**, not a
  cross-lane FIFO (ADR-0017). Don't drop the `handlePerm` lane branch or a lane
  permission would respond on the seam but never clear from the panel. Guarded by
  `internal/web` `TestWorkflowLaneSurfacesToolTimeline`, `TestWorkflowLaneInlinePermission`,
  `TestStreamDemoLaneTagsSessionID`.
- **Workflow lane text (output, tool args/results, permission diffs) is
  model/forge-originated → HTML-escaped.** Agent names and a lane's streamed output
  reach the browser through the same escaped paths as chat (committed lane output
  via the server-side markdown renderer; names/detail via `richtext`/`html/template`
  auto-escaping); per-lane tool cards and permission forms are composed from the
  already-escaping `renderToolCard`/`renderPermForm` before the lane card wraps them
  with `trusted()` — so the `trusted()` wrap is safe only because each fragment
  self-escaped (ADR-0001). Guarded by `internal/web` `TestWorkflowLanesEscapeModelText`,
  `TestWorkflowLaneToolTextEscaped`.
- **A branching step is *skipped*, not failed, and a skipped lane still terminates
  the run.** A `WorkflowStep.When` predicate (B2 / ADR-0021) gates a step on a
  strictly-prior step's settled outcome; an unsatisfied step becomes `laneSkipped`
  (a fifth terminal lane status), **not** `laneFailed`. Two invariants the engine
  must keep: (1) `settled()` (and thus `allSettled()`) counts `laneSkipped` as
  settled, so a run whose last lane skips still reaches `done` — **don't** treat skip
  as a non-terminal/pending state or the run hangs; and (2) the `When.Step` reference
  is validated to be **strictly prior** (1-based, `[1, i]`), which forbids
  self/forward references and makes a cycle structurally impossible — so `evalWhen`'s
  `r.lanes[Step-1]` is always in range and the run always terminates (no graph walk
  needed). A sequential **hard failure** still aborts the chain (ADR-0013 semantics);
  a skip does not. `failLane` returns `[]int` (not a bool) so a **parallel** failure
  can unblock a `when failed` lane — don't revert it to a bool or that branch never
  launches. **Form parser:** the steps textarea splits a line on the colon **after**
  the optional `[predicate]` bracket (`splitStepLine`), so an `output-contains` value
  containing a colon round-trips — don't go back to `strings.Cut(line, ":")` on the
  first colon (it truncates the predicate and mangles the prompt). Guarded by
  `internal/web` `TestRunSequentialSkipsUnsatisfied`, `TestRunParallelGatedRunsAndSkips`,
  `TestRunParallelFailUnblocksWhenFailed`, `TestBranchingDemoRunSkipsLane`,
  `TestWorkflowStepConditionRoundTrip`, `TestWorkflowFormRoundTripsCondition`;
  `internal/ctxforge` `TestWorkflowValidateWhen`, `TestCompileWorkflowCarriesWhen`.
- **A workflow run is persisted to history exactly once, on completion — and a
  skipped lane persists with no cost.** Run history (B3 / ADR-0022) lives in a
  **separate** `telemetry.RunStore` (`<configDir>/runs.json`, a sibling of the spend
  ledger — same versioned-envelope + atomic temp-file+rename discipline), **not** in
  the spend ledger: a spend row is "one metered turn," which can't represent a run's
  start/finish/outcome nor a **branched lane that was skipped and so incurred no turn**
  (the exact case ADR-0021 made first-class). The web adapter records a run in
  `runFrags(run, done=true)` — the **one** completion point, reached **exactly once**
  per run (after `run.done` flips, `handleEvent` stops routing to `handleRunEvent`, so
  `busy` clears there too). **Don't** move `recordRun` earlier (a run could be written
  twice or half-finished) or into a per-lane settle (a half-written run is not a
  history row). The append is **best-effort** under `s.mu` (a disk error is logged, not
  surfaced) and the store's `mu` is a **leaf** lock (never calls back into `s.mu`) —
  same shape as the spend `recordUsage` append, so no new lock-order risk. `recordRun`
  is a no-op when no store is wired (nil-safe). A skipped lane keeps its compiled
  `AgentID` (so history shows the agent that *would* have run, never "chat (built-in)")
  and persists `status:"skipped"` with zero `credits`. The **glyph mapping is one
  source of truth** (`glyphFor(status string)`): `laneGlyph` (live) routes through it
  via `laneStatusName`, and the Runs page uses it directly — so a live lane and a
  historical lane can't drift. Adding the **Runs** top-level page also touched the
  `pageNames` / e2e `pages` count coupling (above). Guarded by `internal/web`
  `TestWorkflowRunRecordedOnceSequential`, `TestWorkflowRunRecordedOnceParallel`,
  `TestWorkflowRunRecordsSkippedLane`, `TestWorkflowRunRecordingNoStoreNoPanic`,
  `TestRunsPartialRendersStructure`, `TestRunsPartialEmpty`; `internal/telemetry`
  `TestRunStoreAppendPersistsAndReloads`, `TestRunStoreEphemeralNeverWrites`,
  `TestLoadRunStoreRejectsCorruptFile`, `TestLoadRunStoreToleratesNewerSchema`,
  `TestRunRecordCarriesSkippedLane`, `TestRunStoreStampsFinishedAtWhenZero`.
- **Keyboard shortcuts are config-backed and dispatched client-side, but the
  keymap is computed server-side.** The rebindable action set is fixed in code
  (`config.KeyActions()`); only overrides persist (`Config.KeyBindings`,
  omitempty). `handleIndex` renders the resolved action→key map onto `<body
  data-keymap>` (JSON, auto-escaped) and the body-level `#help-overlay`; a small
  `keydown` handler dispatches it. **Two guards the handler must keep:** ignore
  keystrokes when the target is an `INPUT`/`TEXTAREA`/`SELECT`/contenteditable
  (so typing the bound chars in the composer is text, not actions — `e2e/tests/
  keybindings.spec.ts` "ignored while typing"), and ignore ctrl/meta/alt-modified
  keys. **Validation is pure** (`config.validateKeyBindings`): single-char key,
  known action id, no duplicate key — a colliding bind is rolled back by
  `editConfig`, never half-applied (`internal/web` `TestSettingsSaveRejectsDuplicateKey`).
  A rebind applies on the **next full page load** (TECH_DEBT #13), so an e2e test
  must reload to observe a saved keymap — the specs only *read* shared config, so
  no `afterEach` reset is needed (cf. the shared-config gotcha). Esc-closes-overlay
  is a **fixed convention, not a binding**. Guarded by `internal/config`
  `TestKeymap*`/`TestKeyBinding*`, `internal/web` `TestIndexRendersKeymapAndOverlay`/
  `TestHelpPageListsShortcuts`/`TestSettingsFormHasKeybindingFields`/
  `TestSettingsSaveAppliesKeybindingOverride`.
- **Snippets share the `/` namespace with commands — built-ins always win.** A
  prompt/snippet library entry (item 3.4) surfaces in the composer autocomplete by
  its `id` as a `/trigger`, but a snippet whose id equals a built-in command or a
  nav-page slug must never shadow it. `isReservedCommand` (`internal/web/
  autocomplete.go`) gates this at **both** menu time (`matchSnippets` skips a
  reserved id) and submit time (`snippetExpansion` returns false for a reserved
  name, so `handleSend` falls through to `runCommand`). Drop either guard and a
  user could redefine `/clear`. The snippet **body** reaches the browser only as an
  `html/template`-escaped `data-body` attribute and is read back as a plain string
  by `fillSnippet` (never parsed as HTML) — ADR-0001 escape-first holds. Guarded by
  `internal/web` `TestReservedCommandBeatsSnippet`, `TestCommandsMenuEscapesSnippetBody`,
  `TestSlashSnippetExpandsAndSends`, `TestUnknownSlashIsNotSent`. The demo seeds
  snippets (`review-pr`, `explain`) so the page/autocomplete are self-contained
  offline; e2e asserts the insert path, not exact figures — same family as the
  self-contained-demo gotcha above.
- **The composer is a `<textarea>` — Enter sends, Shift-Enter is a newline, and an
  inserted multi-line snippet keeps its line breaks.** Item C2 (issue 0018) switched
  the composer from a single-line `<input name="prompt">` to a `<textarea
  name="prompt">`, paying down TECH_DEBT #15 (an `<input type=text>` value-sanitised
  away newlines, flattening a multi-line `fillSnippet` insert). **The old flatten
  gotcha inverts:** a textarea preserves newlines, so the snippet-insert test now
  *asserts* the line breaks survive (`e2e.spec.ts` "inserts a multi-line snippet
  keeping its line breaks"; the server has always carried the newlines in the
  `data-body` attribute — `TestCommandsMenuPreservesMultilineSnippetBody`). Three
  things the textarea must keep coexisting: (1) a textarea inserts a newline on Enter
  natively, so `composerKeydown` intercepts a **bare** Enter and `form.requestSubmit()`s
  (firing the `submit` event htmx hooks) while Shift-Enter / any modifier / IME
  composition falls through to a newline; (2) the form's `hx-on::after-request` keeps
  the REGRESSIONS #8 `event.target === this` guard (don't wipe the field on the
  autocomplete GET) and now refocuses the **textarea** (`querySelector('textarea')`,
  not `'input'`); (3) the body-level `keydown` dispatcher already skips
  `INPUT/TEXTAREA/SELECT/contenteditable`, so typing a bound shortcut char in the
  composer stays text, not an action (`ux.spec.ts` "typing a bound keybinding char …
  is text"). `autosize` accounts for the `border-box` borders `scrollHeight` omits
  (else a permanent 1–2px scrollbar). All four former `#composer input[name=prompt]`
  selectors route through one `composer()` helper. Guarded by `internal/web`
  `TestComposerRendersTextarea` / `TestComposerKeydownAndAutosizeWired` /
  `TestCommandsMenuPreservesMultilineSnippetBody`.
- **Go's `regexp` is RE2 — no backreferences.** A pattern like `([-*_])( *\1){2,}`
  panics at `MustCompile` ("invalid escape sequence: \1"). For repeated-char
  matching (e.g. the markdown horizontal rule), scan the string directly
  (`isHR()` in `internal/web/markdown.go`) instead of a backreference.

## Dead-ends (tried/considered, rejected — don't redo)

| Approach | Why it failed / was rejected | Do instead |
|----------|------------------------------|------------|
| Client-side markdown (JS web component + sanitizer) | Makes rendering browser-only-testable, undercutting the core "reduce + project, unit-tested with no browser" goal; needs a vendored JS lib + client sanitization. | Server-side safe-subset renderer, escape-first. See [ADR 0001](adr/0001-render-markdown-server-side-for-committed-agent-turns.md). |
| Markdown via `gomarkdown` + `bluemonday` | Pulls transitive deps (x/net, aho-corasick, css) into a localhost single-user tool for a bounded need. | In-house subset renderer; zero deps. See ADR 0001. |
| Client-side state reducer (SPA framework) | Duplicates the transcript reducer client-side + forces a JSON API and build chain. | Server owns all state; htmx projects SSE fragments. |
| Dual frontend (keep TUI alongside web) | Two reducers/renderers to maintain. | Hard cut — `internal/tui` deleted; web is the only frontend. |
| htmx from a CDN | Offline/single-binary tool can't depend on a CDN. | Vendor htmx + htmx-ext-sse under `internal/web/static/`. |

## Known gaps (fixed behavior, not yet guarded — or not yet built)

These are tracked in the project roadmap memory; listed here so the lack of a
guard is visible.

- _(All validated-backlog items #4/#7/#8/#9 are now built and guarded. The one
  deferred piece — the Claude folder/`SKILL.md` skill model — is tracked in
  [TECH_DEBT.md](TECH_DEBT.md) item 1, not here.)_
