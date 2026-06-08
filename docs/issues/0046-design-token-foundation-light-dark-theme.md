---
id: 0046
title: "Design-token foundation + light/dark theme — the UI/UX refresh foundation (roadmap v9, item V21)"
status: open
severity: medium
group: 0045
github:
links:
  adr: [0025]
  prs:
  issues: [0045]
  regression:
---

## Summary

The handcrafted stylesheet (`internal/web/static/app.css`, seeded to mirror the TUI palette) is
**dark-only**: a flat `:root` block of raw color tokens, with `#1b1d1e` / `#fff` hard-coded as
on-button text and `rgba(255,255,255,…)` / `rgba(0,0,0,…)` neutral overlays sprinkled through the
rules. There is **no light theme**, and those literals actively block one (a
`rgba(255,255,255,.05)` "hover" lightens the wrong way on a light canvas; white-on-red buttons
assume a dark one). Separately, a **known WCAG AA contrast shortfall** on the destructive
controls (`.abort` / `.plan-reject` / `.elicit-no` / `.no`, ~3.5–4.2:1) is carried as the
`KNOWN_CONTRAST_SELECTORS` allowlist in `e2e/tests/a11y.spec.ts`, pending a palette retune.

**V21 lays the token + theming foundation** the rest of epic
[0045](0045-epic-ui-ux-refresh.md) (roadmap v9 — UI/UX refresh) builds on: a semantic
design-token layer expressing **both** a light and a dark palette, an OS-default + persisted
theme toggle with no flash, and a palette retune that clears the contrast baseline — **without a
build step, a CSS framework, or a JS framework**. It is the **first child** of epic 0045 and
takes **ADR-0025** for the decisions (how the palettes are expressed, how a theme is selected /
persisted without a FOUC, how the destructive-control contrast is fixed without a per-usage color
explosion).

## Repro
1. Open the app — it renders dark regardless of the OS appearance preference; there is no way to
   switch to a light theme.
2. Trigger a destructive control (the chat abort, or the demo's permission reject / plan reject).
   - **Expected:** its text/border clears WCAG AA (≥ 4.5:1).
   - **Actual (before V21):** ~3.5–4.2:1 — carried as the documented `KNOWN_CONTRAST_SELECTORS`
     allowlist so the a11y suite passes while the shortfall stands.

## Proposed resolution

- **`internal/web/static/app.css`:** replace the flat `:root` with a **semantic token layer**:
  `color-scheme: light dark` and every color via `light-dark(lightValue, darkValue)`; add
  `--on-bright: light-dark(#fff,#1b1d1e)` (text on any solid accent/good/warn/bad fill) and
  theme-aware neutral overlays (`--hover` / `--raised` / `--raised-2` / `--code-bg` / `--sunken`).
  Add `html[data-theme="light"]{color-scheme:light}` / `…="dark"{color-scheme:dark}` so the toggle
  re-resolves all tokens by flipping one property. Replace hard-coded on-button `#1b1d1e` / `#fff`
  with `var(--on-bright)` and the neutral `rgba(…)` overlays with the new tokens. **Retune the
  palette** so all text clears AA in **both** themes (dark `--bad` brightened `#f85149`→`#ff7b72`;
  light values tuned per-pair). Add a global `:focus-visible` ring + a `prefers-reduced-motion`
  reset.
- **`internal/web/templates/index.html`:** a synchronous `<head>` script sets `data-theme` from
  `localStorage` (OS-preference fallback via `matchMedia`, wrapped in try/catch) **before first
  paint** (the standard FOUC fix); a topbar **theme toggle** button (outside `<nav>`, with an
  `aria-label`) flips `data-theme` and persists it; a small `toggleTheme()` in the existing body
  script. **Client-only — no server route.**
- **ADR-0025** (written first, ADR-0004): `light-dark()` over duplicated blocks; client-only
  `localStorage` + inline-head script over a server cookie/route; one `--on-bright` over a
  per-color on-color explosion; Open Props / `@layer` deferred. CONTEXT gains the **theme** and
  **design token** terms. **No CONTRACTS / CODEMAP change** (no route, no Go declaration).

## Tests (failing-first)

- **e2e `a11y.spec.ts` (rewritten):** scan **each nav page in BOTH themes** (force
  `data-theme=light` / `dark`), asserting **zero** WCAG 2.1 A/AA violations — with **no**
  allowlist. The `KNOWN_CONTRAST_SELECTORS` allowlist, the `isBaseline` strip, and the
  `[baseline] destructive controls still have the known contrast shortfall` guard test are
  **deleted** (the palette is fixed, so the baseline no longer exists). Fails first against the
  old palette (the destructive controls violate in both themes); passes after the retune.
- **e2e `theme.spec.ts` (new):** the topbar toggle flips `html[data-theme]` and **persists**
  across a reload (`localStorage`); with no stored choice the attribute is absent and the page
  follows the emulated OS `colorScheme` (no flash — the `<head>` script sets the attribute before
  the body paints).
- **Contrast proof (pre-commit):** every text/background pair in both palettes computed against
  the WCAG formula (≥ 4.5:1), including `--on-bright` over each solid fill and colored text over
  its real tint — recorded in the PR. (Locks the retune before the browser gate runs.)

## Notes
- **ADR-0025:** the foundation child. Three decisions: `light-dark()` + `color-scheme` (one token
  set, both palettes) over duplicated `[data-theme]` blocks; an inline-`<head>` script +
  `localStorage` (client-only, no route, no FOUC) over a server-side cookie; and a single
  `--on-bright` companion text token (every semantic color flips lightness the same direction, so
  one on-color contrasts on every fill in both themes) over a `--bad-solid` + per-color
  on-color set. Open Props and CSS `@layer` are recorded **deferred-additive**.
- **Differentiator:** neither — a **presentation/foundation** child (epic 0045). It unlocks the
  light theme, removes a standing a11y debt, and gives every later child the token vocabulary +
  the both-theme a11y gate.
- **Scope held:** foundation only. Navigation→sidebar (V22) and the telemetry dashboard (V23) are
  **separate children**, each with its own ADR.
- **Numbering:** issue **0046** (next free after 0044; epic 0045 takes 0045), the first build of
  epic **0045**. **ADR-0025 consumed** (highest ADR becomes 0025).
