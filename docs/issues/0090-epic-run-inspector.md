---
id: 0090
title: "Epic: Run inspector — replay/audit surface over the per-run event log (roadmap v15)"
status: closed
severity: medium
group:
depends_on: []
github:
links:
  adr: [0052]
  prs: []
  issues: [0091, 0092, 0093, 0094]
  regression:
assets: []
---

## Charter

v14's ADR-0048 shipped the per-run event log — replay *data* with **no replay surface**:
`telemetry.RunEventLog` is written on every run (`hub.go` `pump` → `appendRunEvent`, one
`eventlogs/<runID>.json` per run) and read by nothing. The deep-research pass (roadmap v15,
`docs/NEXT_FEATURES.md`) found the external run-detail pattern fully settled — a
**master/detail step timeline** (per step: type, name, duration, tokens, cost; full
input/output in a detail pane) plus a **chat-style transcript view** of the same events —
and local-first precedents (Phoenix on localhost; claude-code-log/claude-replay rendering
JSONL transcripts as static HTML) prove the minimum viable form is *a renderer over an
append-only log*, which is exactly what we persist.

This epic builds that surface: the repo's own established move (ship the store, then the
deferred reader surface — B3→V11, A2→F1, ADR-0048→this), converging the two differentiators
at the finest grain yet (run → lane → **step/turn** cost).

**Out of scope (research-rejected):** true time-travel / fork-from-step (needs per-step
state checkpoints — a persistence project; ADR-0048 already decided replay = read-only
reconstruction); structural trace-tree diffing (the industry diffs runs *keyed by the same
task* on outputs/cost); AI-assistant-over-traces; full OTel SDK plumbing (the
`invoke_agent`/`execute_tool` *naming* is adopted as a guide only — the spec is still
Development status).

## Children

- [x] **O1 — Run-detail page: the step timeline** (shipped — also wired `EventLogDir`
      in production; the ADR-0048 writer had been switched off everywhere)
      ([0091](0091-run-detail-step-timeline.md), M, **ADR-0052**, BUILD FIRST) —
      `GET /page/runs/{id}` rendering the event log as a lane-grouped master/detail step
      timeline; zero new JS; read-only reconstruction.
- [x] **O2 — Price the timeline: per-step usage/credits** (shipped)
      ([0092](0092-per-step-cost-event-log.md), S/M) — additive `RunEvent` usage/credits
      fields stamped at time of use; per-turn pricing + per-lane subtotals + a per-run
      header cross-check against `RunRecord.Credits`.
- [x] **O3 — Transcript view** (shipped) ([0093](0093-run-transcript-view.md), S) —
      `?view=transcript` flattening the same events into chat reading order through the
      block-AST renderer; per-turn O2 pricing; a timeline ⇄ transcript toggle.
- [x] **O4 — Compare two runs** ([0094](0094-compare-two-runs.md), M, stretch) — keyed
      side-by-side deltas (outcome/duration/credits/per-lane) for two runs of one workflow
      (shipped: `CompareRuns→RunDelta` + `GET /runs/compare` + the compare-with picker).

## Acceptance (epic)

- [x] Each child lands test-first, born in its own PR; `make lint && make test` (floor 65%)
      + `make e2e` for UI slices green; escaping per ADR-0001 throughout.
- [x] The inspector contract (read-only reconstruction, step vocabulary) is recorded in
      ADR-0052 by the first child (ADR-0004 discipline).
- [x] The event-log on-disk contract stays additive-only (ADR-0048): pre-O2 logs load and
      render (usage fields read back zero); the tag-stability pin extends to the new fields.
- [x] No new runtime dependency, no new JS, no build chain; the live SSE/record path stays
      byte-identical when the inspector is never opened.

**Closed:** all four children shipped (O1 timeline, O2 per-step pricing, O3 transcript, O4
keyed run comparison). The inspector is a read-only reconstruction surface over the per-run
event log (ADR-0052/0048): a lane-grouped step **timeline** and a chat-order **transcript**
of one run, plus a keyed **side-by-side comparison** of two runs of one workflow. No new JS,
no runtime dependency; the live SSE/record path is untouched when the inspector is never opened.

## Sequencing

O1 → O2 → O3 → O4, re-ranked on each merge. O4 carries R6/stretch semantics — drop it if a
fresh look ranks it low; the epic closes on its last shipped child.

## Notes

Source: the v15 deep-research pass (2026-06-12) — five search angles + an internal seam map,
14 load-bearing claims verified 3×14 adversarial votes (all SUPPORT). Runner-ups documented
in `docs/NEXT_FEATURES.md` (durable autopilot; active cost governance; learnings forge
entity; worktree lane isolation) stay candidates for v16. Builds on ADR-0048 / issue 0085
(the log), epic 0076 (the block-AST renderer O3 reuses), and the Runs surface of epics
0031/0042.
