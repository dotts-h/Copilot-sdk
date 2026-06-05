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
  issues: [0008]
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
- [ ] **3.1 — Diff review lane** — a collapsible side-by-side/inline diff with the
      approve/reject attached, so file-writing agents feel trustworthy. Touches
      `render.go` (tool result rendering) + the permission form. Next per sequencing.
- [ ] **3.3 — Keybinding surface** — a help overlay + a Settings section over the
      existing `config.Config` key bindings.
- [ ] **3.4 — Prompt/snippet library** — saved, reusable prompts insertable from the
      composer via the slash-command autocomplete.

## Status

3.2 shipped (closes TECH_DEBT #2). 3.1 is next per the recommended sequencing
(… → 3.2 → 3.1 → 2.1); 3.3 / 3.4 are smaller follow-ons.

## Notes

Recommended sequencing (NEXT_FEATURES): 1.x → 2.2 → **3.2 → 3.1** → 2.1. Keep
domain logic pure (`telemetry`/`ctxforge`/`config` dependency-free); write the
failing test first; fold ADR/CONTRACTS/REGRESSIONS into the feature branch.
