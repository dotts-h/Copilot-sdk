---
id: 0063
title: "Token & palette foundation — re-derive the palette in OKLCH, @layer structure, Open Props primitives (roadmap v11, W1)"
status: open
severity: medium
group: 0062
depends_on: []
github:
links:
  adr: []          # reserves ADR-0036 (palette re-derivation + @layer + Open Props; keep-or-drop terracotta identity)
  prs: []
  issues: [0062]
  regression: [20]
assets: []
---

## Summary
The foundation of the playful-polished overhaul (epic 0062): re-derive the color system in **OKLCH**
so we can raise saturation for a "playful" register **without breaking WCAG AA** (contrast tracks
lightness, which is chroma-independent in OKLCH — hold each role to a fixed `L` band), restructure
`app.css` with **CSS `@layer`** for predictable overrides, and bring in **Open Props** primitives
(vendored, no build/CDN) for ramps/sizes/easings/`@keyframes`, with our own **semantic** layer on
top. Everything later (surfaces, motion) builds on these tokens.

## Scope / Touches
- `internal/web/static/app.css` — `@layer tokens, base, components, utilities;`; primitive OKLCH
  ramps (neutrals + accents) → semantic tokens (`--surface-*`/`--text-*`/`--accent-*`, keeping
  `light-dark()`) → component tokens; `color-mix(in oklch …)` hover/press/disabled tints from one
  accent; `@property` for animatable token(s); vendor Open Props (one file or a curated easings +
  animations subset) under `internal/web/static/`.
- **ADR (reserved 0036):** the money decision here is **identity** — does the re-derived palette keep
  the terracotta/copilot-blue heritage or move to a new accent? — plus "adopt Open Props vs hand-roll
  the easing/animation primitives", and the `@layer` ordering contract.
- Browser-support guards: `@media (color-gamut: p3)` + sRGB fallback for any P3 accent; OKLCH /
  `color-mix` / `@layer` / `light-dark` are all Baseline, no `@supports` needed.

## Dependencies
- `depends_on: []` — first slice; the rest of the epic consumes these tokens. Lands behind the axe
  both-theme gate so a saturated re-derivation can't silently regress contrast (fully reversible).

## Acceptance
- Palette re-derived in OKLCH with documented per-role `L` bands; **axe both-theme scan green**
  (validated with APCA/Huetone, not eyeballed); the three-tier token structure + `@layer` is in place;
  Open Props vendored offline (no CDN) or its primitives deliberately hand-rolled (ADR-0036 records
  which). No visual regression of existing components beyond the intended recolor.
- Failing test first (a contrast/token guard where unit-testable); `make lint && make test` + `make
  e2e` green; born in its PR; ADR-0036.

## Notes
Highest-risk slice (could regress AA) → lands first, behind the gate. See [epic 0062](0062-epic-playful-polished-ui-motion-overhaul.md)
and NEXT_FEATURES "Roadmap v11". Sources: Evil Martians (OKLCH), LogRocket (accessible OKLCH ramps),
Linear redesign (generative LCH theme), open-props.style, MDN (`@layer`/`color-mix`/`light-dark`).
