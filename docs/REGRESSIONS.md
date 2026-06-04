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
| 13 | **Agent personas didn't affect chat** — selecting an agent applied only its model/effort/tool-allowlist; its system message, the enabled instructions, and skill prompts never reached the session. `Forge.Compile` produced the full system message but was never called in the web path. | `applyAgentSpec` set model/effort/tools directly from the `Agent` struct and dropped `SystemMessage`; startup built the spec from config without compiling. | Both activation sites (`/agent` and Agents-page select) and the clear path route through `compiledSpec` → `Forge.Compile`, applying the compiled system message; `main.go` compiles the default agent into the initial session. | unit: `internal/web` `TestAgentSystemMessageCompiledOnSelect`, `TestAgentClearCompilesGlobalContext` |

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
