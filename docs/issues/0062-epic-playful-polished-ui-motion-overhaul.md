---
id: 0062
title: "Epic: Playful-polished visual + motion overhaul — re-derived OKLCH palette, depth, and a spring motion system (roadmap v11)"
status: closed
severity: medium
group:
github:
links:
  adr: [0036, 0037, 0038]
  prs: [101, 105, 106, 107]
  issues: [0063, 0064, 0065, 0066]
  regression: [20]
---

## Charter

Roadmap **v9** (epic 0045) modernized the *structure* of the web UI — a semantic light/dark token
layer, a grouped sidebar + ⌘K palette, a KPI telemetry dashboard, and a **restrained** motion pass
(per-nav view-transitions, eased hovers, a 1px button press, card elevation). It deliberately
**deferred** the richer-aesthetic + animation layer (Open Props primitives, CSS `@layer`,
spring motion) as a conscious trade-off — see [ADR-0028](../adr/0028-motion-and-polish-htmx-per-navigation-view-transitions.md)
and NEXT_FEATURES "roadmap v10 … carried Open-Props paydown". The result is correct and accessible
but reads *structural*, not *designed*: still chrome, little life.

This epic builds that deferred layer into a full **playful-polished** overhaul — the Raycast/Arc
register: bold, re-derived color; real depth; and a **spring-based motion system** with delightful
micro-interactions — entirely within the hard constraints (**one hand-written CSS file, htmx + Go
`html/template`, vendored static assets, no build chain, no framework, minimal vanilla JS**). The
research below (2026-06-09, five-leg cited pass) confirms modern vanilla CSS now covers all of it.

> **Hard constraints (a child that would relax one must call it out in its ADR):** no build chain;
> single committed `internal/web/static/app.css`; htmx + server templates; minimal vanilla JS / no
> framework; no CDN (offline single binary). **A11y is a gate:** WCAG 2 AA contrast holds in *both*
> themes (the axe both-theme scan), `:focus-visible` rings, and **all** motion collapses under
> `prefers-reduced-motion`.

## Design direction (from the cited research)

### A. Aesthetic target — "playful-polished" (Raycast discipline × Arc delight)
- **Depth, two ways.** Raycast builds elevation *by stepping surface luminance on a near-black
  ladder with almost no shadows*; Arc builds it *with layered shadows + a 1px translucent glow border
  + low-opacity radial "atmosphere" glows*. We take **luminance-ladder in dark** (shadows are
  invisible there) and **hue-tinted layered shadows in light**, plus optional radial-glow
  backgrounds on hero surfaces. — refero/awesome-design-md (Raycast), blakecrosley (Arc),
  joshwcomeau (shadows)
- **Saturated accents used sparingly.** Bold primary + 3–4 saturated category hues, each paired with
  a ~15%-opacity soft fill, used for emphasis/illustration — never as large backgrounds. — Raycast
- **Tight, engineered type.** A constrained scale with negative tracking at display sizes; one sans
  (the current stack or Inter w/ `ss03`) + a mono for code/keycaps/metadata. — Raycast, Geist
- **Constrained geometry.** A small radius ladder (≈ 4/6/8/10/16/full) and an 8px (with a 4px
  half-step) spacing rhythm. — Raycast, Geist

### B. Palette + token system (re-derive in OKLCH; AA-safe while "playful")
- **OKLCH** for every ramp: equal `L` steps read as equal brightness across hues, so **contrast is a
  function of lightness, independent of chroma** — we can crank saturation for "playful" without
  breaking AA by holding each role to a fixed `L` band (e.g. text `L≤0.30` on surface `L≥0.90`
  light; surface `L≈0.15`, accent `L≈0.65–0.70` for white text in dark). Validate with APCA /
  Huetone, not eyeballing. — evilmartians, logrocket, Linear redesign
- **Three-tier tokens**: primitive (OKLCH ramps) → semantic (`--surface-*`, `--text-*`,
  `--accent-*`) → component; components never reference primitives. Keep `light-dark()` for the
  dual-theme single-declaration tokens (already in use). — penpot, muzli
- **`color-mix(in oklch …)`** to derive hover/press/disabled tints from one accent token (preserves
  chroma where sRGB/HSL go muddy). **`@property`** to make gradient stops / numeric tokens
  animatable. — MDN, chrome docs
- **`@layer tokens, base, components, utilities`** to make overrides predictable and keep the single
  file legible as it grows (Baseline since 2022). — MDN, css-tricks
- **Open Props** (vendored as one ~4.4 kB file, or per-PropPack — *no npm, no CDN*) for primitive
  ramps/sizes/easings/`@keyframes`; we add the **semantic** layer on top (Open Props is primitives
  only). Evaluate "adopt vs hand-roll just the easings/animations" in ADR-0036. — open-props.style,
  css-tricks

### C. Motion system (spring, CSS-only, htmx/SSE-safe)
- **`linear()` spring/bounce easings** — real spring curves in pure CSS (Chrome 113+/FF 112+/Safari
  17.2+); generate with Jake Archibald's linear-easing-generator; ship **motion tokens**
  (`--ease-spring`, `--ease-out-quint` `cubic-bezier(0.23,1,0.32,1)`, `--ease-overshoot`
  `cubic-bezier(0.34,1.56,0.64,1)`) and **duration tokens** (micro ~150–200ms, transform ~300–400ms,
  spring/bounce ~600–1200ms). Gate `linear()` behind `@supports` with a cubic-bezier fallback. —
  joshwcomeau, carmenansio, web.dev, chrome docs
- **Micro-interaction catalogue, all CSS:** hover-lift (`translateY` + shadow), press (`scale(.97)`
  on `:active`), `:focus-visible` ring, **skeleton shimmer** (gradient + `background-position`
  keyframe, or `@property` stop), **list enter/exit + toast + palette open** via
  `@starting-style` + `transition-behavior: allow-discrete` + `popover`/`dialog` (Baseline Aug 2024),
  optional **scroll-driven reveals** (`animation-timeline: view()` — Chrome/Safari only, **gate with
  `@supports`; Firefox has none**). — chrome entry/exit docs, scroll-driven docs, caniuse
- **View Transitions stay PER-NAV opt-in — never `globalViewTransitions`.** The platform runs **one
  transition per document at a time**; wrapping every swap aborts transitions against each other and
  drops htmx `hx-swap-oob` / streamed SSE updates — *exactly the failure ADR-0028 / REGRESSIONS
  already recorded.* The research independently confirms it (Chrome "misconceptions", htmx essay). We
  may add **scoped** element transitions for in-place swaps, gated. — chrome docs, htmx.org,
  REGRESSIONS (global-VT dead-end)
- **`prefers-reduced-motion`** zeroes transitions/animations *and* the `::view-transition-*`
  pseudo-elements (the `*` reset can't reach them) — the existing guard, extended.

### D. Implementation plan (staged, each slice ships a11y-green)
W1 **token/palette foundation** → W2 **elevation + surface + component restyle** → W3 **motion &
micro-interaction system** → W4 **hero-surface polish (Chat + Telemetry)**. Mirrors the v9
foundation→IA→dashboard→motion arc; each child is born in its PR, re-ranked on each merge, and must
keep the both-theme axe scan green. **Risk/trade-offs:** nothing here needs a build chain; the only
genuine gaps are (a) scroll-driven animations (no Firefox → progressive-enhancement only) and (b)
P3-gamut accents (wrap in `@media (color-gamut: p3)` with an sRGB fallback). A re-derived palette is
the highest-risk slice (could regress AA) — it lands first, behind the axe gate, fully reversible.

## Children

- [x] **W1 · Token & palette foundation** — [0063](0063-token-palette-foundation-oklch-layer-openprops.md)
      (M; ADR-0036). `depends_on: []`. Re-derive the palette in OKLCH (AA-safe lightness bands),
      introduce `@layer`, vendor Open Props (or a curated easings/animations subset), add
      `color-mix()` state tints + `@property` animatable tokens. The foundation every later child builds on.
      **Shipped** (ADR-0036; see the issue's close-out).
- [x] **W2 · Elevation, surface & component restyle** — [0064](0064-elevation-surface-component-restyle.md)
      (L; ADR-0038). `depends_on: [0063]`. Surface luminance
      ladder (dark) + hue-tinted layered shadows (light), radius scale, glass/radial-glow atmosphere,
      restyle keycap/badge/button/card/input + the type scale.
      **Shipped** (PR #105; ADR-0038; see the issue's resolution).
- [x] **W3 · Motion & micro-interaction system** — [0065](0065-motion-microinteraction-system.md)
      (M/L; ADR-0037). `depends_on: [0063]`. `linear()` spring easings + motion/duration tokens; the
      micro-interaction catalogue (`@starting-style`/`allow-discrete`/`popover`); per-nav + scoped
      view-transitions (NOT global); reduced-motion guard extended.
      **Shipped** — PR #106, ADR-0037 (see the issue's Resolution).
- [x] **W4 · Hero-surface polish — Chat + Telemetry** — [0066](0066-hero-surface-polish-chat-telemetry.md)
      (M). `depends_on: [0064, 0065]`. Apply the full system to the two hero surfaces (streaming chat,
      dashboard motion, palette delight) as the proof + showcase.
      **Shipped** — PR #107 (see the issue's Resolution). Its merge **closes this epic**.

Dependency graph:
```
0063 (W1) ─┬─► 0064 (W2) ─┐
           └─► 0065 (W3) ─┴─► 0066 (W4)
```
Unblocked now: **0064, 0065** (W1 shipped; the two are parallelizable — disjoint seams:
elevation/surface restyle vs. the motion system).

## Acceptance (epic)

- [x] A re-derived, "playful" OKLCH palette + three-tier token system ships, AA in **both** themes
      (axe both-theme scan green), organized with `@layer`.
- [x] Depth reads as designed: luminance ladder (dark) / hue-tinted layered shadows (light), restyled
      surfaces + components on the constrained radius/space scales.
- [x] A CSS-only **spring motion system** (tokens + micro-interaction catalogue) with delightful
      hover/press/focus/enter-exit/toast/palette/transition states — **all** degrading under
      `prefers-reduced-motion`, none breaking htmx OOB / SSE streaming.
- [x] The two hero surfaces (Chat + Telemetry) showcase the direction end-to-end.
- [x] Each child: born in its PR, ADR where it changes the token/motion contract, `make lint && make
      test` + `make e2e` green; the hard constraints hold (no build chain / framework / new JS dep —
      or it's a conscious ADR trade-off). SemVer **minor** on the epic.

**Closed** with W4 (issue 0066, PR #107): all four children shipped — 0063 (W1) / 0064 (W2,
PR #105, ADR-0038) / 0065 (W3, PR #106, ADR-0037) / 0066 (W4, PR #107) — every acceptance item
above holds, the hard constraints were never relaxed (no new ADR needed for W4: it consumed
0036/0037/0038 without contract changes), and the standing guards grew two new invariants along
the way (no undefined `var(--x)` references; reduced-motion zeroes delays). SemVer **minor**
bump to follow on the next release.

## Notes

This is a **presentation** epic (the second after v9), run from the deferred Open-Props/motion
paydown. Research recorded in NEXT_FEATURES "Roadmap v11" with the full source list. Reserved ADRs:
**0036** (token/palette + `@layer` + Open Props decision — does re-deriving the palette keep or drop
the terracotta/blue identity?), **0037** (the motion system — `linear()` springs, view-transition
policy). Carries REGRESSIONS #20 (light-theme contrast) forward as a standing a11y guard.

## Numbering

Highest on disk before this pass: issues → **0061**, epic → **0052**, ADRs → **0034** (0035 reserved
by 0061). This epic takes **0062**; children take **0063–0066**; it reserves **ADR-0036** and
**ADR-0037**.
