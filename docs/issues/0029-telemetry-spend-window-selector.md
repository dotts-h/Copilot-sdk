---
id: 0029
title: Telemetry spend-window selector (roadmap v4, item G3/V9)
status: open
severity: medium
group: 0024
github:
links:
  adr:
  prs: []
  issues: [0024, 0028]
  regression:
assets: []
---

## Summary

The Telemetry "Spend over time" trend hardcodes a **14-day window**
(`internal/web/pages.go` `spendTrend`: it slices `DailyTotals` to the most-recent 14
days, then scales each bar to the busiest day *in that window* — REGRESSIONS #14, guarded
by `TestTelemetryTrendWindowsAndScalesToVisibleMax`). A user with months of history can't
see past 14 days. G3 adds a **14/30/90-day window selector** on the Telemetry page: a small
three-button control (the active one marked) that re-renders the trend over the chosen
window. **No schema change, no new store** — a presentation-layer slice over the existing
pure `DailyTotals` reader. The one invariant that must survive (REGRESSIONS #14): slice to
the chosen window **first**, **then** compute `maxUSD` over what's shown — never compute
the max over full history before slicing, or an off-window peak makes every visible bar a
sliver. Source: `docs/NEXT_FEATURES.md` item G3/V9.

## Repro
1. Open the Telemetry page with a spend ledger holding more than 14 days of history.
   - **Expected:** a 14/30/90 window selector lets the user widen the "Spend over time"
     trend; the default (no param) is 14 days (the historical behavior); a wider window
     surfaces older days; the busiest day *in the chosen window* always fills the bar.
   - **Actual (before G3):** the trend is fixed at the most-recent 14 days with no way to
     widen it; months-old history is invisible on the page.

## Proposed resolution

- **`internal/web` (Telemetry view):** `spendTrend(window int)` takes the window (days)
  instead of the hardcoded 14; `handlePage` reads `?window=` (default 14, clamp to the
  allowed set {14, 30, 90}, garbage/out-of-range → 14 via `clampWindow`) and threads it
  through `renderPage` → `telemetryPartial` → `spendTrend`. The `maxUSD` scaling stays
  **after** the window slice (REGRESSIONS #14 invariant). The `telemetryPage` template
  gains a window selector (three buttons, active one marked) that re-fetches
  `GET /page/telemetry?window=N` into `#main`, mirroring the Models-page effort row. All
  values through `html/template` (ADR-0001).
- **No new store, no schema change** — the windowing is a web-layer slice over the existing
  pure `telemetry.DailyTotals` reader.
- **Tests:** web — the trend renders the default 14-day window with no param; `?window=30`
  and `?window=90` widen it (more/older rows given a >14-day seeded history); a
  garbage/out-of-range window falls back to 14 (`clampWindow`); the selector renders with
  the active window marked; AND the REGRESSIONS #14 invariant holds for the **new** windows
  (seed an off-window all-time peak and assert the busiest in-window day fills 100% per
  window). e2e — assert the window-selector **structure** (three buttons, the active one
  marked, switching re-renders the trend), never exact figures (the shared demo ledger is
  append-only across the suite).

## Notes
- **No ADR:** a presentation-layer change over an existing pure reader (like 0025/0026/
  0027/0028). Captured in CONTRACTS §3 (the Telemetry-page window selector: the `?window=`
  param, the allowed set + default/clamp, and that the `maxUSD` scaling stays window-local).
  A REGRESSIONS entry is added **only if** a real bug/gotcha is found-and-fixed.
- **Differentiator:** completes the cost surface — the spend trend becomes inspectable over
  the full history, not just the last 14 days. The **last child** of epic 0024 (roadmap
  v4); on its merge the epic closes.
- **Numbering:** issue **0029** (next free after 0028), fifth and final build of epic
  **0024** (roadmap v4). No ADR consumed.
</content>
