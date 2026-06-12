---
id: 0094
title: "Compare two runs of one workflow — keyed side-by-side deltas (O4, stretch)"
status: closed
severity: low
group: 0090
depends_on: [0091]
github:
links:
  adr:
  prs: []
  issues: [0090]
  regression:
assets: []
---

## Summary

Pick two runs of the same workflow (e.g. a run and its rerun) and render a side-by-side
comparison: outcome, duration, total credits, per-lane credits/status/pauses deltas, and —
when both logs exist — the final assistant outputs. **Keyed** comparison (same task key:
the workflow), with output + cost deltas — the Braintrust experiment-comparison pattern.
Explicitly **not** a structural trace-tree diff (research-rejected: the industry diffs
keyed runs, not span trees). A pure `RunDelta(a, b RunRecord)`-style reader in
`internal/telemetry` (the `*Shares`/`RunAggregates` cousin) + presentation in the web layer.

## Why now

Completes the inspector's loop with V18's rerun: rerun → compare is the local equivalent of
an experiment re-run, answering "did the new definition do better, and what did it cost?"
Stretch child (R6 semantics) — drop if a fresh look ranks it low.

## Touches

- `internal/telemetry` — a small pure reader (`RunDelta` or similar; returns ids/values,
  no labels).
- `internal/web` — `runs.go` (run-picker control on the detail page or Runs page, compare
  partial), `templates/fragments.html`.

## Acceptance

- [x] Two runs of one workflow render side-by-side with per-field deltas (credits,
      duration, per-lane); runs of different workflows refuse the comparison cleanly.
- [x] Final outputs render only when both event logs exist; missing logs degrade to the
      summary-only comparison.
- [x] The reader is pure + table-tested (deterministic order, lane alignment by index);
      `make lint && make test` (floor 65%) green.

## Notes

M-sized; no ADR expected (pure cross-record reader + presentation, the V15/V16 pattern).
An ADR only if a genuinely new seam appears (it shouldn't).

**Closed:** shipped as the pure `telemetry.CompareRuns(a, b RunRecord) RunDelta` — keyed on
`WorkflowID` (a cross-workflow pair returns `Comparable=false`, refused not diffed), with
B−A run-level deltas (outcome/credits/duration/pauses) and lanes **aligned by index** over the
union of indices (a one-sided lane carries `Present=false`, the gap rendered, not a bogus
zero). Presentation is `GET /runs/compare?a=&b=` (`handleRunCompare` → `runComparePartial`),
reached from a **"compare with"** picker on the run-detail page that lists the workflow's other
runs (`compareCandidates`, cross-workflow runs excluded). Each run's **final assistant output**
(`finalAssistantOutput`, last `EvMessage`) renders only when **both** event logs exist, else the
view degrades to summary-only. Demo seeds a `build-and-harden` rerun (`run-demo-3`) with its own
log so the offline inspector showcases the keyed diff + both outputs. No ADR (pure cross-record
reader + presentation, the V15/V16 pattern). Table tests cover the keying, B−A deltas, by-index
alignment, mismatched lane counts, and determinism; web tests cover the side-by-side render, the
cross-workflow refusal, the both-logs-present output gate, escaping, the mux, and the picker; an
e2e asserts the picker → comparison + both final outputs. `make lint && make test` (telemetry
96.9%, web 89.9%) + `make e2e` green.
