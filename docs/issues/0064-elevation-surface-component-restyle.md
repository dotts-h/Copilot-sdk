---
id: 0064
title: "Elevation, surface & component restyle — luminance ladder + hue-tinted shadows, radius/space scales, component pass (roadmap v11, W2)"
status: closed
severity: medium
group: 0062
depends_on: [0063]
github:
links:
  adr: [0038]      # the dual-theme elevation model (ladder dark / layered tinted shadows light)
  prs: []
  issues: [0062]
  regression: [20]
assets: []
---

## Summary
With the OKLCH token foundation in place (0063), give the UI real **depth** and a designed surface
treatment: a **surface luminance ladder** in dark (raised surfaces step lighter — shadows are
invisible on dark) and **hue-tinted layered shadows** in light (3–5 stacked `box-shadow` layers, one
`--shadow-color` hue variable), plus optional low-opacity **radial-glow "atmosphere"** on hero
surfaces (Arc) and a 1px translucent border on glass. Restyle the core components against the
constrained **radius** (≈4/6/8/10/16/full) and **8px (+4px half-step) spacing** scales, and tighten
the **type scale** (negative tracking at display sizes; sans + mono pairing).

## Scope / Touches
- `internal/web/static/app.css` — semantic `--surface-base/raised/overlay` luminance steps (dark) +
  layered hue-tinted shadow tokens (light); radius + space token scales; type scale + tracking; glass
  border + radial-glow utilities; restyle keycap/kbd, badge/tag, button (primary/ghost/destructive),
  card (KPI, run, session), input, sidebar, command palette, table.
- `internal/web/templates/*.html` — only class/markup tweaks needed to express the restyle (no route
  or data changes).

## Dependencies
- `depends_on: [0063]` — consumes the new tokens (`@layer`, OKLCH surfaces, shadow tokens). Disjoint
  from the motion system (0065) at the seam, but visually best landed before/with it.

## Acceptance
- Depth reads as designed in both themes (luminance ladder dark / hue-tinted layered shadows light);
  components restyled on the constrained radius/space/type scales; **axe both-theme scan green** (the
  recolor + elevation keep AA, incl. the carried REGRESSIONS #20 light-theme guard).
- Failing/guard test where unit-testable; `make lint && make test` + `make e2e` green; born in its PR;
  ADR only if a new elevation model is a decision worth recording.

## Notes
Sources: Raycast surface ladder (awesome-design-md/refero), Arc layered-shadow + glow (blakecrosley),
Josh Comeau "Designing Shadows" (hue-tinted layered shadows, one `--shadow-color`), Geist
radius/space scales. See [epic 0062](0062-epic-playful-polished-ui-motion-overhaul.md).

## Close-out (2026-06-09)

Shipped on the W2 branch with **ADR-0038** (the dual-theme elevation model *was* a decision worth
recording). What landed:

- **ADR-0038** settled the model: elevation lives in the token tier as a **surface ladder**
  (`--surface-base/raised/overlay`; strictly increasing luminance in dark — `--p-gray-8` L .305 is
  the new overlay step — white + shadows in light) plus a **3-step shadow scale** (`--shadow-1..3`,
  2/3/5 stacked layers) derived from **one `--shadow-color`** (terracotta-warm hue 55 at ~half
  strength light, near-black dark) via the established `color-mix(in oklch)` pattern. `--bg`/`--panel`
  became one-level **aliases** of the ladder; `--shadow`/`--shadow-lg` replaced by the scale.
- **Failing tests first** (`css_tokens_test.go`): the ladder joins the contrast guard's surface set
  (every text role AA on every step, both themes — tightest new pair 5.34:1, `--bad` on the dark
  overlay), a **ladder-monotonicity** guard, presence guards for the radius/space/type/shadow scales,
  and **literal bans** — `border-radius` outside the tokens layer must be `var(--radius-*)`/`0`,
  `box-shadow` must be `var(--shadow-*)`/`none`. The contrast guard learned the one-level alias
  grammar. Red on the old file (≈40 literal radii + 2 literal rgba shadows), green on the restyle.
- **Restyle pass**: radius ladder 4/6/8/10/16/pill swept through every component; core component
  padding/gaps normalized to the 8px(+4px) `--space-1..6` grid; `--text-0..5` type scale over
  `--font-sans`/`--font-mono` with `--tracking-tight` at display sizes (page h2, KPI values) and one
  `--tracking-wide` for the uppercase micro-labels; keycap `kbd` (raised surface, 2px bottom edge);
  cards (KPI/run/workflow/session) moved to `--surface-raised` + `--border-glass` rim + `--shadow-1`;
  overlay/⌘K/cmd-menu moved to `--surface-overlay` + glass rim + `--shadow-3` (radius-5 hero);
  `.glass`/`.glow` utilities staged for W4; the chip-model `opacity` dim replaced with `--dim`
  (REGRESSIONS #20/#21 pattern). Zero template changes — the restyle was expressible in CSS alone.
- Deferred: P3-gamut accent variants (`@media (color-gamut: p3)`) still wait on the guard learning
  gamut-conditional primitives (carried from the 0063 close-out).
- Gates: `make lint && make test` green; `make e2e` green including the axe both-theme scan.
