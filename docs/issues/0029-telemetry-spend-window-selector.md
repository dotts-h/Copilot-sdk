---
id: 0029
title: Telemetry spend-window selector (roadmap v4, item G3/V9)
status: closed
severity: medium
group: 0024
github:
links:
  adr:
  prs: [55]
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

## Resolution (shipped)

Built as specified, no schema change and no new store — a presentation-layer slice over the
existing pure `telemetry.DailyTotals` reader. `internal/web` (`pages.go`): `spendTrend(window
int)` takes the window (days) instead of the hardcoded 14, slicing `DailyTotals` to the
chosen window **first** and computing the bar-scaling `maxUSD` **after** the slice — the
REGRESSIONS #14 invariant, so an off-window all-time peak can't shrink the visible bars.
`telemetryPartial(window int)` threads the window through and builds the selector's
button data (`{Value, Active}` per window). `clampWindow(raw string) int` parses the
`?window=` value to one of `spendWindows` ({14, 30, 90}), falling back to
`defaultSpendWindow` (14) for an empty / unparseable / out-of-range / negative value.
`renderPage(slug, window string)` threads the raw value to `telemetryPartial` (clamped);
`handlePage` reads `r.URL.Query().Get("window")`; `cmdNav` passes `""` (→ default). The
`telemetryPage` template gained a `.window-row` control (three `.window` buttons, the active
one marked `.window.on`) that re-fetches `GET /page/telemetry?window=N` into `#main`,
mirroring the Models-page effort row; matching `.window`/`.window-row` CSS was added
(`static/app.css`, cloned from `.effort`). All values flow through `html/template`
(ADR-0001).

Tests: unit (`internal/web`) `TestClampWindow` (allowed set + empty/garbage/out-of-range/
negative/non-integer → 14), `TestSpendTrendWidensWithWindow` (14 < 30 < 90 surface
more/older rows; 90 clamped by available history; most-recent day stable across windows),
`TestTelemetryTrendScalesToVisibleMaxPerWindow` (the #14 invariant asserted for **each**
window — an off-window peak doesn't leak and the busiest in-window day fills 100%),
`TestTelemetryPageRendersWindowSelector` (three buttons, the active one marked, the others
not); the pre-existing `TestTelemetryTrendWindowsAndScalesToVisibleMax` (the #14 guard) is
retained. e2e: a selector-**structure** test (three buttons, the active one marked,
switching to 90d re-renders the trend — never figures, since the demo ledger is shared +
append-only). Gates green (`make lint && make test`, web coverage 88.8%); the e2e Chromium
browser is blocked by the env's network allowlist, so the spec was verified to
compile/discover via `npx playwright test --list` and CI runs the real Playwright suite.

Docs: CONTRACTS §3 (the Telemetry-page window selector — the `?window=` param, the allowed
set + default/clamp, the window-local `maxUSD`). No REGRESSIONS entry — no bug was
found-and-fixed; the clamp/default, the negative-window guard, and the maxUSD-after-slice
invariant were guarded preemptively (self-review with `/code-review` high effort confirmed
them, and surfaced one polish gap — the unstyled selector buttons — now fixed). No ADR.
Shipped on branch `claude/spend-window-selector` (**PR #55**). **The last child of epic
0024 — on this merge the epic closes.**

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
