# Binding: frontend

> An adapter doc, not a recipe. Copy it to `docs/bindings/frontend.md` in the
> target repo (the adopt-recipes skill does this) and fill the backfill slots.
> It binds the open slots Layer 0/1 leave for a frontend repo: npm-flavored
> gates, design tokens, accessibility, and visual regression. Rules that earn a
> repo-specific exception move into the repo's own CONVENTIONS with a reason.

## Gates (fills the quality recipe's npm slots)

- `build_system=npm`; `lint_command` and `test_command` are npm scripts
  (`npm run lint`, `npm test`) so CI and local gates stay identical.
- The coverage floor applies to unit/component tests; visual and a11y suites are
  *additional* gates, not coverage donors.
- Type checking (if the repo is typed) is part of `lint`, not a separate
  optional step — a type error is a lint failure.

## Design tokens

- **All colors, spacing, radii, and typography come from tokens** (CSS custom
  properties or the design system's equivalent). A raw hex value or px literal
  in component styles is a review flag.
- **Dim text with a dedicated dim token, never `opacity`.** Opacity dims the
  foreground *and* its contrast against the surface, dropping text below WCAG AA
  on tinted fills — this trap recurs. Use a contrast-tuned color token at full
  opacity.
- Both themes (light/dark, if present) are first-class: every token has a value
  in each theme, and theme-specific overrides live with the token definitions,
  not scattered in components.

## Accessibility

- **Axe (or equivalent) scans run in CI** on every page/surface the suite can
  reach, in both themes.
- **Any surface hidden until opened** (dialog, overlay, menu) gets its own
  open-then-scan test the day it lands — the static-page scan can't reach it.
- Interactive elements are reachable and operable by keyboard; focus is visible
  and managed across open/close transitions.
- Contrast meets WCAG AA; the token system (above) is where that's enforced once.

## Visual regression

- Screenshot/visual-regression coverage for the component states that matter
  (default, hover/focus, error, loading, both themes). Update snapshots only in
  a diff a human approved — a snapshot update is a *decision*, not noise.
- Flaky visual tests are quarantined with an issue filed (issues recipe), never
  deleted silently.

## Contracts (fills the contracts recipe's sections)

- The component library's public API (exported components + props) is a
  **Provides** entry when other repos consume it.
- The design-token names themselves are a contract: renaming a token is a
  coordinated change, recorded in `docs/CONTRACTS.md`.

## Backfill checklist (flagged by adopt-recipes)

- [ ] Wire the coverage floor into the npm test runner's threshold config.
- [ ] Add the axe scan job to CI (or record where it already runs).
- [ ] Point this doc's token rules at the repo's actual token source file.
