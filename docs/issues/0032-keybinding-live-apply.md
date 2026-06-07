---
id: 0032
title: Keybinding live-apply — rebind takes effect without a full page reload (roadmap v5, item V10)
status: closed
severity: low
group: 0030
github:
links:
  adr: adr/0014-keybinding-surface-config-backed-keymap-with-minimal-js-dispatch.md
  prs: [57]
  issues: [0030]
  regression: REGRESSIONS #18
assets: []
---

## Summary

A keyboard-shortcut **rebind on the Settings page took effect only on the next full
page load** (TECH_DEBT #13). The keymap is config-backed and dispatched by a minimal
vanilla-JS handler reading `<body data-keymap>` (ADR-0014 / issue 0011), but the
Settings `POST /settings` that persisted a rebind re-rendered only the `#main`
settings partial — it never refreshed the live `<body data-keymap>` or the help
overlay (both rendered by the index shell), so the new binding was dead until a
reload. **V10 closes the gap:** on a successful keybinding POST, the response appends
an `hx-swap-oob` re-render of the help overlay **and** an `applyKeymap(…)` script that
updates `<body data-keymap>` and rebuilds the dispatcher's reverse map, so the rebind
applies **live**. A web-layer + config-read change; no schema change. Source:
`docs/NEXT_FEATURES.md` item V10. **Second child of epic 0030 (roadmap v5).**

## Repro
1. Open the app, navigate to **Settings**, and rebind a shortcut (e.g. change "Open
   the Telemetry page" from `t` to `y`); click **Save**.
2. Without reloading the page, press the new key outside a text field.
   - **Expected:** the new binding fires immediately (`y` opens Telemetry).
   - **Actual (before V10):** nothing happens — the JS dispatcher still reads the
     old keymap; the rebind works only after a full page reload.

## Proposed resolution

- **`internal/web` (Settings keybinding POST):** on a successful rebind, return —
  alongside the persisted settings partial — an `hx-swap-oob` fragment that
  re-renders the `#help-overlay` (which lists the bindings) **and** a
  `<script>applyKeymap(…)</script>` that updates `<body data-keymap>` so the live JS
  dispatcher picks up the new binding without a reload.
- **Config read only** — the keymap is already persisted (`Config.Keymap()`); no
  schema change. A shared `keymapJSON` helper serializes the keymap so the initial
  index render and the live-apply swap share one source.
- **No new store, no telemetry/ctxforge dependency** beyond the existing keymap read.
- **Invariant:** binding text is config/user-originated, so every rendered binding
  goes through `html/template`/`esc` (the overlay) and `encoding/json` (the script
  JSON), never `trusted()` raw (ADR-0001). A POST that doesn't change bindings (or an
  invalid rebind rolled back) must not desync the live keymap from the persisted one.

## Resolution (shipped)

Built as specified — a web-layer + config-read change, no schema/store/telemetry
touch. **`internal/web`:**

- `handleSettingsSave` (`settings.go`): on a successful save that carried the
  keyboard-shortcut section (`formHasKeyBindings`), it appends `keymapLiveApply(...)`
  to the response. The keymap is read back from the **now-persisted** config
  (`s.config.Keymap()`), so the live attribute can never desync from disk — a no-op
  save re-emits the identical keymap, and a rolled-back-invalid save hits the error
  branch (which emits **nothing**).
- `keymapLiveApply` / `keymapJSON` / `helpOverlayAttr` (`pages.go`): the live-apply
  payload is an `hx-swap-oob="true"` re-render of `#help-overlay` (matched by its
  body-level id) plus `<script>applyKeymap(<json>)</script>`. `keymapJSON` is the one
  serializer shared with `handleIndex`'s `<body data-keymap>` render, so the initial
  attribute and the live swap carry one source. The JSON is HTML-safe in the script
  context — `encoding/json` unicode-escapes `<`, `>`, `&` (so no `</script>` can form
  from a bound metacharacter) and every key is a validated single character.
- `index.html`: the dispatcher's load-time keymap parse was refactored into an
  `applyKeymap(km)` bridge that sets `document.body.dataset.keymap` **and** rebuilds
  the `key→action` reverse map the keydown listener reads — from one source. Load-time
  init routes through the same `applyKeymap`, so the attribute and the dispatcher's
  cached map can never drift. (The gotcha: the dispatcher cached the reverse map at
  load, so OOB-swapping the attribute alone would have left the rebind inert —
  REGRESSIONS #18.)

Tests (failing-first): `internal/web` `TestSettingsSaveLiveAppliesKeymapViaOOB` (the
POST response includes the `hx-swap-oob` overlay re-render + the `applyKeymap` script
carrying the NEW binding + the overlay's `<kbd>` reflecting it),
`TestSettingsSaveLiveApplyMatchesPersistedKeymap` (a no-op save re-emits the persisted
keymap; a rolled-back invalid save emits no live-apply payload — no desync),
`TestSettingsSaveLiveApplyEscapesBinding` (a single `<` / `&` rebind renders
`&lt;`/`&amp;` in the overlay and `<`/`&` in the script JSON, never raw).
The config-level rebind round-trip/validation is already guarded by `internal/config`
`TestKeyBindingsRoundTrip` / `TestValidateRejectsBadKeyBindings` /
`TestKeyBindingNormalizeDropsEmptyAndRevertsToDefault`. e2e
(`e2e/tests/keybindings.spec.ts`): a new "a rebind takes effect live, without a full
page reload" spec asserts the `<body data-keymap>` **structure** reflects the change
without a reload and that the new key dispatches an action live (reverting the
mutation in `afterEach` per the shared-config gotcha); the existing
single-key-dispatch assertions are kept. Gates green (`make lint && make test`, web
coverage 88.9%); the e2e Chromium browser is blocked by the env's network allowlist,
so the spec was verified to compile/discover via `npx playwright test --list` and CI
runs the real Playwright suite.

Docs: CONTRACTS (the Settings keybinding POST now live-applies via an OOB
`#help-overlay` + `<body data-keymap>` swap, escaped per ADR-0001); TECH_DEBT #13 →
resolved (named guarding test); REGRESSIONS #18 (the cached-dispatcher desync gotcha);
ADR-0014 addendum (V10 **completes** the keymap mechanism — no new ADR). Shipped on
branch `claude/keybinding-live-apply`.

## Notes
- **No ADR:** this completes the ADR-0014 keymap mechanism (live-apply on the
  existing Settings POST) rather than changing its contract; captured as an addendum
  in ADR-0014 + CONTRACTS, mirroring the price-override / `refreshBudget` live-apply
  seams (no new route, no new store).
- **Differentiator:** polish — the keybinding surface now behaves like the rest of
  the live-applied Settings knobs (budget, price overrides), closing the last
  "applies on your next session" rough edge in the keymap.
- **Numbering:** issue **0032** (next free after 0031), second build of epic **0030**
  (roadmap v5). No ADR/epic consumed (epic 0030 stays open — H1 remains its last
  unbuilt child).
</content>
</invoke>
