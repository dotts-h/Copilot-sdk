---
id: 0066
title: "Hero-surface polish — apply the full system to Chat + Telemetry as the proof/showcase (roadmap v11, W4)"
status: open
severity: low
group: 0062
depends_on: [0064, 0065]
github:
links:
  adr: []
  prs: []
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
