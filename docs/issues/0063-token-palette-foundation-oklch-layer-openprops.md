---
id: 0063
title: "Token & palette foundation — re-derive the palette in OKLCH, @layer structure, Open Props primitives (roadmap v11, W1)"
status: closed
severity: medium
group: 0062
depends_on: []
github:
links:
  adr: [0036]
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

## Close-out (2026-06-09)

Shipped on the W1 branch with ADR-0036. What landed:

- **ADR-0036** settled the three decisions: terracotta/copilot-blue identity **kept** (re-derived,
  chroma ≈ +25–40% inside fixed per-role L bands); **Open Props adopted** as a vendored two-file
  subset (v1.7.23 `easings` + `animations`, MIT, byte-for-byte, imported into the tokens layer —
  includes the `linear()` springs W3 consumes); `@layer tokens, base, components, utilities` as the
  ordering contract with a no-un-layered-rules invariant.
- **Failing test first:** `internal/web/css_tokens_test.go` — the structure guard (layer order,
  un-layered-rule ban, vendored imports, the components-never-touch-`--p-*` tiering rule, ≥ 1
  `@property` registration) plus a **WCAG contrast guard** that converts every `oklch()` primitive
  to sRGB (Ottosson matrices, out-of-gamut = fail) and asserts AA (≥ 4.5:1) for 13
  text-role/surface pairs in both themes. Red on the old flat file, green on the rewrite.
- **`app.css` restructured** into the four layers: a 19-step OKLCH primitive ramp
  (`--p-gray-*`, terracotta/blue/green/gold/red), the unchanged ADR-0025 semantic names re-pointed
  at primitives via `light-dark()`, and `color-mix(in oklch)` state tints (`--*-soft` soft fills,
  `--btn-bg-hover/press`, chip tokens) replacing every scattered `rgba(…)` tint literal and the
  `filter: brightness()` hover hack. `@property --mix-soft` registers the animatable tint strength
  (the W3 seam). Duration scale (`--dur-1..3`) + Open Props easing aliases staged for W3.
- Gates: `make lint && make test` green (web 90.3%), `make e2e` 142 passed — the axe both-theme
  scan stayed green over the recolor (lowest guarded pair: accent-on-bg light, 5.39:1).
- Learning: the "playful" chroma targets (terracotta C 0.14, gold C 0.11 at light-text L) sit
  **outside the sRGB gamut** — the guard's gamut check caught both; shipped C 0.125/0.103. P3-gamut
  variants behind `@media (color-gamut: p3)` remain W2 material.
