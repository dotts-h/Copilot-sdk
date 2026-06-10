---
id: 0065
title: "Motion & micro-interaction system — linear() spring easings, motion tokens, the CSS-only interaction catalogue (roadmap v11, W3)"
status: closed
severity: medium
group: 0062
depends_on: [0063]
github:
links:
  adr: [0037]
  prs: [106]
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

## Resolution (shipped)

Shipped in **PR #106** with
[ADR-0037](../adr/0037-motion-system-linear-springs-motion-tokens-view-transition-policy.md). What landed:

- **Motion tokens** (tokens layer): `--dur-1..4` (.15s/.35s/.7s/1.2s — extends the existing family),
  `--ease-out-quint`, `--ease-overshoot`; `--ease-spring` defaults to the overshoot cubic-bezier and
  is upgraded to the vendored Open Props `linear()` spring inside
  `@supports (animation-timing-function: linear(0, 1))` — fallback-first, never a silent IACVT
  degrade to `ease`.
- **Catalogue** (all CSS, zero template/JS changes): hover-lift on the card surfaces (moving between
  the W2-owned `--shadow`/`--shadow-lg` by name), `scale(.97)` press (primaries keep + join the V24
  1px sink), `:focus-visible` `outline-offset` ease (ring intact), `.skeleton` pulsing the registered
  `@property --mix-soft`, `@starting-style` + `allow-discrete` enter/exit for the help/⌘K overlays,
  the palette's `[hidden]`-filtered items, the append-once action cards (`#perms > .perm` etc. —
  child combinators keep the high-frequency `lanes`/budget innerHTML swaps out) and the `p.ok`/
  `p.error` flash notes, plus a transform-only scroll-driven reveal double-gated behind
  `@supports (animation-timeline: view())` + `prefers-reduced-motion: no-preference`.
- **Guard extended test-first** (`css_tokens_test.go` §3): token vocabulary bands/curves, the
  linear() `@supports` gate + ungated fallback, reduced-motion reach (incl. the view()-timeline
  blind spot), and the grep-shaped no-`globalViewTransitions` policy guard over the embedded FS.
- **View-transition policy**: per-nav opt-in re-affirmed; global rejected again (REGRESSIONS
  dead-end), now machine-enforced; scoped element transitions considered and deferred (every
  in-place swap is streaming-adjacent).
- Gates: `make lint && make test` green (web 90.4%); `make e2e` green — first full run 144/144
  incl. the both-theme axe scan + multi-agent/stream specs (a later run's 2 retried flakes
  reproduce identically on plain origin/main: the pre-existing CPU-race class documented in
  playwright.config.ts, not introduced here).
