# 0036. OKLCH palette re-derivation (terracotta identity kept), the `@layer` ordering contract, and a vendored Open Props subset

- Status: accepted
- Date: 2026-06-09
- Deciders: Horia
- Related: `internal/web/static/app.css` (the single hand-written stylesheet),
  `internal/web/static/open-props.easings.min.css` +
  `internal/web/static/open-props.animations.min.css` (vendored, v1.7.23, MIT),
  `internal/web/css_tokens_test.go` (the contrast/structure guard),
  [ADR-0025](0025-design-token-foundation-and-light-dark-theming.md) (the
  `light-dark()` semantic token layer this builds on),
  [ADR-0028](0028-motion-and-polish-htmx-per-navigation-view-transitions.md)
  (the motion pass that deferred Open Props), REGRESSIONS #20 (light-theme
  contrast), epic [0062](../issues/0062-epic-playful-polished-ui-motion-overhaul.md),
  issue [0063](../issues/0063-token-palette-foundation-oklch-layer-openprops.md)

## Context

Epic 0062 builds the "playful-polished" register (Raycast discipline × Arc
delight) that v9 deliberately deferred. Its foundation slice (0063 / W1) needs
three decisions settled before the CSS is touched:

1. **Identity** — does re-deriving the palette keep the terracotta/copilot-blue
   heritage, or move to a new accent?
2. **Open Props** — adopt it (vendored, offline) for the easing/animation
   primitives, or hand-roll them?
3. **Layering** — what is the `@layer` ordering contract that keeps the single
   hand-written `app.css` legible and its overrides predictable as W2–W4 grow it?

Hard constraints (epic charter): no build chain, a single committed hand-written
`app.css`, vendored static assets only (offline single binary), and WCAG 2 AA in
*both* themes as a gate (the axe both-theme scan; REGRESSIONS #20).

Why OKLCH at all: in OKLCH, perceived lightness (`L`) is independent of chroma,
so **contrast is a function of lightness alone** — chroma can be raised for the
playful register without moving a color off its AA-safe band. The old hex
palette had no such invariant: every retune was a manual re-audit.

## Decision

### 1. Keep the terracotta + copilot-blue identity, re-derived in OKLCH

The palette is **re-derived, not replaced**: warm terracotta (hue ≈ 55) stays
the primary accent and copilot blue (hue ≈ 255) the secondary, with chroma
raised (≈ +25–40%) inside **fixed per-role lightness bands**:

| role (as text)        | light theme           | dark theme            |
|-----------------------|-----------------------|-----------------------|
| surface (`--bg`)      | L ≥ 0.97              | L ≤ 0.26              |
| body text (`--fg`)    | L ≤ 0.32              | L ≥ 0.90              |
| dim text (`--dim`)    | L ≈ 0.45–0.50         | L ≈ 0.70–0.75         |
| accents/status as text| L ≤ 0.55              | L ≥ 0.70              |

Rationale: the terracotta thread runs from the TUI palette
(`internal/tui/styles.go`) through ADR-0025; "playful" comes from chroma,
depth (W2) and motion (W3), not from a rebrand. Fully reversible — the
semantic token names are unchanged, so a future identity swap edits only the
primitive ramp.

Each band is **validated programmatically, not eyeballed**: a Go test converts
every `oklch()` primitive to sRGB and asserts WCAG AA (≥ 4.5:1) for every
text-role/surface pair in both themes, plus `--on-bright` on every solid
accent/status fill. The axe both-theme e2e scan stays as the integration gate.

### 2. Adopt Open Props — a vendored two-file subset (easings + animations)

Vendor exactly two published files from **Open Props v1.7.23** (MIT, Adam
Argyle) under `internal/web/static/`, byte-for-byte as published (auditable
against unpkg):

- `open-props.easings.min.css` — the easing ramp (`--ease-1…5`, in/out/elastic)
  **including the `linear()` spring/bounce set** (`--ease-spring-1…5`) that W3
  (issue 0065) consumes.
- `open-props.animations.min.css` — the `@keyframes` catalogue + composed
  `--animation-*` shorthands (fade/scale/slide/shake…), W3's raw material.

They load via layered imports at the top of `app.css` (no template change, no
extra `<link>`, same-origin so still offline):

```css
@layer tokens, base, components, utilities;
@import url("open-props.easings.min.css") layer(tokens);
@import url("open-props.animations.min.css") layer(tokens);
```

The **color/size/shadow packs are deliberately NOT adopted**: the OKLCH
re-derivation above *is* the color story (Open Props ramps would fight it),
and W2 derives its elevation/radius/space scales from the design research, not
a generic kit. Hand-rolling the easings was rejected: the published curves are
battle-tested, the spring `linear()` strings are tedious to generate correctly,
and two minified files (~9 kB total) cost nothing under the vendored-assets
rule (precedent: `htmx.min.js`).

### 3. The `@layer` ordering contract

```
@layer tokens, base, components, utilities;
```

- **tokens** — `@property` registrations aside (see below), *only* custom
  properties: primitive OKLCH ramps (`--p-*`), the semantic tier
  (`--bg`/`--fg`/`--accent`/… via `light-dark()`), component/state tokens
  (`color-mix()` tints), motion durations/easings. The Open Props imports land
  here (zero-specificity `:where(html)` selectors).
- **base** — element defaults and global behavior: resets, `html`/`body`,
  `:focus-visible`, the reduced-motion guard, view-transition wiring.
- **components** — every `.class` component rule (the bulk of the file).
- **utilities** — single-purpose overrides that must beat components
  (`.sr-only`, `.dim`).

The contract, enforced by the structure guard in `css_tokens_test.go`:

1. The layer-order statement is the **first rule** of `app.css`; imports follow.
2. **No un-layered rules** — an un-layered rule outranks *every* layer, so one
   stray block would silently invert the whole cascade. Every top-level
   construct must be `@layer`, `@import`, or `@property`.
3. **Tiering** — components never reference primitives (`var(--p-*)` is banned
   outside the tokens layer); they consume semantic/component tokens only, so
   a palette retune touches one layer.
4. `@property` registrations sit at the top level (registration is
   cascade-independent; keeping them outside `@layer` makes that explicit).

## Consequences

- A palette retune is now a one-layer edit with a compile-time-style guard:
  the Go test fails before axe ever runs, and names the exact failing pair.
- `color-mix(in oklch, …)` state tints (hover/press/soft fills) derive from
  the accent tokens, replacing the scattered raw `rgba(…)` literals — the
  "tokens, never literals" doctrine (ADR-0025) finally covers the tint cases.
- W2/W3 inherit their raw material (luminance-band primitives, spring easings,
  keyframes) without new dependencies or template changes.
- Two more static files ship in the embedded FS (~9 kB); the upgrade path is
  re-fetching the published files and re-running the guard.
- OKLCH / `color-mix()` / `@layer` / `light-dark()` are all Baseline; no
  `@supports` gating needed at this tier (scroll-driven animations and P3
  accents, which do need gates, are W2/W3 concerns).
