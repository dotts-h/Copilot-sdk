# 0027. Telemetry KPI dashboard: pure readers + pure Go SVG builders, server-rendered inline charts

- Status: accepted
- Date: 2026-06-08
- Deciders: Horia
- Related: the **third child of the UI/UX refresh epic**
  ([0045](../issues/0045-epic-ui-ux-refresh.md), roadmap v9) — the telemetry-dashboard follow-on to
  the V21 token/theme foundation ([ADR-0025](0025-design-token-foundation-and-light-dark-theming.md))
  and the V22 navigation regroup ([ADR-0026](0026-grouped-sidebar-navigation-and-command-palette.md)),
  on both of which it builds (the cards + charts are styled with the V21 semantic tokens, never raw
  literals, and live on the Telemetry page the V22 sidebar groups under *Observe*). Keeps the **no
  build chain / single committed CSS file** doctrine (`internal/web/static/app.css`) and the
  **minimal-JS, no-framework** posture — the charts are **server-rendered inline `<svg>`** with **zero
  JS** and **no charting library**, re-rendering through the existing `?window=` htmx swap. Holds the
  **domain-logic-stays-pure** architecture rule: the period-over-period + sparkline series are a new
  pure `telemetry` reader, table-tested in isolation. Touches `internal/telemetry/dashboard.go` (the
  pure reader), `internal/web/svg.go` (the pure SVG builders), `internal/web/dashboard_render.go` +
  `internal/web/telemetry_render.go` (the view assembly), `internal/web/templates/fragments.html`
  (the `telemetryPage` card row + charts), `internal/web/static/app.css` (the `.kpi*`/`.spark`/
  `.band`/`.bullet` styles), `internal/bootstrap/bootstrap.go` (a prior-window seed so the offline
  deltas are real), the e2e suite (`telemetry.spec.ts`, the both-theme `a11y.spec.ts`),
  `docs/CONTEXT.md` (the **KPI card** / **sparkline** / **bullet graph** / **trend band** terms), and
  [issue 0048](../issues/0048-telemetry-kpi-dashboard.md). **No server route, no persisted schema, no
  `copilot.Client` change** — CONTRACTS unchanged; the new telemetry symbols are additive readers and
  the web SVG builders are unexported, so CODEMAP is unchanged but for the additive `telemetry`
  declarations.

## Context

A v9 research pass (recorded under epic 0045) reviewed the web UI against modern front-end practice.
After V21 cleared the theming/a11y foundation and V22 regrouped the navigation, the standout
remaining gap is the **telemetry surface itself**: the cost/usage page reads as **plain tables** — a
key/value summary, a per-model grid, and a stack of `ul.trend` bar lists — with **no KPI hierarchy,
no period-over-period deltas, and no sparklines**, where the same persisted-ledger data could read
like a credible BI report (Few / NN/g dashboard practice: a row of "big number" cards, each with a
delta and a sparkline; a cumulative trend with a forecast; a spend-vs-budget bullet).

This child adds that dashboard **without** a build step, a charting library, a JS framework, or a new
route. The constraints are not limiting: **server-rendered inline SVG** from pure Go builders covers
sparklines, an area/line trend band, and a bullet graph with zero JS, and the existing `?window=`
selector already re-renders the whole Telemetry partial server-side — so a window change re-renders
every chart with no client code.

The decisions an ADR must settle: **where the SVG is built**, **where the period-over-period +
sparkline series are computed**, **the KPI set + delta semantics (and the coloring rule)**, **the
chart inventory (what ships in V23)**, **the a11y of inline SVG**, and **confirming no new route /
schema**.

## Considered options

- **Where the SVG is built.**
  - **Pure Go SVG builders — a numeric series → an `<svg>` string, injected as trusted HTML
    (chosen).** A small set of pure functions (`sparkPoints`, `areaPath`, `bulletGeom`, and the
    `sparklineSVG`/`trendBandSVG`/`bulletSVG` assemblers in `internal/web/svg.go`) compute coordinates
    over a fixed viewBox and emit the markup as a string, exactly mirroring the project's
    string-building renderers (`render.go`, `palette.go`, `help.go`) injected via `trusted(...)`. The
    **coordinates are unit-tested directly** (`svg_test.go` asserts `sparkPoints([0,10],…) ==
    "2,26 98,2"`, the closed `areaPath`, the clamped `bulletGeom` fractions), so the charting is
    deterministic and regression-guarded without a browser. This keeps charting **zero-JS** and
    **htmx-swappable** (the SVG is part of the server-rendered partial).
  - *Inline `html/template` path math (compute `d`/`points` in the template).* Rejected — arithmetic
    in templates is unreadable and untestable, and `html/template`'s contextual escaping fights raw
    SVG path data. The Go builder is the same "compute in Go, inject trusted HTML" seam the rest of
    the renderers use.
  - *A charting library (Chart.js / a Go SVG lib / D3).* Rejected — a charting library is a JS
    dependency + a build/CDN step (the no-framework, no-build constraint), and a Go SVG dependency is
    weight for three simple shapes the builder covers in ~120 lines. The hard constraint is **no
    charting library**.

- **Where the period-over-period + sparkline series are computed.**
  - **A new pure `telemetry` reader — `Dashboard(records, now, window)` + `ChangePct` (chosen).**
    Domain logic stays pure (the architecture rule): `telemetry.Dashboard` rolls the ledger into the
    **current** window's spend, the immediately-preceding **equal-length** window's spend (the delta
    baseline), and the current window's **zero-filled daily series** (the sparklines' source);
    `WindowSpend` carries the derived `AvgCostPerTurn`/`DailyRate`; `ChangePct(prior, current)` is the
    period-over-period `Delta`. All of it is **table-tested** in `dashboard_test.go` with a fixed
    `now`, so the numbers behind every card are pinned independently of the web layer. The web layer
    only joins these figures to the SVG builders.
  - *Ad-hoc math in the web layer.* Rejected — it buries domain logic (window splitting, the "new vs
    %" delta rule, the zero-fill) in render code where it can't be unit-tested in isolation, against
    the `telemetry` package's dependency-free, fully-unit-tested contract.

- **The KPI set + delta semantics.**
  - **Four cards — Total spend, Turns, Avg cost/turn, Burn rate (cr/day) — each a Δ% vs the prior
    equal-length window + a sparkline, colored by a per-metric *higher-is-worse* flag (chosen).**
    Each card shows a "big number" for the current window, a Δ badge (▲/▼/→ with the signed percent),
    and a sparkline over that metric's daily series. The **delta coloring is not a blanket green-▲**:
    a per-metric `higherIsWorse` flag decides favorability, so **spend ▲ is `--warn`, not `--good`**
    (a rise in cost is bad), while **Turns ▲ is `--good`** (more activity is not waste); avg
    cost/turn and burn rate are higher-is-worse like spend. The badge carries a stable **direction**
    class (`up`/`down`/`flat`) for the glyph and a separate **tone** class (`good`/`warn`/`neutral`)
    for the color, both off `--good`/`--warn` (`--dim` for neutral). A metric with **no prior
    baseline** (the prior window had zero spend) reads **"new"** in a neutral tone rather than an
    infinite percentage; an unchanged metric reads `0%` neutral.
  - *One delta rule for all (every ▲ green / every ▼ red), or no deltas.* Rejected — a blanket-green
    ▲ actively misleads on a cost dashboard (it praises a spend increase); dropping deltas loses the
    period-over-period read that is the dashboard's point. The per-metric flag is one boolean per card.

- **The chart inventory (what ships in V23).**
  - **All three ship: the per-card sparkline (polyline), the trend band (cumulative-spend area, solid,
    + a dashed burn-rate forecast continuation), and the spend-vs-budget bullet (a budget track, a
    month-to-date measure bar, and a target marker at the projected month-end spend) (chosen).** The
    three are small pure builders over data the page already computes (the daily series, the
    `Projection`/`DailyRate`, the `Budget` + month-to-date), so shipping all three in V23 is bounded;
    deferring any would leave the dashboard half-told. The bullet renders only when a **monthly
    budget** is configured (else there is no track to measure against).
  - *Defer the band and/or the bullet to V24.* Rejected — they reuse the same builder + token
    machinery as the sparkline and the same window re-render, so there is no integration cost to
    amortize by splitting; V24 is reserved for **motion/polish** (View Transitions + a component pass),
    not more charts.

- **A11y of inline SVG (both-theme axe).**
  - **Each chart is `role="img"` with a `<title>` **and** an `aria-label` summarizing the trend; the
    glyph in the delta badge is `aria-hidden` (chosen).** axe's `svg-img-alt` rule requires an
    accessible name on a `role="img"` SVG; a `<title>` child plus a matching `aria-label` satisfies it
    robustly. The charts carry **no color literal** — strokes/fills resolve from `currentColor` or a
    token (`var(--accent)`/`var(--accent2)`/`var(--fg)`/`var(--bad)`/`var(--sunken)`) — so they theme
    with the page and clear contrast in **both** themes, and the existing tables remain below as the
    full text equivalent of the same data.
  - *`aria-hidden` SVGs leaning entirely on the adjacent tables.* Rejected — the cards' sparklines
    have no adjacent per-card table, so a summarizing accessible name is the honest equivalent; the
    `role="img"` + name is a one-attribute cost that makes each chart self-describing.

- **No new route / schema (confirm).**
  - **None — the existing `?window=` htmx selector re-renders the whole `telemetryPartial`
    server-side, so the dashboard re-renders per window with no new endpoint, no JS, and no persisted
    state (chosen + confirmed).** V23 adds **no server route, no schema change, no `copilot.Client`
    change** — it reads the same ledger the rest of the page reads.

## Decision

Add a pure `telemetry.Dashboard(records, now, window)` reader returning the current window's
`WindowSpend`, the prior equal-length window's `WindowSpend`, and the current window's zero-filled
`[]DayPoint` series, plus `WindowSpend.AvgCostPerTurn`/`DailyRate` and a `ChangePct(prior,current)
Delta` (with a `HasPrior` "new" flag), all table-tested. Add pure Go SVG builders in
`internal/web/svg.go` — `sparkPoints`/`areaPath`/`bulletGeom` (coordinate-tested) and the
`sparklineSVG`/`trendBandSVG`/`bulletSVG` assemblers — each emitting a fixed-viewBox `role="img"`
`<svg>` with a `<title>`+`aria-label` and **token/`currentColor`** strokes (no literals).
`(*Server).dashboardView(window, now)` joins them into the KPI-card view data: four cards (Total
spend, Turns, Avg cost/turn, Burn rate), each with a value, a `deltaView` badge (direction + tone via
a per-metric higher-is-worse flag), and a sparkline; the cumulative **trend band** (solid actuals area
+ dashed forecast at the window's daily rate over the days left this month); and the spend-vs-budget
**bullet** (when a budget is set). `telemetryPartial` computes it and the `telemetryPage` template
renders the card row + charts **above** the existing tables (which stay as the accessible data). The
offline demo seeds two prior-window records so the deltas are real offline. No build step, no charting
library, no JS, no server route, no schema change.

## Consequences

- Positive: the Telemetry page reads as a **BI-style dashboard** — a row of big-number cards with
  honest, per-metric-colored deltas and sparklines, a cumulative trend with a burn-rate forecast, and
  a spend-vs-budget bullet — all **server-rendered inline SVG, zero JS, no charting library, no build
  step**. The charts re-render through the existing `?window=` swap, so the window selector drives the
  whole dashboard with no new client code.
- Domain purity held: the window split, the derived metrics, the zero-filled series, and the delta
  rule live in `telemetry` (dependency-free, table-tested); the coordinate math lives in pure,
  unit-tested SVG builders. The web layer only assembles. New UI uses the V21 tokens — the charts
  carry no hex/`rgba`.
- A11y (both themes): each chart is `role="img"` with a `<title>`+`aria-label`; strokes/fills are
  tokens/`currentColor`, so the both-theme axe scan (now covering the SVG dashboard) passes and the
  existing tables remain the text equivalent. The `ux.spec` no-overflow guard holds (the cards wrap).
- One-fact-one-home: the delta semantics (the "spend ▲ is warn", "new vs %") live once in `ChangePct`
  + `deltaView`; the KPI set + higher-is-worse flags live once in `dashboardView`; the coordinate
  conventions live once in the SVG builders.
- Scope held: this is the **telemetry-dashboard** child only. The motion/polish pass (V24 — htmx View
  Transitions + a component pass over cards/buttons/tables/meters/the new KPI/SVG surfaces) is a
  **separate child** of epic 0045 with its own ADR. The deferred **Open Props** primitives + CSS
  **`@layer`** (recorded in ADR-0025) remain deferred-additive; this child does not adopt them.
- Contract surface: **none.** No new route, no `copilot.Client` change, no persisted schema. CONTRACTS
  is unchanged; CODEMAP gains only the additive `telemetry` reader declarations (the web SVG builders
  and `dashboardView` are unexported).
