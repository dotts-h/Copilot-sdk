---
id: 0091
title: "Run-detail page — lane-grouped step timeline over the per-run event log (O1)"
status: closed
severity: medium
group: 0090
depends_on: []
github:
links:
  adr: [0052]
  prs: []
  issues: [0090]
  regression:
assets: []
---

## Summary

The per-run event log (ADR-0048, issue 0085) is write-only: nothing reads
`eventlogs/<runID>.json`. Add the read surface — a `GET /page/runs/{id}` drill-down (each
`runRow` header on the Runs page links to it) rendering the log as a lane-grouped **step
timeline**: one row per load-bearing event (`EvUserMessage`, committed
`EvMessage`/`EvReasoning`, `EvToolStart`+`EvToolEnd` joined into one tool step,
`EvPermission`, `EvToolDecision`, `EvError`, `EvSubagentStart/End`,
`EvCompactionStart/End`) with type glyph + name + lane + clock time, and the full
args/result/text behind a `<details>` disclosure — master/detail with **zero new JS**.
Deltas (`EvMessageDelta`/`EvReasoningDelta`/`EvToolProgress`) are coalesced away (the
committed event is the record). **Read-only reconstruction — never re-execution.**

## Why now

The v6 finding again: the store holds data that can't be seen, one renderer away. The
external pattern (master/detail timeline; LangSmith/Langfuse/AgentOps/OpenAI tracing, and
the claude-code-log/claude-replay renderer-over-JSONL precedent) is settled, so design risk
is ~zero. Keystone for O2 (pricing) and O3 (transcript).

## Touches

- `internal/web` — `runs.go` (`runDetailPartial`, `handleRunDetail`, a link on `runRow`),
  `hub.go` (route `GET /page/runs/{id}` beside the existing runs routes),
  `templates/fragments.html` (`runDetailPage` block), `static/app.css` (`.inspector`).
- `internal/telemetry` — read side only (`LoadRunEventLog` exists; no schema change).
- `docs/adr/0052-run-inspector-read-only-replay.md` — written first (ADR-0004): the
  inspector contract (read-only reconstruction; the step vocabulary, aligned to the OTel
  GenAI names as a guide).

## Acceptance

- [x] Each run row on the Runs page links to its detail page; the page renders the
      timeline grouped by lane, newest-run-vocabulary consistent with `runRow`
      (status glyphs, lane labels resolved under `forgeMu`).
- [x] Tool steps join `EvToolStart`+`EvToolEnd` (args + result + success in one
      disclosure); deltas and `EvToolProgress` never render; all text escaped per ADR-0001
      (a hostile tool result/message cannot inject markup).
- [x] A run with no log file (pre-0085 run, or `eventLogDir` disabled) renders the summary
      card + a "no event log" note — no error, no 404.
- [x] An unknown run id 404s cleanly; the route never touches the live run state
      (`s.mu` held only to read, lock order preserved).
- [x] Unit tests cover: timeline grouping/joining, delta coalescing, escaping, missing-log
      and unknown-id degradation; an e2e spec opens a demo run's detail page
      (`e2e/tests/inspector.spec.ts`). `make lint && make test` (floor 65%) green.

## Notes

M-sized. The detail page is the first parameterized page route (`/page/runs/{id}`) — it
stays outside `pageNames` (no sidebar entry; reached from the Runs page), so the nav/palette
contract is untouched. ADR-0052 records the contract.

## Close-out

Built test-first on `claude/get-next-endpoint-e8u4bz` (the harness-mandated branch).

- **Reader:** `internal/web/runs.go` — `handleRunDetail` / `runDetailPartial` / pure
  `buildRunTimeline` (lane grouping, delta coalescing, heuristic `EvToolStart`+`EvToolEnd`
  join — the persisted `RunEvent` carries no call id, so the join matches by lane + name,
  documented in ADR-0052). Route `GET /page/runs/{id}` in `hub.go`; `runDetailPage` block +
  `runRow` name link in `fragments.html`; `.inspector`/`.tstep` styles in `app.css`.
- **Found & fixed an upstream gap:** the per-run event log (ADR-0048) was **never enabled
  outside tests** — `EventLogDir` was unset in `bootstrap.go`, so the writer ran nowhere and
  the reader would have had nothing to read. Wired it in production (`EventLogDir =
  configDir`) and seeded one demo run's log (`seedDemoEventLog`, a temp dir) so the inspector
  renders a real timeline offline; the second demo run exercises the "no event log" path.
- **Tests:** `internal/web/run_detail_test.go` (grouping/joining, unmatched-tool, escaping,
  missing-log, unknown-id 404, mux end-to-end) + `e2e/tests/inspector.spec.ts` (3 specs).
  `make lint && make test` green (web 89.7%); full e2e green.
- Docs folded into the branch (ADR-0004): ADR-0052, CONTRACTS §3 route + narrative, CODEMAP
  regen, this close-out.
