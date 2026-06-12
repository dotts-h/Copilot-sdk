---
id: 0091
title: "Run-detail page — lane-grouped step timeline over the per-run event log (O1)"
status: open
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

- [ ] Each run row on the Runs page links to its detail page; the page renders the
      timeline grouped by lane, newest-run-vocabulary consistent with `runRow`
      (status glyphs, lane labels resolved under `forgeMu`).
- [ ] Tool steps join `EvToolStart`+`EvToolEnd` (args + result + success in one
      disclosure); deltas and `EvToolProgress` never render; all text escaped per ADR-0001
      (a hostile tool result/message cannot inject markup).
- [ ] A run with no log file (pre-0085 run, or `eventLogDir` disabled) renders the summary
      card + a "no event log" note — no error, no 404.
- [ ] An unknown run id 404s cleanly; the route never touches the live run state
      (`s.mu` held only to read, lock order preserved).
- [ ] Unit tests cover: timeline grouping/joining, delta coalescing, escaping, missing-log
      and unknown-id degradation; an e2e spec opens a demo run's detail page.
      `make lint && make test` (floor 65%) green.

## Notes

M-sized. The detail page is the first parameterized page route (`/page/runs/{id}`) — it
stays outside `pageNames` (no sidebar entry; reached from the Runs page), so the nav/palette
contract is untouched. ADR-0052 records the contract.
