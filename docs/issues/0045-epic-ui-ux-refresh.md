---
id: 0045
title: "Epic: UI/UX refresh — design-token foundation, theming, navigation IA, and a real telemetry dashboard (roadmap v9)"
status: closed
severity: medium
group:
github:
links:
  adr: [0025, 0026, 0027, 0028]
  prs: [44, 79, 81]
  issues: [0046, 0047, 0048, 0049]
  regression: [20]
---

> **Closed 2026-06-09 — all children shipped.** V21 token/theming foundation
> ([0046](0046-design-token-foundation-light-dark-theme.md), ADR-0025), V22 sidebar + ⌘K palette
> ([0047](0047-grouped-sidebar-command-palette.md), ADR-0026), V23 telemetry dashboard
> ([0048](0048-telemetry-kpi-dashboard.md), ADR-0027), and V24 motion & polish
> ([0049](0049-motion-and-polish.md), ADR-0028) all landed, every hard constraint held (no build
> chain, single committed CSS file, htmx + server templates, minimal JS). The "next value×fit
> pass" the epic deferred to became **roadmap v10** (epics 0050–0052), so this closes as exhausted.

## Charter

Roadmap **v4–v8** drove the two differentiators — cost-awareness and orchestration — to depth:
reconciliation, run history, aggregates, and (v8) an interactive orchestration surface. With the
*functional* surface mature, a dedicated **UX/front-end research pass** (deep-research, recorded
in this epic's first PR) re-read the web UI against modern front-end practice and found the
standout gap is no longer a missing reader or action — it is **presentation**:

1. **No theme system.** The stylesheet is dark-only, with raw color literals and
   `rgba(255,255,255,…)` overlays hard-coded through the rules — there is no light theme, and
   those literals actively block one. A **known WCAG AA contrast shortfall** on the destructive
   controls is carried as an allowlist in the a11y suite.
2. **Navigation overload.** ~13 flat top-bar items (Chat · Sessions · Telemetry · Skills ·
   Instructions · Agents · Workflows · Runs · MCP · Snippets · Models · Settings · Help) — past
   the count where a top bar scans well (NN/g: that's left-sidebar territory).
3. **A flat telemetry surface.** The cost/usage page reads as plain tables — no KPI hierarchy,
   no sparklines, no period-over-period deltas — where the data could read like a credible BI
   report (Few / NN/g dashboard practice).

This epic refreshes the **presentation layer** while holding the project's hard constraints:
**no build chain, a single committed CSS file, htmx + server-rendered Go templates, and a
minimal-JS / no-framework posture.** The research confirmed these are not limiting: modern
**vanilla CSS** (`light-dark()`, `color-scheme`, custom-property tokens, container queries,
`:has()`, View Transitions) and **server-rendered inline SVG** cover theming, polish, and
charting with no framework and no build step. Tailwind is viable only via its no-Node standalone
binary and is a poor fit for hand-written templates; **Open Props** and CSS **`@layer`** are
kept as *deferred, additive* options (they need no markup change).

## Tasks

- [ ] **V21 — design-token foundation + light/dark theme** (M; the foundation every later child
      builds on) → [0046](0046-design-token-foundation-light-dark-theme.md) (ADR-0025). A
      semantic design-token layer expressing **both** palettes via `light-dark()` keyed on
      `color-scheme`; an OS-default, persisted, no-FOUC theme toggle (a synchronous `<head>`
      script + a topbar button, client-only via `localStorage`); a palette retune that clears
      the destructive-control contrast baseline so the `KNOWN_CONTRAST_SELECTORS` allowlist is
      **deleted** and the a11y scan runs over **both** themes; a global `:focus-visible` ring +
      `prefers-reduced-motion` reset. **No build step, no framework, no server route.** **First
      child** — the epic is born in its PR.
- [x] **V22 — navigation → grouped sidebar + ⌘K command palette** (M/L; the highest-impact IA
      change; **own ADR**) → [0047](0047-grouped-sidebar-command-palette.md) (ADR-0026, PR #79).
      Top bar → a left sidebar grouping the 13 items into *Primary* (Chat, Sessions) · *Build*
      (Agents, Workflows, Skills, Instructions, Snippets) · *Observe* (Runs, Telemetry) · *Config*
      (Models, MCP, Settings) · *Help*, with config/help deferred to the bottom (progressive
      disclosure). A ⌘/Ctrl-K command palette (input + filtered list + one global keydown — minimal
      vanilla JS, reusing the existing keymap dispatch) so grouping never blocks a power user.
      **Shipped.**
- [x] **V23 — telemetry dashboard: KPI cards + server-rendered SVG sparklines** (L; **own ADR**) →
      [0048](0048-telemetry-kpi-dashboard.md) (ADR-0027, PR #81). A top row of "big number" KPI cards
      each with a period-over-period delta (▲/▼ %) and an inline sparkline; a trend band (cumulative
      spend area; burn-rate forecast dashed); a spend-vs-budget bullet. Charts are **server-rendered
      inline `<svg>` from pure Go builders** (zero JS, htmx-swappable via the existing `?window=`
      selector) — no charting library, no new route, no schema change. **Shipped.**
- [x] **V24 — motion & polish: View-Transition swaps + component pass** (S/M; enhancement) →
      [0049](0049-motion-and-polish.md) (ADR-0028). Opts the sidebar nav links into the browser View
      Transitions API with per-swap `transition:true` (one `{{range .Nav}}` loop; the ⌘K palette
      inherits it), scoped to `#main` with a `view-transition-name` so navigation cross-fades (instant
      where unsupported); an explicit `::view-transition-*` guard silences it under
      `prefers-reduced-motion`. **`globalViewTransitions` was tried and rejected** — it wraps the
      `hx-swap-oob` streaming updates and dropped run/turn completion swaps (REGRESSIONS); per-nav
      opt-in touches no streaming swap. A settle-aware `navTo` (waits for `htmx:afterSettle`) keeps the
      now-async nav deterministic. A token-driven component pass (new `--speed`/`--ease`/`--shadow`/
      `--shadow-lg` tokens; eased interactive controls; a 1px button press; resting card elevation)
      that changes no color pairing, so the both-theme axe scan is unaffected. The deferred **Open
      Props** primitives + CSS **`@layer`** stay deferred-additive (a conscious trade-off recorded in
      ADR-0028). **No build step, no framework, no new JS, no server route, no schema change.** **Last
      child — its merge closes the epic. Shipped.**

## Status

**All four children shipped — epic exhausted, closes on V24's merge.** The functional surface (v4–v8)
is mature; v9 was a **presentation** epic. V21 laid the token + theme + a11y foundation (PR #44); V22
regrouped the nav into a sidebar + ⌘K palette (PR #79); V23 turned the Telemetry page into a KPI/SVG
dashboard (PR #81); V24 added motion & polish — View-Transition page swaps (scoped to `#main`, with a
streaming opt-out and a reduced-motion guard) + a token-driven component pass (issue 0049, ADR-0028,
on branch `claude/next-features-research-8aBvS-Hq8Tb`). The presentation layer is now refreshed —
themed, regrouped, dashboarded, and in motion — while every hard constraint held (no build chain,
single committed CSS file, htmx + server templates, minimal JS / no framework). Per repo convention
the epic is re-ranked from a fresh value×fit pass on each merge; with all children shipped, **next is
a roadmap v10 value×fit pass** against the two differentiators (cost-awareness ⋈ orchestration).

## Notes

Per CONVENTIONS: write the failing test first; the UI gate is `make e2e` (Playwright axe over
**both** themes). Hold the hard constraints — **no build chain, single committed CSS file, htmx +
server templates, minimal JS / no framework**; flag any child that would relax one (a new build
step or JS dependency) as a conscious trade-off in its ADR. V21 takes **ADR-0025** (the token +
theming foundation); V22 and V23 are structural and each take their own ADR. Keep new e2e marker
classes **disjoint** (the V16–V19 lesson). Fold ADR/CONTEXT updates into the feature branch
(ADR-0004).

## Numbering

Highest on disk before this pass: issues → **0044**, epic → **0042**, ADRs → **0024**. This epic
takes **0045**; V21 → issue **0046** / ADR-0025, V22 → issue **0047** / ADR-0026, V23 → issue
**0048** / ADR-0027. Highest issue is now **0048**, highest ADR **0027**.
