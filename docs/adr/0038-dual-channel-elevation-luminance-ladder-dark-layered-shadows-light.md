# 0038. Dual-channel elevation — luminance ladder (dark) / hue-tinted layered shadows (light) — and the constrained radius/space/type scales

- Status: accepted
- Date: 2026-06-10
- Deciders: Horia
- Related: `internal/web/static/app.css`, `internal/web/css_tokens_test.go`
  (the guard: ladder contrast + monotonicity, scale structure, the radius-literal
  ban), [ADR-0036](0036-oklch-palette-rederivation-layer-contract-vendored-open-props.md)
  (the token/`@layer` contract this builds on), REGRESSIONS #20 (light-theme
  contrast), epic [0062](../issues/0062-epic-playful-polished-ui-motion-overhaul.md),
  issue [0064](../issues/0064-elevation-surface-component-restyle.md)

## Context

W2 of epic 0062 gives the UI designed depth. One elevation mechanism cannot
serve both themes: a shadow is invisible against a near-black canvas, and a
lightened surface reads as wash-out against a near-white one. The components
also carried scattered geometry literals (border-radius 4/5/6/8/10/999px, ad-hoc
paddings, ad-hoc letter-spacing), so "restyle" without scales would just move
the literals around. Constraints: ADR-0036's `@layer`/token grammar, WCAG AA in
both themes (the guard + axe scan), no template/route changes.

## Decision

### 1. Elevation is dual-channel, switched by `light-dark()` inside the tokens

- **Dark: a surface luminance ladder.** `--bg` (L 23.5%) → `--panel` (26.5%) →
  `--overlay` (31%, new `--p-gray-8`): raised surfaces step **lighter** —
  Raycast's model. The guard proves the dark ladder is strictly
  lighter-when-raised (`TestSurfaceLadderDarkStepsLighter`) and adds `--overlay`
  to the text-role × surface AA matrix (worst pair ≥ 5:1).
- **Light: hue-tinted layered shadows.** One `--shadow-color`
  (`oklch(35% .05 258 / .06)`, the chrome's slate-blue — never flat black, which
  reads grey-on-grey) feeds `--shadow-1/2/3` as 2/4/5 stacked layers
  (Comeau): all layers share the color, alpha accumulates where they overlap,
  so depth compounds; the whole system retunes by editing one token. In dark
  the same tokens resolve to a soft black ground line (`oklch(0% 0 0 / .22)`) —
  present but subordinate to the ladder. The guard proves each step consumes
  `var(--shadow-color)` and stacks strictly deeper.
- **Glass + atmosphere.** `--border-glass` (1px translucent: black/.08 hairline
  light, white/.14 top-light dark — Arc) on dialogs/popovers; `.glass` and
  `.atmosphere` (low-opacity radial accent glows, reusing the ~10% soft tints so
  text never leaves its AA band) are opt-in utilities consumed by W4.

`--overlay` is a **semantic surface**, not an alias: dialogs (`.overlay-card`)
and popovers (`.cmd-menu`) sit on it with `--shadow-2/3` + the glass border.
(≠ `.overlay`, the scrim container class.)

### 2. Constrained geometry + type scales, with a total radius migration

- **Radius ladder** `--radius-1…5/full` (4/6/8/10/16/999px). The migration is
  total: a `border-radius` px literal outside the tokens layer **fails the
  guard** (`TestNoRadiusLiteralsOutsideTokens`) — the scale cannot erode back
  into literals.
- **Space scale** `--space-1…6` (4/8/12/16/24/32 — 8px rhythm, 4px half-step),
  applied where components were restyled; existing paddings migrate
  opportunistically, not wholesale (a layout-churn risk with no guard value).
- **Type scale** `--text-0…5` on the unchanged system sans + mono pair
  (`--font-sans`/`--font-mono`), with `--tracking-display` (**negative**,
  guarded) at display sizes and `--tracking-caps` for uppercase labels.

## Alternatives rejected

- **Shadows in both themes** (one channel): invisible on dark; the old
  `rgba(0,0,0,.5)` dark shadows added mud, not depth.
- **A translucent white overlay ladder** (`rgba(255,255,255,.0N)` paints, as
  `--raised`/`--raised-2` do): composites escape the oklch guard grammar, so
  the ladder's contrast would be unproven; opaque primitives keep every step in
  the AA matrix. The existing alpha overlays stay for hover/sunken accents only.
- **Open Props size/shadow packs**: rejected in ADR-0036 — the scales here are
  the design research's (Raycast/Geist), not a generic kit's.
- **Tokenizing every padding/margin now**: high churn, no invariant gained;
  the radius ban is the guarded beachhead.

## Consequences

- Elevation reads correctly in both themes and is retunable from two tokens
  (`--shadow-color`, plus the ladder primitives) under a compile-time-style
  guard; axe stays the integration gate (REGRESSIONS #20 carried).
- W4 styles hero surfaces with `.atmosphere`/`.glass`/`--radius-5` without new
  tokens; W3's motion seam (durations/easings/transitions) was untouched.
- New radii must pick a ladder step — a designed constraint, enforced.
