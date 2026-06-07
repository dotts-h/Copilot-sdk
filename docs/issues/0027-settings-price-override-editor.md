---
id: 0027
title: Settings price-override editor (roadmap v4, item G1/V2)
status: closed
severity: medium
group: 0024
github:
links:
  adr:
  prs: []
  issues: [0024, 0026]
  regression: "REGRESSIONS.md — 'A live price-override reprice must REBUILD from defaults and mutate the SHARED price book in place'"
assets: []
---

## Summary

The cost surface still has one **hand-edit-JSON-only** knob.
`config.TelemetryConfig.PriceOverrides` (model → `[input, cached, output]` USD per million
tokens) is loaded at startup and applied over `DefaultPriceBook` (`internal/bootstrap`),
but it is the **only** cost setting with no UI — `settings.go` `renderSettingsForm`
surfaces every other cost knob (allowance, warn fraction, hard cap) but omits the
per-model rate table by design. G1 adds a per-model rate editor on the Settings page,
persisted through `editConfig` (snapshot → apply → validating Save → rollback-on-invalid)
**and** applied live to the in-process price book so the next turn's estimate/meter prices
at the new rate without a restart. Source: `docs/NEXT_FEATURES.md` item G1/V2.

## Repro
1. Open the Settings page.
   - **Expected:** a per-model price-override table — one row per model, three numeric
     fields each (input · cached · output per-MTok), pre-filled from
     `config.Telemetry.PriceOverrides` (the built-in default as placeholder). Saving an
     override persists it and reprices the live meter immediately.
   - **Actual (before G1):** the Settings form shows allowance / warn / hard cap / OTLP /
     token-env / keybindings, but **no** per-model rate table — overriding a model's price
     required editing `config.json` by hand and restarting.

## Proposed resolution

- **`internal/config` (pure):** a `Validate` hook rejecting any negative rate in
  `PriceOverrides` (each `[3]float64` entry `>= 0`), beside the allowance/warn/cap checks.
  Additive; older configs (no `priceOverrides`) stay valid. Unit-tested.
- **`internal/telemetry` (pure seam):** `BuildPriceBook(overrides) *PriceBook` (fresh
  `DefaultPriceBook` + overrides — rebuild-not-incremental) and `(*PriceBook).Replace(src)`
  (atomic in-place swap of a shared book's contents under the book's own RWMutex). The
  minimal honest seam so the web layer rebuilds-and-applies without reaching into
  `PriceBook` internals; keeps `telemetry` dependency-free. Unit-tested.
- **`internal/web` (Settings view):** `renderSettingsForm` gains the per-model table
  (rows from `DefaultPriceBook().Models()` ∪ existing overrides, index-keyed fields so a
  dotted model id can't collide with the delimiter); `handleSettingsSave` parses the rows
  (an all-blank/zero row = no override, so it falls back to the default — never a `$0`
  price), persists through `editConfig`, and applies **live**: rebuild via
  `telemetry.BuildPriceBook` then `Replace` the shared book in place so the account meter
  AND every per-session meter reprice. Preserve-on-absent (a partial POST that omits the
  section leaves stored overrides untouched, like keybindings). Values through
  `html/template` (ADR-0001).
- **Tests:** unit (config — negative rejected, non-negative & absent accepted; telemetry —
  the rebuild prices an overridden model at the new rate and a non-overridden one at the
  default, a removed override reverts to the default, a shared `Replace` reprices both a
  account and a session meter). web — the form renders the rows pre-filled, a save
  round-trips the override into config AND reprices the live meter (both meters — the
  no-drift guarantee), a negative rate is rejected and rolled back, a blank/zero row
  persists no override. e2e — assert the section STRUCTURE (subhead, a row, three inputs),
  drive a save, never assert exact figures (reset the mutated config in `afterEach`).

## Resolution (shipped)

Built as specified, no schema change to the persisted shape (the `priceOverrides` field
already existed). `internal/config` (`config.go`): `Validate` now rejects any negative
`PriceOverrides` rate (each `[3]float64` entry `>= 0`), beside the allowance/warn/cap
checks; absent overrides stay valid (backward-compatible). `internal/telemetry`
(`pricing.go`): `BuildPriceBook(overrides)` builds a fresh `DefaultPriceBook` + overrides
(rebuild-not-incremental), and `(*PriceBook).Replace(src)` atomically swaps a shared
book's contents in place under a new internal RWMutex (so `Rate`/`Set`/`Models` are now
lock-guarded and a live reprice is race-safe against concurrent turn-pricing reads);
`internal/bootstrap` now uses `BuildPriceBook` too, single-sourcing the startup and live
builds. `internal/web` (`settings.go`, `forms.go`, `fragments.html`): the Settings form
gains the per-model price-override table (index-keyed `price.<i>.{model,in,cached,out}`
fields, pre-filled, default-as-placeholder); `handleSettingsSave` parses the rows
(blank/zero row → no override; negative → rejected), persists through `editConfig`, and
applies live by rebuilding + `Replace`-ing the shared book — repricing the account meter
**and** every per-session meter at once (the same `*PriceBook` by reference), so the
statusline (sessionMeter) and the gate (account meter) can't drift. Tests: unit
(`internal/config` `TestValidateRejectsBadValues` extended + `TestValidateAcceptsPriceOverrides`;
`internal/telemetry` `TestBuildPriceBookAppliesOverridesOverDefaults`,
`TestBuildPriceBookRemovedOverrideRevertsToDefault`, `TestPriceBookReplaceRepricesSharedMeters`),
web (`TestSettingsFormRendersPriceOverrideRows`,
`TestSettingsSaveRoundTripsPriceOverrideAndRepricesLive`,
`TestSettingsSaveBlankPriceRowPersistsNoOverride`, `TestSettingsSaveNegativeRateRollsBack`),
e2e (the Settings spec asserts the price-override section structure + a save, resetting the
override in `afterEach`). Docs: CONTRACTS §3 (the Settings price-override surface + live
reprice) + §4 (the `PriceOverrides` non-negative validation + the
`BuildPriceBook`/`Replace` seam); a REGRESSIONS testing-note records the
rebuild-not-incremental + shared-book-in-place reprice gotcha (each naming its guard test).
No bug shipped — the gotchas were guarded preemptively. Shipped on branch
`claude/price-override-editor`.

## Notes
- **No ADR:** additive UI over an existing config field (`PriceOverrides`), with one
  non-obvious choice — the live-apply seam — that proved an obvious mirror of the startup
  price-book build (`internal/bootstrap`) plus the established `refreshBudget`
  live-cache-refresh pattern. Per ADR-0004 an ADR leads only a *non-obvious* decision; the
  rebuild-not-incremental + shared-book-in-place choice is captured in CONTRACTS + a
  REGRESSIONS note instead.
- **Differentiator:** completes the cost surface — the last hand-edit-JSON cost knob now
  has a UI, and a rate tweak takes effect without a restart.
- **Numbering:** issue **0027** (next free after 0026), third build of epic **0024**
  (roadmap v4). No ADR consumed.
