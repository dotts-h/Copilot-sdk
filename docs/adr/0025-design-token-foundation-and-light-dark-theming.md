# 0025. Design-token foundation and light/dark theming via `light-dark()` + `color-scheme`

- Status: accepted
- Date: 2026-06-08
- Deciders: Horia
- Related: the **first child of the UI/UX refresh epic**
  ([0045](../issues/0045-epic-ui-ux-refresh.md), roadmap v9) — the foundation every later
  child (nav→sidebar, telemetry dashboard, command palette) builds on. Keeps the **no build
  chain / single committed CSS file** doctrine of the handcrafted stylesheet
  (`internal/web/static/app.css`, seeded to mirror the TUI palette) and the **minimal-JS,
  no-framework** posture (cf. the keymap dispatch of
  [ADR-0014](0014-keybinding-surface-config-backed-keymap-with-minimal-js-dispatch.md) and
  the composer scripts of [ADR-0015](0015-prompt-snippet-library-forge-backed-composer-insertion.md)).
  Clears the documented destructive-control contrast baseline tracked in
  `e2e/tests/a11y.spec.ts` (the `KNOWN_CONTRAST_SELECTORS` allowlist). Touches
  `internal/web/static/app.css`, `internal/web/templates/index.html` (the `<head>` theme
  script + the topbar toggle), `e2e/tests/a11y.spec.ts` / `e2e/tests/theme.spec.ts`,
  `docs/CONTEXT.md` (the **theme** / **design token** terms),
  [issue 0046](../issues/0046-design-token-foundation-light-dark-theme.md). **No server route,
  no persisted schema, no `copilot.Client` change** — CONTRACTS unchanged.

## Context

A v8 research pass (recorded under epic 0045) reviewed the web UI against modern front-end
practice. The stylesheet is **dark-only**: a flat `:root` block of raw color tokens, with the
near-black `#1b1d1e` and `#fff` hard-coded as on-button text and `rgba(255,255,255,…)` /
`rgba(0,0,0,…)` neutral overlays sprinkled through the rules. Two consequences follow. First,
there is **no light theme**, and the hard-coded literals actively block one: a
`rgba(255,255,255,.05)` "hover" lightens the wrong way on a light background, and white-on-red
buttons assume a dark canvas. Second, a **known WCAG AA contrast shortfall** on the destructive
controls (`.abort` / `.plan-reject` / `.elicit-no` / `.no` — red text/borders or white-on-red
at ~3.5–4.2:1) is carried as a documented allowlist in the a11y suite, pending a palette retune.

This child establishes the **token + theming foundation**: a semantic design-token layer that
expresses **both** a light and a dark palette, a persisted theme toggle, and a palette retune
that clears the contrast baseline — without adding a build step, a CSS framework, or a JS
framework. It is deliberately scoped to the foundation; the larger redesign work it unlocks
(navigation → sidebar, the telemetry dashboard) are **separate children** of epic 0045.

The decisions an ADR must settle: **how the two palettes are expressed**, **how a theme is
selected and persisted without a flash**, and **how the destructive-control contrast is fixed**
without a per-usage color explosion.

## Considered options

- **How the two palettes are expressed.**
  - **One token set whose values use the CSS `light-dark()` function, keyed on `color-scheme`
    (chosen).** `:root { color-scheme: light dark; --bg: light-dark(#faf9f7, #1b1d1e); … }`.
    Each token is written **once** with its light and dark value; the browser resolves it from
    the active `color-scheme`. `light-dark()` is Baseline since 2024-05 (all evergreen
    engines), and the app is a **localhost tool the user runs in a current browser**, so the
    support floor is met; Playwright's Chromium (the a11y gate) supports it fully. This halves
    token maintenance versus duplicated blocks and natively themes form controls / scrollbars
    via `color-scheme`.
  - *Duplicated `[data-theme="dark"]` / `[data-theme="light"]` token blocks.* Rejected as the
    primary mechanism — every token written twice, the classic drift risk this repo's
    "one fact, one home" doctrine exists to prevent. It is the **fallback** approach for
    pre-2024 browsers, which this tool does not target.
  - *A CSS framework / utility pipeline (Tailwind standalone binary, Open Props, Pico).* The
    research found Tailwind viable **without** Node (the v4 standalone binary, vendorable +
    Makefile-driven) but a poor fit for hand-written Go `html/template` + htmx partials
    (utility-class verbosity in templates with no component-extraction escape hatch); the
    Play CDN / UnoCSS runtime are dev-only (in-browser JIT). **Open Props** (a pure
    custom-property token file, no build) is the one additive option worth keeping on the
    table, but adopting it now means re-pointing every token at its primitives — a large,
    low-urgency change. **Deferred, not rejected:** it can be layered under our semantic tokens
    later with zero markup change. This child keeps the **single committed CSS file**.

- **How a theme is selected and persisted (and the flash avoided).**
  - **A tiny synchronous inline `<script>` in `<head>` sets `<html data-theme>` before first
    paint, from `localStorage` (falling back to the OS preference); a topbar toggle flips it
    and persists (chosen).** Running the read in the head, synchronously, before the body
    paints is the standard fix for the flash-of-wrong-theme (FOUC) — any later hook (external
    script, `DOMContentLoaded`) is too late and flickers. The CSS maps the attribute to the
    `color-scheme` property — `html[data-theme="light"] { color-scheme: light }` /
    `dark` — so the toggle changes **one property** and every `light-dark()` token re-resolves.
    With **no** stored choice the attribute is absent and `color-scheme: light dark` lets the
    **OS preference** win. This is consistent with the app's existing minimal scripts (keymap
    dispatch, composer autosize) — **no framework, one small function, client-only**.
  - *Server-side theme (a cookie + a Go-rendered attribute / a `POST /theme` route).* Rejected
    — it adds a route and a CONTRACTS entry for a purely presentational, per-browser preference
    that `localStorage` owns natively, and it still needs the inline read to avoid the flash.
    Keeping it client-only means **zero** server/seam/schema surface.

- **How the destructive-control contrast is fixed (without a per-usage color explosion).**
  - **Retune the semantic colors so each flips bright (dark theme) / deep (light theme), and
    introduce one paired `--on-bright` text token for solid fills (chosen).** Every semantic
    color (`--accent`, `--accent2`, `--good`, `--warn`, `--bad`) is light in the dark theme and
    deep in the light theme, so each clears 4.5:1 **as text on the page background in both
    themes** (the dark `--bad` is brightened from `#f85149` to `#ff7b72`, lifting `.abort`'s red
    text/border above the floor; the light values are tuned likewise — verified per-pair). Any
    **solid fill** of those colors then takes a **single** companion text token,
    `--on-bright: light-dark(#fff, #1b1d1e)`: because all five colors flip lightness the **same**
    direction, one on-color contrasts on every fill in both themes — so the hard-coded
    `#1b1d1e` / `#fff` button texts collapse to `var(--on-bright)`, and the white-on-red
    shortfall disappears (dark theme renders dark text on the light-red fill; light theme renders
    white text on the deep-red fill — both ≥ 4.5:1). The neutral overlays become theme-aware
    tokens (`--hover` / `--raised` / `--code-bg` / `--sunken`) that flip direction so
    "raised/sunken/hover" reads correctly on either canvas.
  - *A second, dedicated `--bad-solid` (dark red) fill token + per-color on-colors.* Rejected as
    redundant — once `--bad` itself flips lightness per theme, `--on-bright` already contrasts on
    it in both, so a separate solid red and four separate on-colors are surface no rule needs.

## Decision

Replace the flat `:root` block with a **semantic token layer** that sets `color-scheme: light
dark` and expresses every color via `light-dark(lightValue, darkValue)`, adding the companion
`--on-bright` (text on any solid accent/good/warn/bad fill) and theme-aware neutral overlays
(`--hover`, `--raised`, `--raised-2`, `--code-bg`, `--sunken`). Add the attribute hooks
`html[data-theme="light"] { color-scheme: light }` and `…="dark" { color-scheme: dark }` so a
toggle re-resolves all tokens by flipping one property. Replace the hard-coded on-button
`#1b1d1e` / `#fff` with `var(--on-bright)` and the neutral `rgba(…)` overlays with the new
tokens. Retune the palette so all text clears WCAG AA in **both** themes (the dark `--bad`
brightened to `#ff7b72`; light values tuned per-pair). Add a global `:focus-visible` ring and a
`prefers-reduced-motion` reset. In the shell (`index.html`): a synchronous `<head>` script sets
`data-theme` from `localStorage` (OS-preference fallback) before paint, and a topbar **theme
toggle** button (outside `<nav>`, with an `aria-label`) flips and persists it. The a11y suite
scans **each page in both themes** and the `KNOWN_CONTRAST_SELECTORS` allowlist + its baseline
test are **removed**. No build step, no CSS/JS framework, no server route, no schema change.

## Consequences

- Positive: the app ships a **light and a dark theme** from one token set, following the OS by
  default and togglable + persisted per browser, with **no build chain and no framework** — the
  handcrafted-single-CSS-file doctrine holds. The dark theme is visually unchanged but for the
  brightened destructive red; the light theme is new.
- A11y (the baseline cleared): all text clears WCAG AA **in both palettes** (verified
  per-pair), so the `KNOWN_CONTRAST_SELECTORS` allowlist and its `[baseline]` guard test are
  deleted; the a11y scan now runs over **both** themes, and any contrast regression in either
  fails the suite. A global `:focus-visible` ring and a `prefers-reduced-motion` reset raise the
  keyboard / motion baseline.
- One-fact-one-home: each color value lives **once** (its `light-dark()` token); a solid fill's
  text is **always** `--on-bright`; neutral elevation is **always** a `--hover`/`--raised`/
  `--sunken` token — no more per-rule literals to drift. New UI must use the tokens, never raw
  hex / `rgba(255,255,255,…)`.
- Scope held: this is the **foundation only**. The navigation regroup (top-bar → grouped
  sidebar + ⌘K palette) and the telemetry dashboard (KPI cards + server-rendered inline-SVG
  sparklines) are **separate children** of epic 0045, each taking its own ADR. **Open Props**
  and CSS **`@layer`** are recorded as **deferred, additive** enhancements (they need no markup
  change and can layer under these tokens later) — not adopted here to keep the diff bounded and
  the cascade unchanged.
- Browser floor (the conscious trade-off): `light-dark()` is Baseline-2024; pre-2024 engines
  fall back to the dark values (the `light-dark()` line is dropped as invalid) until the optional
  duplicated-block fallback is added. Acceptable for a localhost tool on a current browser; the
  Playwright/Chromium gate supports it fully.
- Contract surface: **none.** No new route, no `copilot.Client` change, no persisted schema — the
  theme is a client-only `localStorage` preference. CONTRACTS is unchanged; CODEMAP is unchanged
  (no new Go top-level declaration).
