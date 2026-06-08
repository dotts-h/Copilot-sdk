---
id: 0045
title: "Epic: UI/UX refresh — design-token foundation, theming, navigation IA, and a real telemetry dashboard (roadmap v9)"
status: open
severity: medium
group:
github:
links:
  adr: [0025]
  prs: [44]
  issues: [0046]
  regression:
---

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
- [ ] **V22 — navigation → grouped sidebar + ⌘K command palette** (M/L; the highest-impact IA
      change; **own ADR**). Top bar → a left sidebar grouping the 13 items into *Primary*
      (Chat, Sessions) · *Build* (Agents, Workflows, Skills, Instructions, Snippets) · *Observe*
      (Runs, Telemetry) · *Config* (Models, MCP, Settings) · *Help*, with config/help deferred
      to the bottom (progressive disclosure). A ⌘/Ctrl-K command palette (input + filtered list
      + one global keydown — minimal vanilla JS, reusing the existing keymap dispatch) so
      grouping never blocks a power user. *Teed up; not started.*
- [ ] **V23 — telemetry dashboard: KPI cards + server-rendered SVG sparklines** (L; **own ADR**).
      A top row of "big number" KPI cards each with a period-over-period delta (▲/▼ %) and an
      inline sparkline; a trend band (cumulative spend area; burn-rate actuals solid / forecast
      dashed); a spend-vs-budget bullet. Charts are **server-rendered inline `<svg>` from Go
      templates** (zero JS, htmx-swappable) — no charting library. *Teed up; not started.*
- [ ] **V24 — motion & polish: View-Transition swaps + component pass** (S/M; enhancement).
      Opt into htmx's same-document View Transitions (`htmx.config.globalViewTransitions`) for
      page swaps (degrades to instant where unsupported), and a component polish pass (cards,
      buttons, tables, meters) on the new tokens. Candidate to fold the deferred **Open Props**
      primitives + CSS **`@layer`** structure here. *Teed up; not started.*

## Status

**Open — building the first child (V21).** The functional surface (v4–v8) is mature; v9 is a
**presentation** epic. V21 lays the token + theme + a11y foundation every later child depends on
(a light theme, the contrast fix, the design-token vocabulary). V22 (navigation IA) is the
highest-impact follow-on, then V23 (the telemetry dashboard). Per repo convention each child is
born in its PR and the epic is re-ranked from a fresh value×fit pass on each merge.

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
takes **0045**; its first child **V21** takes issue **0046**. **ADR-0025 consumed** (the token +
theming foundation; highest ADR becomes 0025).
