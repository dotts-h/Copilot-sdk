---
id: 0064
title: "Elevation, surface & component restyle — luminance ladder + hue-tinted shadows, radius/space scales, component pass (roadmap v11, W2)"
status: closed
severity: medium
group: 0062
depends_on: [0063]
github:
links:
  adr: [0038]      # the dual-channel elevation model was a decision worth recording
  prs: [105]
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

## Resolution (shipped)

Shipped in **PR #105** with **ADR-0038** (the dual-channel elevation model). What landed:

- **Dual-channel elevation:** dark = surface luminance ladder `--bg` → `--panel` → `--overlay`
  (new `--p-gray-8`, L 31% — raised steps lighter); light = hue-tinted layered shadows, one
  `--shadow-color` feeding `--shadow-1/2/3` (2/4/5 Comeau-style stacked layers); `--border-glass`
  1px glass border; `.glass`/`.atmosphere` opt-in utilities staged for W4.
- **Scales:** radius `--radius-1…5/full` with a **total** migration (px-literal ban guarded),
  space `--space-1…6` (8px rhythm, 4px half-step), type `--text-0…5` + negative
  `--tracking-display` / `--tracking-caps` on the unchanged sans+mono stacks.
- **Guard extended** (failing first): `--overlay` joined the text-role × surface AA matrix (worst
  new pair ≈ 5.25:1 dark), `TestSurfaceLadderDarkStepsLighter` (ladder monotonicity),
  `TestTokenScales` (scale structure, shadow stacking depth, negative display tracking),
  `TestNoRadiusLiteralsOutsideTokens`.
- **Component pass:** keycap kbd, badges/tags, buttons, KPI/run/workflow/session cards, inputs,
  sidebar, ⌘K palette + dialogs (now on `--overlay` with `--shadow-3` + glass border), tables.
  **Zero template changes**; the 0065 motion seam untouched.
- Gates: `make lint && make test` green (web 90.3%), `make e2e` 142 passed — both-theme axe scan
  green (REGRESSIONS #20 carried).
