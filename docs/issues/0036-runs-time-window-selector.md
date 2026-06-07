---
id: 0036
title: "Runs time-window selector (roadmap v6, item V12)"
status: open
severity: low
group: 0031
github:
links:
  adr:
  prs:
  issues: [0031]
  regression:
assets: []
---

## Summary

The Telemetry "Spend over time" trend carries a **14/30/90-day window selector** (G3,
issue 0029): a clamped `?window=` threaded `handlePage → renderPage → telemetryPartial →
spendTrend` slices the trend so a long history stays scannable. Its orchestration sibling,
the **Runs** page, has **no such control** — `runsPartial` always renders the entire run
history (and rolls the whole thing up in the per-workflow summary). A user with months of
runs can't slice the view.

**V12 closes that parity gap**: mirror the Telemetry trend's window selector on the Runs
page, **reusing** the shared `clampWindow` (not re-implementing it), so the run history can
be sliced to a 14/30/90-day window. Third child of epic
[0031](0031-epic-orchestration-accountability.md) (roadmap v6 — orchestration
accountability / Runs-surface parity). Source: `docs/NEXT_FEATURES.md` "roadmap v6"
section.

## Repro
1. Open the Runs page with a long run history.
2. There is no window control — the entire history renders, and the per-workflow summary
   rolls up every run ever recorded.
   - **Expected:** a 14/30/90-day selector (like the Telemetry trend's) slices the history
     to the recent window, in both the history list and the per-workflow summary.
   - **Actual (before V12):** no selector; the full history is always shown.

## Proposed resolution

- **`internal/web` (`runs.go`):** `runsPartial(window int)` takes the (clamped) window; a
  pure `windowRuns(records, window)` slices the history to the records started within
  `window` days of the **most recent run** (tail-relative like `spendTrend`, so a
  long-idle history still shows its latest window) **before** both `RunAggregates` and the
  history list — an out-of-window run drops from both. Build the `Windows` template data
  (the active window marked) like `telemetryPartial`.
- **`internal/web` (`pages.go`):** `renderPage`'s `"runs"` case calls
  `s.runsPartial(clampWindow(window))` — the same `clampWindow` the Telemetry page reuses
  (garbage / out-of-range → default 14).
- **`internal/web/templates/fragments.html` (`runsPage`):** add the `window-row` selector
  (three `button.window` controls re-fetching `GET /page/runs?window=N` into `#main`,
  active one marked) mirroring the Telemetry trend selector markup, rendered only when
  history exists.
- **No schema change, no new store, no new telemetry reader, no new ADR** — a pure
  presentation-layer slice over the existing v1 run records, reusing `clampWindow` /
  `spendWindows`.

## Notes
- **No ADR:** a pure presentation-layer slice reusing the existing `clampWindow` /
  `spendWindows`; no persisted-contract change and no cross-package seam. Captured as a
  pure-reader/UI composition in CONTRACTS §3, like the prior Runs-surface additions and
  the Telemetry window selector (0029).
- **Differentiator:** advances the **accountable** half of the orchestration story — the
  Runs surface gains the same windowing the cost surface already has. Third child of epic
  0031.
- **Numbering:** issue **0036** (next free after 0035), third build of epic **0031**. No
  ADR consumed (highest ADR stays 0022).
