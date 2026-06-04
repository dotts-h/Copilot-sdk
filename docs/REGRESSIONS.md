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

## Testing notes (gotchas that bit us)

- **The demo is one shared in-memory session** (`workers: 1`, `fullyParallel:
  false`). Per-session counters (messages, tools) accumulate across the whole
  suite, so browser assertions on them must be **relative** (read → act → assert
  it increased), never a fixed value.
- The demo must be **self-contained**: anything a browser test drives (forge
  rows, models, effort) has to be seeded in `-demo` mode, not assumed on disk.

## Known gaps (fixed behavior, not yet guarded — or not yet built)

These are tracked in the project roadmap memory; listed here so the lack of a
guard is visible.

- **Markdown rendering** — deferred (not built). No guard yet.
- **Editable Settings → configure the SDK from the UI** — deferred. The page is
  still read-only; no mutation guard.
- **Session pick / start / continue** — deferred. No guard.
- **Claude-CLI-style skills/agents + a default chat agent** — deferred. No guard.
