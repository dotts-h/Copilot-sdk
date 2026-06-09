---
id: 0064
title: "Elevation, surface & component restyle — luminance ladder + hue-tinted shadows, radius/space scales, component pass (roadmap v11, W2)"
status: open
severity: medium
group: 0062
depends_on: [0063]
github:
links:
  adr: []          # an ADR only if a new dual-theme elevation model is decided
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
