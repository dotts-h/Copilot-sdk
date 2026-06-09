# 0038. Dual-theme elevation model — surface luminance ladder (dark), hue-tinted layered shadows (light) — and the constrained geometry/type scales

- Status: accepted
- Date: 2026-06-09
- Deciders: Horia
- Related: `internal/web/static/app.css` (the single hand-written stylesheet),
  `internal/web/css_tokens_test.go` (the structure/contrast guard, extended here),
  [ADR-0036](0036-oklch-palette-rederivation-layer-contract-vendored-open-props.md)
  (the OKLCH primitives + `@layer` contract this builds on),
  [ADR-0025](0025-design-token-foundation-and-light-dark-theming.md) (the
  `light-dark()` semantic tier), REGRESSIONS #20/#21 (the `opacity`-dimming
  contrast trap), epic [0062](../issues/0062-epic-playful-polished-ui-motion-overhaul.md),
  issue [0064](../issues/0064-elevation-surface-component-restyle.md)

## Context

W2 of epic 0062 gives the UI real depth. Two facts force a *dual* model:

1. **Shadows are nearly invisible on a dark canvas.** Raycast-style dark UIs
   build elevation by *stepping surface luminance* — raised surfaces are
   lighter — with almost no shadow.
2. **On a light canvas, flat gray shadows look muddy.** Arc/Comeau-style depth
   comes from **3–5 stacked `box-shadow` layers tinted toward the canvas hue**,
   derived from a single shadow-color variable.

The same component must read as "raised" in both themes without per-theme
component rules — elevation has to live in the **token tier**, like color
already does (ADR-0025/0036). The restyle also needs the constrained geometry
the research prescribes (a small radius ladder, an 8px + 4px-half-step spacing
rhythm, a tight type scale with negative display tracking) so components stop
accreting ad-hoc `6px`/`8px`/`.55rem` literals.

## Decision

### 1. Elevation is three semantic surface tokens + three shadow tokens

A **surface ladder** — `--surface-base` (the page), `--surface-raised`
(cards/panels), `--surface-overlay` (modals, menus, the palette) — each a
`light-dark()` pair over OKLCH primitives:

| token               | light theme            | dark theme                  |
|---------------------|------------------------|-----------------------------|
| `--surface-base`    | warm paper (L ≈ .992)  | charcoal (L ≈ .235)         |
| `--surface-raised`  | white (L = 1)          | charcoal raised (L ≈ .265)  |
| `--surface-overlay` | white (L = 1)          | charcoal overlay (L ≈ .305) |

In **dark** the ladder is strictly increasing in L (the luminance *is* the
elevation); in **light** raised/overlay share white and the **shadows** carry
the depth. `--bg`/`--panel` become one-level **aliases** of
`--surface-base`/`--surface-raised` (one fact, one home — a retune edits the
ladder, not two names), and the contrast guard learns to resolve aliases.

A matching **shadow scale** — `--shadow-1` (resting), `--shadow-2` (raised),
`--shadow-3` (overlay) — builds every layer from **one `--shadow-color`**:
hue-tinted toward the terracotta-warm canvas (hue ≈ 55) at ~half strength in
light, a near-black at full strength in dark (dark shadows only add edge
definition under the ladder; they must read stronger to register at all).
Per-layer opacity derives via the established `color-mix(in oklch, …,
transparent)` pattern — no scattered `rgba()` literals.

A **1px translucent glass border** token (`--border-glass`) and a low-opacity
**radial-glow** utility (`.glow`, accent-soft atmosphere) complete the Arc
register; overlay surfaces wear the glass border now, hero glow application is
W4's call.

### 2. Constrained geometry and type, as tokens with a guard

- **Radius ladder:** `--radius-1…5` = 4/6/8/10/16 px + `--radius-full` (pill).
  Small controls/tags take 1–2, cards/turns 3–4, hero/overlay surfaces 5.
- **Spacing scale:** `--space-1…6` = 4/8/12/16/24/32 px (rem-denominated) — the
  8px rhythm with the 4px half-step. Core component padding/gaps normalize to it.
- **Type scale:** `--text-0…5` (.72/.8/.9/1/1.18/1.35 rem) over `--font-sans` /
  `--font-mono` stacks, with `--tracking-tight` (−0.015em) applied at display
  sizes (page headings, KPI values) and `--tracking-wide` for the uppercase
  micro-labels. Body stays 15px; "engineered" reads through tightness, not size.

The structure guard is extended to make the model self-enforcing, in the same
spirit as the `@layer` contract:

1. `border-radius` outside the tokens layer may use **only** `var(--radius-*)`
   (or `0`) — the literal-radius accretion path is closed.
2. `box-shadow` outside the tokens layer may use **only** `var(--shadow-*)`
   (or `none`) — shadows can't bypass the single `--shadow-color` derivation.
3. The surface ladder joins the **contrast guard's surface set**: every text
   role must clear WCAG AA on every ladder step in both themes (this is what
   pins the dark overlay step's L — the lowest-L bright text roles bound it).
4. Ladder **monotonicity**: relative luminance must be strictly increasing
   base→raised→overlay in dark, non-decreasing in light.

### 3. What this is NOT

- **No motion.** Hover-lift/press springs are W3 (issue 0065, reserved
  ADR-0037); W2 ships resting elevation only, so the two stay disjoint at the
  seam.
- **No P3-gamut accent variants.** A `@media (color-gamut: p3)` chroma boost
  would need the contrast guard to learn gamut-conditional primitives first;
  deferred (recorded on the issue close-out).
- **No `opacity` dimming anywhere new** — the REGRESSIONS #20/#21 trap; dim
  text uses the AA-tuned `--dim` token.

## Consequences

- Elevation, like color, is now a one-layer token edit with a compile-time
  guard: a new surface step or shadow level extends a slice in the test and a
  token in the ladder, and AA + monotonicity are proven before axe runs.
- Components stop carrying geometry literals; the radius/shadow bans mean the
  scales can't erode silently (the guard names the offending rule).
- The dark theme gains one primitive (`--p-gray-8`, the overlay step); the old
  `--shadow`/`--shadow-lg` pair is replaced by the 3-step scale.
- W3's hover-lift has its raw material (`--shadow-2` as the lifted state) and
  W4's hero polish has `.glow`/`.glass` ready to apply.
- Trade-off: spacing/type adoption is normative, not guard-banned (a literal
  `padding` ban would outlaw legitimate optical offsets); the scales are
  presence-guarded and applied to the core components, and drift is a review
  concern, not a test failure.
