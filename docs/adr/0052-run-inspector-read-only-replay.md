# 0052. Run inspector — read-only step-timeline reconstruction over the per-run event log

- Status: accepted
- Date: 2026-06-12
- Deciders: Horia
- Related: issue 0091 (epic 0090), [ADR-0048](0048-per-run-event-log-replay-vs-summary.md), [ADR-0022](0022-workflow-run-history-sibling-append-only-run-store.md), [ADR-0001](0001-markdown-rendering-escape-first-whitelist.md), [ADR-0026](0026-grouped-sidebar-nav-by-intent.md), `internal/web/runs.go`, `internal/web/hub.go`, `internal/telemetry/eventlog.go`

## Context

ADR-0048 shipped the per-run **event log** (`telemetry.RunEventLog`, one
`eventlogs/<runID>.json` per run) — replay *data* with **no replay surface**: the Hub's
`pump` writes it, and nothing reads it. The roadmap-v15 deep-research pass (epic 0090) found
the external run-detail pattern fully settled: a **master/detail step timeline** (one row per
load-bearing step; the full input/output behind a disclosure), with local-first precedents
(Phoenix on localhost; claude-code-log / claude-replay rendering JSONL transcripts as static
HTML) proving the minimum viable form is *a renderer over an append-only log* — exactly what
we persist. This ADR records the contract for that reader: the **run inspector**.

## Decision

A new **read-only** page route `GET /page/runs/{id}` renders a run's event log as a
**lane-grouped step timeline**. It is the first *parameterized* page route; it stays outside
`pageNames` (no sidebar entry, no palette entry — reached from a link on each Runs-page row),
so the nav/palette contract (ADR-0026) is untouched.

### Read-only reconstruction — never re-execution

The inspector **reconstructs** a past run from its log; it never re-runs anything, never
touches the live run state (`s.mu`), and never mutates the store. True time-travel /
fork-from-step is explicitly out of scope (it needs per-step state checkpoints — ADR-0048
already decided replay = read-only reconstruction). The route reads `s.runs` for the run's
summary header and `telemetry.LoadRunEventLog` for the timeline; lane labels resolve under
`forgeMu` (the same lock discipline as `runRow`).

### The step vocabulary

One row per **load-bearing** event, a fixed whitelist aligned to the OTel GenAI step names
as a *guide only* (the spec is still Development status — no SDK plumbing is adopted):

- `EvUserMessage` → a user-message step;
- committed `EvMessage` / `EvReasoning` → a message / reasoning step (the **committed**
  event is the record);
- `EvToolStart` + `EvToolEnd` → **joined into one tool step** (args from the start, result +
  success from the end, in one disclosure);
- `EvPermission`, `EvToolDecision`, `EvError`, `EvSubagentStart` / `EvSubagentEnd`,
  `EvCompactionStart` / `EvCompactionEnd` → one step each.

Everything else is **coalesced away**: the streaming deltas (`EvMessageDelta`,
`EvReasoningDelta`, `EvToolProgress`) and the non-load-bearing lifecycle events
(`EvIdle`, `EvUsage`, `EvContextWindow`, `EvHookRun`, …) never render — the committed event
is the record, so the deltas that built it are noise in an audit view.

### Tool join is heuristic (the persisted record carries no call id)

`telemetry.RunEvent` (ADR-0048) persists `Tool`/`Args`/`Result`/`Success` but **not** a
tool-call id. The join therefore matches each `EvToolEnd` to the most recent still-open
`EvToolStart` **in the same lane** preferring an equal tool name, falling back to the most
recent open start in that lane. An unmatched start (a tool that never returned — a crashed or
in-flight run) renders as a tool step with no result; an unmatched end renders on its own.
This is a reconstruction heuristic, documented here so a future call-id field (a clean,
additive upgrade) can make the join exact.

### Escaping (ADR-0001)

Every reconstructed string — message/reasoning text, tool args, tool result, error text —
is escaped by `html/template`'s contextual auto-escaping inside the timeline template (the
disclosure bodies sit in `<pre>`, so newlines are preserved without a `<br>` transform). A
hostile tool result or message **cannot inject markup**.

### Graceful degradation

- A run with **no log file** (a pre-0085 run, or `EventLogDir` disabled) renders the summary
  card + a "no event log" note — **no error, no 404**.
- An **unknown run id** 404s cleanly (`http.NotFound`).
- The on-disk contract stays as ADR-0048 left it: this is a pure reader, **no schema change**.

## Consequences

- The event log stops being write-only — the v6 "store one renderer away from being seen"
  finding is closed for runs. This is the keystone for O2 (price the timeline — additive
  per-step usage/credits) and O3 (transcript view — the same events in chat reading order
  through the block-AST renderer).
- **Zero new JS, zero new runtime dependency:** the disclosure is a native `<details>`; the
  page is a server-rendered fragment swapped into `#main` like every other page. The live
  SSE/record path stays byte-identical when the inspector is never opened.
- The demo seeds one run's event log (to a temp dir) so the inspector renders a real timeline
  offline; the other demo runs exercise the "no event log" degradation path.
