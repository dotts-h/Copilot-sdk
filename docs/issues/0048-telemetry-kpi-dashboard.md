---
id: 0048
title: "Telemetry dashboard — KPI cards + server-rendered inline-SVG sparklines (roadmap v9, item V23)"
status: open
severity: medium
group: 0045
github: 80
links:
  adr: [0027]
  prs:
  issues: [0045]
  regression: [20]
---

## Summary

The Telemetry page is a **flat stack of tables** — a key/value cost summary, a per-model grid, and a
column of `ul.trend` bar lists (spend over time, per-model/agent/workflow share, ledger-vs-runs
reconciliation). There is **no KPI hierarchy, no period-over-period deltas, and no sparklines**, where
the same persisted-ledger data could read like a credible BI report (Few / NN/g dashboard practice).
This is the **telemetry-dashboard** gap epic [0045](0045-epic-ui-ux-refresh.md) (roadmap v9 — UI/UX
refresh) named as the third child, after the V21 token/theme foundation and the V22 navigation
regroup.

**V23 adds a dashboard above the existing tables:** a top row of **KPI "big number" cards** (total
spend, turns, avg cost/turn, burn rate cr/day), each with a **period-over-period Δ%** badge and a
**sparkline**; a **trend band** (cumulative-spend area + a dashed burn-rate forecast); and a
**spend-vs-budget bullet**. Charts are **server-rendered inline `<svg>` from pure Go builders** — zero
JS, no charting library — and re-render through the existing `?window=` htmx swap. It takes
**ADR-0027** for the decisions (where the SVG is built, where the deltas/series are computed, the KPI
set + delta-coloring rule, the chart inventory, SVG a11y, and confirming no new route/schema). Built
on V21's tokens — **no build step, no framework, no charting lib, no new server route, no schema
change**.

## Repro
1. Open the Telemetry page — it is a stack of tables with no KPI hierarchy, no deltas, no sparklines.
   - **Expected:** a row of big-number KPI cards (each with a ▲/▼ Δ% vs the prior equal-length window
     and a sparkline), a cumulative trend band with a burn-rate forecast, and a spend-vs-budget bullet
     — above the existing tables (which stay as the accessible data).
   - **Actual (before V23):** plain tables only.

## Proposed resolution

- **`internal/telemetry/dashboard.go` (new pure reader):** `Dashboard(records, now, window)` →
  current-window `WindowSpend`, prior equal-length `WindowSpend`, and the current window's zero-filled
  `[]DayPoint` series; `WindowSpend.AvgCostPerTurn`/`DailyRate`; `ChangePct(prior,current) Delta` (with
  a `HasPrior` "new" flag). Table-tested with a fixed `now` (domain logic stays pure).
- **`internal/web/svg.go` (new pure builders):** `sparkPoints`/`areaPath`/`bulletGeom`
  (coordinate-tested) + `sparklineSVG`/`trendBandSVG`/`bulletSVG`, each emitting a fixed-viewBox
  `role="img"` `<svg>` with a `<title>`+`aria-label` and token/`currentColor` strokes (no literals).
- **`internal/web/dashboard_render.go` + `telemetry_render.go`:** `dashboardView(window, now)` joins
  the reader + builders into the KPI-card view data (4 cards with a value, a `deltaView` badge —
  direction + tone via a per-metric higher-is-worse flag — and a sparkline) plus the band + bullet;
  `telemetryPartial` computes it.
- **`internal/web/templates/fragments.html`:** the `telemetryPage` renders the `.kpi` card row + the
  `.kpi-charts` (band + bullet) **above** the existing tables.
- **`internal/web/static/app.css`:** the single committed file gains the `.kpi*` / `.spark` / `.band`
  / `.bullet` styles (tokens only — no raw hex/rgba); the cards wrap (no horizontal overflow).
- **`internal/bootstrap/bootstrap.go`:** seed two prior-window records so the offline deltas are real.
- **ADR-0027** (written first, ADR-0004): pure Go SVG builders over template path math / a charting
  lib; pure telemetry readers over ad-hoc web math; the 4-card set + per-metric higher-is-worse delta
  coloring (spend ▲ = warn, turns ▲ = good); ship all three charts; `role="img"` + `<title>`/aria-label
  SVG a11y; **no new route / schema**. CONTEXT gains **KPI card**, **sparkline**, **trend band**,
  **bullet graph**.

## Tests (failing-first)

- **Go unit (`internal/telemetry/dashboard_test.go`):** the current/prior window split (off-window
  records excluded), the zero-filled ascending series, the derived metrics, and `ChangePct` semantics
  (rose/fell/flat/new-from-zero/both-zero).
- **Go unit (`internal/web/svg_test.go`):** exact `sparkPoints`/`areaPath` coordinates, clamped
  `bulletGeom` fractions, each builder's `role="img"`+`<title>`+aria-label, the dashed forecast, the
  bullet track/bar/target, escaped accessible names, no color literal in the sparkline.
- **Go unit (`internal/web/dashboard_render_test.go`):** `dashboardView` builds 4 labelled cards with
  the right delta direction/tone (spend ▲ → up/warn, turns ▲ → up/good), the band + bullet; nil
  without a ledger; the page renders the `.kpi`/`.spark`/`.band`/`.bullet` markers.
- **e2e `telemetry.spec.ts` (new):** the KPI cards render with a value + a Δ badge (a real up/down
  direction class), each card's sparkline svg has an accessible name, the band + bullet render, and
  re-selecting a `?window=` re-renders them server-side (no JS).
- **e2e `a11y.spec.ts`:** the both-theme axe scan covers the SVG dashboard (Telemetry page) and now
  waits for the chat elicit form so its contrast is guarded (REGRESSIONS #20); `ux.spec.ts` (no
  overflow) and `theme.spec.ts` stay green.

## Notes
- **ADR-0027:** the telemetry-dashboard child. Decisions: pure Go SVG builders (coordinate-tested,
  injected as trusted HTML) over template path math / a charting library; pure `telemetry` readers
  (table-tested) over ad-hoc web math; a 4-card set with a **per-metric higher-is-worse** delta
  coloring rule (spend/avg/burn ▲ = warn, turns ▲ = good — not a blanket green ▲, and "new" when no
  prior baseline); ship all three charts (sparkline, trend band, bullet); `role="img"` +
  `<title>`/aria-label + token/`currentColor` strokes for both-theme a11y; **no new route / schema**.
- **Differentiator:** **cost-awareness** — it makes the cost/usage data read as a dashboard (KPI
  hierarchy, deltas, sparklines, a forecast band, a budget bullet).
- **Scope held:** the motion/polish pass (V24 — htmx View Transitions + a component pass over
  cards/buttons/tables/meters/the new KPI surfaces) is a **separate child**, its own ADR (ADR-0028).
  The deferred **Open Props** + CSS **`@layer`** (ADR-0025) remain deferred-additive.
- **Numbering:** issue **0048** (next free after 0047), **ADR-0027** (next after 0026); epic stays
  **0045**.
