# 0037. The motion system: linear() springs behind an @supports gate, a motion-token vocabulary, the CSS-only micro-interaction catalogue, and the view-transition policy re-affirmed

- Status: accepted
- Date: 2026-06-10
- Deciders: Horia
- Related: `internal/web/static/app.css` (tokens/base/components layers),
  `internal/web/css_tokens_test.go` (the motion guard, §3),
  [ADR-0036](0036-oklch-palette-rederivation-layer-contract-vendored-open-props.md)
  (the `@layer` contract + the vendored Open Props easings this consumes),
  [ADR-0028](0028-motion-and-polish-htmx-per-navigation-view-transitions.md)
  (the per-nav view-transition decision this re-affirms), REGRESSIONS
  (the `globalViewTransitions` dead-end), epic
  [0062](../issues/0062-epic-playful-polished-ui-motion-overhaul.md),
  issue [0065](../issues/0065-motion-microinteraction-system.md)

## Context

W3 of epic 0062 ships the "delight" layer: a cohesive spring motion system in
pure CSS — no build chain, no framework, no new JS. The foundation exists
(ADR-0036): the vendored Open Props easings carry real `linear()` spring
curves (`--ease-spring-1…5`), `@property --mix-soft` was registered animatable
for exactly this slice, and the `@layer` contract bounds where rules may live.
The decisions to settle: how `linear()` springs degrade on engines without
`linear()` support; the motion-token vocabulary later children build with; how
the catalogue stays safe for the htmx OOB/SSE streaming swaps; and whether the
view-transition policy changes.

## Decision

### 1. Springs via `linear()`, gated — a cubic-bezier overshoot is the default

A raw `linear(…)` value in a *used* property is **invalid at computed-value
time** on engines without `linear()` support, which silently degrades the
timing function to `ease` — the worst failure mode (no error, wrong feel). So
the default is inverted:

```css
--ease-spring: var(--ease-overshoot);          /* cubic-bezier(0.34,1.56,0.64,1) */
@supports (animation-timing-function: linear(0, 1)) {
  :root { --ease-spring: var(--ease-spring-2); } /* the real Open Props spring */
}
```

The fallback is the *declared* value; the `@supports` block is the upgrade.
The guard (`TestMotionSpringSupportsGate`) pins that **every** `linear()` /
Open Props spring/bounce reference in `app.css` sits inside that gate and that
an ungated cubic-bezier `--ease-spring` exists. (`@supports` nests *inside*
`@layer tokens` — the structure guard's no-un-layered-rules invariant holds.)

### 2. The motion-token vocabulary (tokens layer, one home)

Durations extend the existing `--dur-*` family (no parallel scale):
`--dur-1` .15s micro (hovers, focus, cross-fades) · `--dur-2` .35s transforms
(lifts, presses, enter/exit) · `--dur-3` .7s springs · `--dur-4` 1.2s ambient
loops (shimmer). Easings: `--ease` (the V24 workhorse), `--ease-out-quint`
`cubic-bezier(0.23,1,0.32,1)` (decisive settle), `--ease-overshoot`
`cubic-bezier(0.34,1.56,0.64,1)` (playful pop), `--ease-spring` (above).
`TestMotionTokenVocabulary` pins the curves and the duration bands. Components
never hard-code a duration or curve.

### 3. The micro-interaction catalogue — CSS only, streaming-safe by construction

All in the components layer, animating only transform/opacity/shadow or a
registered token (never a color pairing — the axe both-theme scan is
unaffected; never layout):

- **hover-lift** — cards rise 2px and move `--shadow` → `--shadow-lg`
  (referencing the W2 elevation tokens by name, not redefining them);
- **press** — `scale(.97)` on `:active`; the solid primary buttons keep the
  V24 1px sink, joined with the compress;
- **focus ring polish** — `outline-offset` eases 0→2px; the ring itself
  appears instantly at full size/contrast (WCAG 2.4.7 intact);
- **skeleton shimmer** — `.skeleton` pulses the registered `--mix-soft`
  through the soft-fill `color-mix()` derivation (typed `<percentage>`, so it
  interpolates); theme-aware, no gradient math. W4 applies it to the hero
  surfaces;
- **enter/exit** — `@starting-style` + `transition-behavior: allow-discrete`
  on the `[hidden]`-toggled overlays (help + ⌘K palette: scrim fade, card
  spring-pop) and the palette's filtered items; one-shot entry on the
  append-once action cards (`#perms > .perm` etc.) and the `p.ok`/`p.error`
  flash notes. **Child combinators are load-bearing**: a lane-embedded `.perm`
  rides the high-frequency `lanes` innerHTML swap and would re-trigger its
  entry on every update. No new JS — `setOverlay`'s `[hidden]` toggle is the
  only state hook;
- **scroll-driven reveal** — `ul.rows .row` slides in via
  `animation-timeline: view()`, double-gated (see §5). Transform only, never
  opacity: a half-revealed row must not fail the contrast scan.

Streaming safety: element transitions/animations run *after* a swap lands —
they cannot wrap, defer, or drop it (unlike a view transition). The
multi-agent/stream e2e specs are the integration gate.

### 4. View-transition policy: per-nav opt-in re-affirmed, global rejected again

Unchanged from ADR-0028, now guarded at unit level
(`TestMotionNoGlobalViewTransitions` greps the embedded templates + static JS
for `globalViewTransitions: true`, pins the nav links' `transition:true`, and
asserts no SSE listener carries a `transition` modifier). The platform runs
**one** view transition per document; global wrapping aborts transitions
against each other and drops `hx-swap-oob`/SSE swaps — the recorded
REGRESSIONS dead-end, independently re-confirmed by the 0062 research. Scoped
element transitions for in-place swaps were considered and **deferred**: every
in-place swap in this UI is streaming-adjacent (lanes/status/timeline), so the
risk buys nothing the catalogue's plain transitions don't already give.

### 5. Reduced motion is a hard invariant — and the star reset has a blind spot

The base-layer guard (`*` duration/iteration reset + the explicit
`::view-transition-*` `animation: none`) collapses every transition, keyframe
animation, `@starting-style` entry, and `allow-discrete` exit to ~0. But a
**scroll-driven timeline ignores durations** — zeroing `animation-duration`
does not disable a `view()` animation. So every `animation-timeline` use is
double-gated: inside `@supports (animation-timeline: view())` *and* an
explicit `@media (prefers-reduced-motion: no-preference)`.
`TestMotionReducedMotionGuard` pins both gates and the star reset's reach.

## Consequences

- A spring vocabulary with one fact-one-home tokens: retuning the app's feel
  is a tokens-layer edit; engines without `linear()` get a coherent overshoot,
  never a silent `ease`.
- The catalogue ships with zero template/JS changes and zero new contrast
  pairings — the axe both-theme scan and the streaming specs gate it; the new
  unit guards (`TestMotion*`) fail loud before e2e ever runs.
- The global-view-transitions dead-end is now machine-enforced, not just
  documented: a future `htmx.config.globalViewTransitions = true` fails
  `go test`.
- `.skeleton` lands as a documented, currently-unconsumed primitive — W4
  (issue 0066) is its consumer; revisit there if it stays unused.
- Firefox sees no scroll-driven reveals (no `animation-timeline`) and pre-17.2
  Safari no spring curves — both degrade to the already-shipped baseline, by
  construction of the gates.
