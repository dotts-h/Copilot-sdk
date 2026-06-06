---
id: 0015
title: Real parallel workflow lanes (roadmap v2, item B1)
status: closed
severity: medium
group: 0013
github:
links:
  adr: ../adr/0017-per-lane-tool-and-permission-surface-for-parallel-workflow-lanes.md
  prs: []
  issues: [0013]
  regression: ../REGRESSIONS.md
assets: []
---

## Summary

Workflows (ADR-0013) ship the **sequential** path end-to-end, but the **parallel**
fan-out — the differentiated half of "orchestration" — is model/engine-only: the
`MockClient` returns a single session id, so the offline demo and e2e can only
drive one lane at a time, and a lane surfaces only its message + metered cost (no
per-lane tool timeline or inline permission). Make parallel runs observable and
fully exercised. Source: `docs/NEXT_FEATURES.md` item B1; promotes TECH_DEBT #12;
extends ADR-0013.

## Repro
1. Define a workflow with `mode: parallel` and two steps.
2. Run it in the offline demo.
   - **Expected:** two lanes stream concurrently in `#lanes`, each with its own
     tool cards and inline permission prompts.
   - **Actual:** `MockClient` hands back one session id, so `handleRunEvent`'s
     `SessionID`-keyed routing can't disambiguate concurrent lanes — the demo/e2e
     can only drive the sequential path; the parallel engine logic is unit-tested
     directly but never browser-driven, and a lane shows only message + usage.

## Resolution (shipped — ADR-0017)

- **Distinct lane identities:** `MockClient.CreateSession` now returns distinct
  ids (`mock-session-N`); `streamDemoLane(sid, …)` tags every event with the
  lane's id, so a seeded **parallel** demo workflow (`parallel-review`) drives
  concurrent lanes through the unchanged `SessionID`-keyed `laneFor`.
- **Per-lane tool + permission surface:** `handleRunEvent` reduces
  `EvToolStart/Progress/End` + `EvPermission` onto per-lane state; `renderLanes`
  surfaces each lane's own tool timeline and inline permission by reusing the chat
  `renderToolCard`/`renderPermForm` (all escaped, ADR-0001). `handlePerm` is
  lane-aware: a lane permission is answered over the shared `/perm/{id}` route,
  dropped from the lane, and `#lanes` refreshed out-of-band. Permissions are
  **per-lane**, not a cross-lane FIFO (the open question — recorded in ADR-0017).
- **Browser-drivable parallel demo:** the seeded parallel workflow + a new
  `e2e/tests/e2e.spec.ts` spec assert two concurrent lanes, per-lane tool cards,
  and inline permission forms (structure, not figures — shared demo session).
- **Invariants held:** `SessionID`-keyed attribution (REGRESSIONS "a workflow run
  owns the turn") and lane-text escaping (ADR-0001); no seam method added.

See ADR-0017 and the guard tests listed in TECH_DEBT #12 (Paid).

## Resolution (planned — superseded by the above)

- **Distinct lane identities (`internal/copilot/mock.go`):** give each
  `CreateSession` a distinct session id so `SessionID`-keyed lane routing
  (`workflow.go` `laneFor`) disambiguates concurrent lanes — the only safe key
  when lanes run at once (see the REGRESSIONS workflow-run gotcha).
- **Per-lane tool + permission surface (`internal/web/workflow.go`):** extend
  `handleRunEvent` to render a lane's tool timeline and inline permission cards
  (today it folds only message + usage), reusing the chat tool/permission
  rendering, all HTML-escaped (ADR-0001).
- **Browser-drivable parallel demo (`internal/web/demo.go`):** seed a parallel
  workflow and emit `SessionID`-tagged events for two lanes so the demo drives a
  real concurrent run; add an `e2e/` parallel spec (mind the shared-session and
  `pages.length` couplings).
- **Contract/architecture:** no seam change — `SessionID` is already on `Event`;
  this fulfils the parallel half ADR-0013 scoped. Note the now-exercised parallel
  path in ARCHITECTURE and clear TECH_DEBT #12.

## Notes

- **Decision:** extends ADR-0013 (multi-agent workflow run / handoff surface) —
  the parallel engine and `SessionID` routing already exist; this issue makes them
  observed, not just unit-tested. If per-lane permission queuing needs a new
  decision (FIFO across lanes vs per-lane), record a short follow-up ADR.
- **Guard tests to add:** `internal/copilot` that `CreateSession` ids are
  distinct; `internal/web` a `TestRun*`/reducer test driving two `SessionID`-tagged
  lanes concurrently with per-lane tool + permission fragments; an `e2e/` parallel
  workflow spec (assert structure, not figures — shared demo session).
- **Pairs with:** A2 (per-workflow/per-lane cost attribution) and B3 (run history)
  — a parallel run is the natural unit of orchestrated spend.
