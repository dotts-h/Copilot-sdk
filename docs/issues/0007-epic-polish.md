---
id: 0007
title: "Epic: polish that compounds (Tier 3)"
status: open
severity: medium
group:
github:
links:
  adr:
  prs: []
  issues: [0008, 0009, 0011]
  regression:
assets: []
---

## Charter

Tier 3 is the visible polish that compounds on the cost-aware / orchestration
core: per-conversation accounting, a dedicated diff review lane, a keybinding
surface, and a prompt/snippet library. Each reuses existing primitives. Source:
`docs/NEXT_FEATURES.md` Tier 3.

## Tasks

- [x] **3.2 — Per-session telemetry totals** → [0008](0008-per-session-telemetry-totals.md)
      (ADR-0011, TECH_DEBT #2). A per-session `telemetry.Meter` scopes the
      statusline to *this* conversation; the budget gauge / hard-cap / Telemetry
      rows stay account-wide.
- [x] **3.1 — Diff review lane** → [0009](0009-diff-review-lane.md) (ADR-0012). A
      file-write permission renders a collapsible, side-numbered **inline** unified
      diff (diffstat + intention) with approve/reject on the existing `/perm` flow;
      parsed server-side by a pure `parseUnifiedDiff` and HTML-escaped.
- [x] **3.3 — Keybinding surface** → [0011](0011-keybinding-surface.md) (ADR-0014).
      A config-backed keymap (fixed action set + persisted overrides, pure-validated)
      surfaced in a help overlay + the Help page + a Settings section, dispatched by
      a small vanilla-JS `keydown` handler reading `<body data-keymap>`.
- [ ] **3.4 — Prompt/snippet library** — saved, reusable prompts insertable from the
      composer via the slash-command autocomplete.

## Status

3.2 (ADR-0011, closes TECH_DEBT #2), 3.1 (ADR-0012, diff review lane), and 3.3
(ADR-0014, keybinding surface) have shipped; 2.1 (multi-agent run / handoff)
shipped under Tier-2 epic 0005 (ADR-0013). The one remaining Tier-3 follow-on
under this epic is **3.4 (prompt/snippet library)**.

## Notes

Recommended sequencing (NEXT_FEATURES): 1.x → 2.2 → **3.2 → 3.1** → 2.1. Keep
domain logic pure (`telemetry`/`ctxforge`/`config` dependency-free); write the
failing test first; fold ADR/CONTRACTS/REGRESSIONS into the feature branch.
