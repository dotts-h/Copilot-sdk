---
id: 0066
title: "Hero-surface polish — apply the full system to Chat + Telemetry as the proof/showcase (roadmap v11, W4)"
status: closed
severity: low
group: 0062
depends_on: [0064, 0065]
github:
links:
  adr: []
  prs: [107]
  issues: [0062]
  regression: [20]
assets: []
---

## Summary
The capstone: apply the re-derived palette (0063), surface/elevation restyle (0064), and motion
system (0065) end-to-end to the two **hero surfaces** that define the product — **Chat** (the live
streaming transcript) and **Telemetry** (the KPI dashboard) — so the direction is proven and shown
off where it matters most, not just in the chrome.

## Scope / Touches
- `internal/web` (`render.go`/`telemetry_render.go`/templates) — Chat: refined streaming-token feel,
  tool-card + reasoning-block treatment, the cost statusline/footer, inline permission/diff polish;
  Telemetry: KPI-card depth + number motion, sparkline/band/bullet polish, per-model table, the
  `?window=` swap. No data/route/schema changes — presentation only.
- Tiny vanilla JS only where unavoidable; reuse the 0065 catalogue + tokens.

## Dependencies
- `depends_on: [0064, 0065]` — needs both the restyle and the motion system to apply them together.
  Last child — its merge **closes epic 0062**.

## Acceptance
- Chat + Telemetry visibly embody the playful-polished direction (color, depth, motion) with no
  regression to streaming/OOB behavior; **axe both-theme scan green**; motion degrades under
  `prefers-reduced-motion`.
- Failing/guard test where unit-testable; `make lint && make test` + `make e2e` green; born in its PR.
  `/verify` or `make run` to exercise the live streaming + dashboard feel before push.

## Notes
Showcase slice — the before/after the overhaul is judged on. See [epic 0062](0062-epic-playful-polished-ui-motion-overhaul.md).

## Resolution (shipped)

**PR #107** (`feat/0066-hero-surface-polish`). The capstone consumed all three contracts
(ADR-0036/0037/0038 — no contract change, so no new ADR) on the two hero surfaces,
presentation-only:

- **Chat:** streamed tokens fade in via the append-once `#cur > span` child combinator under a
  blinking live caret (`content … / ""` keeps it out of the a11y tree); tool cards rise onto the
  elevation system (`--panel` ladder / `--shadow-1`) with a mono tool identity; a pending tool
  result consumes the staged `.skeleton`; action cards rest on `--shadow-1`; the composer bar gets
  the `--border-glass` hairline + an eased focus halo; the statusline joins the mono register;
  `.chat` consumes `.atmosphere`.
- **Telemetry:** the hero consumes `.atmosphere`; KPI cards = ladder step + `.glass` + a one-shot
  staggered entry (re-runs on the `?window=` swap — click-driven, never SSE); sparklines gain a
  soft area fill + a `pathLength="1"` draw-in; tables get caps headers/tabular numerals/row hover.
- **W3 bug fixed:** the hover-lift read the nonexistent `var(--shadow-lg)` — IACVT silently
  *removed* the resting shadow on hover. Now `--shadow-1 → --shadow-2`, with a new contract guard
  `TestNoUndefinedTokenReferences` (every `var(--x)` consumed must be declared) killing the class.
- **Guards extended:** `TestHeroSurfacesConsumeStagedSystem` (atmosphere/glass/skeleton wiring +
  the child-combinator invariant); the reduced-motion reset now zeroes animation/transition
  *delays* (pinned); skeleton/sparkline markup pinned in the render/svg tests.
- Gates: `make lint && make test` green (web 90.3% coverage), e2e 141 passed (3 known CPU-race
  flakes self-healed), both-theme axe green; live feel verified via `-demo` + Playwright
  screenshots (both themes, streaming, window swap, reduced motion, hover-deepens).

Its merge closes **epic 0062** (children 0063–0066 / PRs #101/#105/#106/#107).
