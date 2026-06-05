---
id: 0011
title: Keybinding surface (Tier 3, item 3.3)
status: closed
severity: low
group: 0007
github:
links:
  adr: ../adr/0014-keybinding-surface-config-backed-keymap-with-minimal-js-dispatch.md
  prs: []
  issues: [0007]
  regression:
assets: []
---

## Summary

ARCHITECTURE/README advertised that `config.Config` "carries key bindings", but
it didn't — and the web UI had no keyboard shortcuts at all (only browser-default
autofocus + Enter-to-submit). Add a real keybinding surface: a help overlay
listing the shortcuts and a Settings section that reads/writes the schema. To
keep the surface truthful, the shortcuts must actually fire. Source:
`docs/NEXT_FEATURES.md` item 3.3.

## Repro
1. Open the app; press any key expecting a shortcut (help, new chat, …).
2. Expected: a documented, customisable set of shortcuts + a discoverable overlay.
3. Actual (before): nothing — no schema, no overlay, no dispatch.

## Resolution

- **Schema (pure, `internal/config`):** a fixed ordered action set
  (`config.KeyActions()` — `{id, label, default}`) with persisted **overrides**
  (`Config.KeyBindings map[string]string`, `omitempty`, keyed by action id).
  `Config.Keymap()` resolves the effective key (override-or-default);
  `validateKeyBindings` enforces a known id, a single-character key, and no
  duplicate key across actions. Round-trips backward-readable (older files have
  no `keyBindings`).
- **Surface (`internal/web`):** a body-level **help overlay** (`#help-overlay`,
  toggled by its bound key, closed by Esc, survives htmx swaps), a **Help page**
  shortcut table, and a **Keyboard shortcuts** section in the Settings form (one
  single-key field per action; blank reverts to default). Editing flows through
  `editConfig` (rollback-on-invalid).
- **Dispatch:** `handleIndex` renders the action→key map onto `<body data-keymap>`
  (JSON, auto-escaped) and a ~40-line vanilla-JS `keydown` handler dispatches it —
  ignoring keystrokes typed into fields and modified keys, routing each action to
  an existing affordance (nav-link click, focus composer, `.abort` click,
  `htmx.ajax` new session, overlay toggle). Esc-closes-overlay is fixed, not a
  binding. All keys/labels HTML-escaped (ADR-0001). Design recorded in **ADR-0014**
  (config-backed keymap; minimal JS vs a library; pure validation/escaping).

## Notes

Guarding tests: `internal/config` `TestKeymapAppliesDefaultsAndOverrides`,
`TestValidateRejectsBadKeyBindings`, `TestKeyBindingNormalizeDropsEmptyAndRevertsToDefault`,
`TestKeyBindingsRoundTrip`, `TestEmptyKeyBindingsOmittedFromDisk`; `internal/web`
`TestIndexRendersKeymapAndOverlay`, `TestHelpPageListsShortcuts`,
`TestSettingsFormHasKeybindingFields`, `TestSettingsSaveAppliesKeybindingOverride`,
`TestSettingsSaveRejectsDuplicateKey`; browser `e2e/tests/keybindings.spec.ts`
(overlay toggle, single-key nav, don't-hijack-typing, Settings/Help listings).
Contract change (additive `config.KeyBindings`) recorded in CONTRACTS; gotchas
(field-focus guard, apply-on-reload) in REGRESSIONS; apply-on-reload as
TECH_DEBT #13. Closes the keybinding item of epic 0007; remaining Tier-3 follow-on
is 3.4 (prompt/snippet library).
