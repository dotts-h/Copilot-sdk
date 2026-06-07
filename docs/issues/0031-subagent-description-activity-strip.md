---
id: 0031
title: Surface SubagentInfo.Description on the sub-agent activity strip (roadmap v5, item V3)
status: closed
severity: medium
group: 0030
github:
links:
  adr:
  prs:
  issues: [0030]
  regression:
assets: []
---

## Summary

During a parallel / multi-agent run the chat shows a **sub-agent activity strip** — one
animated chip per concurrent sub-agent (a spinner + the agent's display name + its model).
The SDK already populates each sub-agent's `Description` (`internal/copilot/normalize.go`
maps `d.AgentDescription` → `SubagentInfo.Description`; the field exists on
`copilot.SubagentInfo`), but `renderSubagents` (`internal/web/render.go`) drops it — it
builds the `subagentChip` fragment with only `Label` + `Model`. So a user watching three
concurrent sub-agents sees their names but not *what* each is doing. **V3 surfaces the
description** as the chip's `title=` tooltip so concurrent sub-agents during a parallel run
say what they're doing. **No SDK change, no new store, no schema change** — a pure
web-layer render change over an already-populated field. Source: `docs/NEXT_FEATURES.md`
item V3.

## Repro
1. Run a parallel workflow (or the chat demo, which seeds one scripted sub-agent with a
   description) and watch the sub-agent activity strip while sub-agents run.
   - **Expected:** each chip surfaces *what* the sub-agent is doing (its description), so
     concurrent sub-agents are distinguishable by task, not just by name.
   - **Actual (before V3):** the chip shows only the display name + model; the
     SDK-populated `Description` is dropped on the floor by `renderSubagents`.

## Proposed resolution

- **`internal/web` (sub-agent strip):** `renderSubagents` adds the sub-agent's
  `Description` to the `subagentChip` fragment map (passed raw, auto-escaped by
  `html/template` exactly like `Label`); the `subagentChip` template renders it as a
  `title=` tooltip. An **empty description renders the prior chip** (the template omits the
  `title` attribute entirely). All values flow through `html/template` (ADR-0001) — the
  description is model/SDK-originated text and must never be `trusted()` raw.
- **No SDK change** — `SubagentInfo.Description` and its population in `normalize.go`
  already exist; a normalize test asserting `AgentDescription` → `Description` is added.
- **No new store, no schema change, no telemetry/ctxforge/config touch.**
- **Tests:** web — a sub-agent with a description renders it in the chip's `title=`; an
  empty description renders no `title` (prior shape); the description is HTML-escaped (seed
  `<b>`/`&` and assert `&lt;b&gt;`/`&amp;`, mirroring `TestWorkflowLanesEscapeModelText`).
  copilot — `normalize` maps `AgentDescription` → `Description`. e2e — the sub-agent-strip
  spec asserts the description **structure** (a non-empty `title` attribute) appears on a
  chip during a run, never exact figures; the existing "doesn't leak a chip after settle"
  assertion is kept.

## Resolution (shipped)

Built as specified — a pure web-layer render change over the already-populated
`SubagentInfo.Description`, no SDK/store/schema change. `internal/web` (`render.go`):
`renderSubagents` adds `"Description": sa.Description` to the `subagentChip` fragment map
(passed raw, auto-escaped by `html/template` in attribute context exactly like `Label`).
The `subagentChip` template (`templates/fragments.html`) gained
`{{if .Description}} title="{{.Description}}"{{end}}` — the chip's tooltip — so a non-empty
description surfaces *what* the sub-agent is doing and an **empty description renders the
prior chip** (no `title` attribute). The description flows through `html/template`
auto-escaping (ADR-0001), never `trusted()` raw.

Tests: web (`internal/web/subagent_test.go`) — `TestSubagentChipSurfacesDescription` (a
seeded description renders as `title="…"`), `TestSubagentChipEmptyDescriptionKeepsPriorShape`
(no `title` attribute when the description is empty), `TestSubagentChipEscapesDescription`
(a `<b>…</b> &` description renders `&lt;b&gt;…&lt;/b&gt;` + `&amp;`, mirroring
`TestWorkflowLanesEscapeModelText`). copilot (`subagent_test.go`):
`TestHandlerMapsSubagentLifecycle` now also asserts `AgentDescription` → `Description`.
e2e (`e2e/tests/e2e.spec.ts`): a new strip spec asserts the chip carries a non-empty
`title` attribute **structure** during a run (the demo already seeds a sub-agent with a
description; its dwell was widened so the strip is observably visible), keeping the
pre-existing "doesn't leak a chip after settle" assertion. Gates green
(`make lint && make test`, web coverage 88.8%); the e2e Chromium browser is blocked by the
env's network allowlist, so the spec was verified to compile/discover via
`npx playwright test --list` and CI runs the real Playwright suite.

Docs: CONTRACTS §2 (the `SubagentInfo` shape + the activity strip surfacing `Description`
as an escaped `title=` tooltip). No REGRESSIONS entry — no bug was found-and-fixed; the
escape path and the empty-description prior-shape were guarded preemptively (self-review
with `/code-review` high effort confirmed the escape path, the empty-description shape, and
that no `trusted()` raw wrap slipped in). No ADR. Shipped on branch
`claude/subagent-description`. **First child of epic 0030 (roadmap v5).**

## Notes
- **No ADR:** a pure presentation-layer surfacing of an already-populated SDK field, like
  the v4 pure-reader compositions. Captured in CONTRACTS §2 (the `SubagentInfo` shape; the
  strip surfaces `Description` as a `title=` tooltip, escaped per ADR-0001). A REGRESSIONS
  entry is added **only if** a real bug/gotcha is found-and-fixed.
- **Differentiator:** sharpens the orchestration surface — a parallel run's concurrent
  sub-agents become legible by *task*, not just by name. First child of epic 0030
  (roadmap v5 — orchestration visibility & polish).
- **Numbering:** issue **0031** (next free after 0029), first build of epic **0030**
  (roadmap v5). No ADR consumed.
