---
id: 0065
title: "Motion & micro-interaction system — linear() spring easings, motion tokens, the CSS-only interaction catalogue (roadmap v11, W3)"
status: open
severity: medium
group: 0062
depends_on: [0063]
github:
links:
  adr: []          # reserves ADR-0037 (motion system: linear() springs + the view-transition policy)
  prs: []
  issues: [0062]
  regression: [20]
assets: []
---

## Summary
The "delight" layer: a cohesive, **CSS-only spring motion system**. Ship motion tokens
(`--ease-spring` via `linear()`, `--ease-out-quint` `cubic-bezier(0.23,1,0.32,1)`, `--ease-overshoot`
`cubic-bezier(0.34,1.56,0.64,1)`; duration tokens micro ~150–200ms / transform ~300–400ms / spring
~600–1200ms) and a **micro-interaction catalogue** built from them, all degrading under
`prefers-reduced-motion` and **none breaking htmx OOB / SSE streaming**.

## Scope / Touches
- `internal/web/static/app.css` — motion + duration tokens (or adopt Open Props easings from 0063);
  `linear()` springs gated behind `@supports (animation-timing-function: linear(0,1))` with a
  cubic-bezier fallback; the catalogue: hover-lift, press `scale(.97)`, `:focus-visible` ring,
  **skeleton shimmer**, **list enter/exit + toast + ⌘K palette open** via `@starting-style` +
  `transition-behavior: allow-discrete` + `popover`/`dialog`, optional **scroll-driven reveals**
  (`animation-timeline: view()`, **gated with `@supports` — no Firefox**); extend the
  `prefers-reduced-motion` reset to cover the new animations + `::view-transition-*` pseudo-elements.
- `internal/web/templates/*.html` + tiny vanilla JS only where a state hook is unavoidable (prefer
  pure CSS / `popover`).
- **View-transition policy (ADR-0037):** keep **per-nav opt-in** (`transition:true`), **never**
  `htmx.config.globalViewTransitions` — the platform runs one transition per document at a time, so
  global wrapping aborts transitions against each other and drops `hx-swap-oob` / streamed SSE swaps
  (the exact dead-end in [ADR-0028](../adr/0028-motion-and-polish-htmx-per-navigation-view-transitions.md)
  / REGRESSIONS, re-confirmed by this research). May add **scoped** element transitions for in-place
  swaps, gated.

## Dependencies
- `depends_on: [0063]` — uses the token/`@layer` foundation (and Open Props easings if adopted).
  Independent of the surface restyle (0064) at the seam, but 0066 applies both together.

## Acceptance
- A documented motion-token set + the micro-interaction catalogue land; springs degrade to
  cubic-bezier where `linear()` is unsupported and **everything** collapses under
  `prefers-reduced-motion`; no streaming/OOB swap is wrapped in a competing transition (guarded by the
  existing multi-agent/stream e2e specs staying green).
- Failing/guard test where unit-testable; `make lint && make test` + `make e2e` green; born in its PR;
  ADR-0037.

## Notes
Sources: Josh Comeau + Carmen Ansio + web.dev (`linear()` springs, spring→duration tables), Chrome
docs (entry/exit `@starting-style`, scroll-driven, view-transition misconceptions), htmx.org
view-transitions essay, caniuse (support matrix). See [epic 0062](0062-epic-playful-polished-ui-motion-overhaul.md).
