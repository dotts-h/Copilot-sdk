# 0014. Keybinding surface: a config-backed keymap with minimal JS dispatch

- Status: accepted
- Date: 2026-06-05
- Deciders: Horia
- Related: `internal/config` (`keybindings.go`: `KeyAction`/`KeyActions`/
  `ResolvedKey`/`Config.Keymap`/`validateKeyBindings`, `config.go` `KeyBindings`),
  `internal/web` (`pages.go` `renderShortcuts`/`helpOverlay`/`helpPartial`,
  `server.go` `handleIndex`, `settings.go`, `templates/index.html`,
  `static/app.css`), `docs/NEXT_FEATURES.md` item 3.3,
  [ADR-0001](0001-render-markdown-server-side-for-committed-agent-turns.md)

## Context

The docs (ARCHITECTURE, README) advertised that `config.Config` "carries key
bindings", but it did not: there was no keybinding schema and the web frontend
had no keyboard shortcuts at all (only the composer's autofocus and Enter-to-
submit, both browser defaults). Item 3.3 asks for a keybinding **surface**: a
help overlay listing the shortcuts and a Settings section that reads/writes the
schema. To make that surface truthful — an overlay must not advertise shortcuts
that do nothing — the shortcuts have to actually fire.

Three questions: **where the keymap lives**, **how a keystroke becomes an
action**, and **how the surface is validated/escaped**.

## Considered options

- **Where the keymap lives.**
  - **In `config.Config`, with the action set fixed in code (chosen).** The
    rebindable actions are a closed set the frontend dispatches on, so they live
    in code (`config.KeyActions()` — an ordered `[]KeyAction{id,label,default}`);
    only the *overrides* are persisted (`Config.KeyBindings map[string]string`,
    `omitempty`, keyed by action id). `Config.Keymap()` resolves the effective
    key per action (override-or-default). This keeps the action vocabulary from
    drifting out from under the JS, keeps the persisted surface minimal and
    backward-readable, and keeps validation pure and unit-testable.
  - **A free-form map of arbitrary key→command strings.** Rejected: it lets a
    file bind an action the frontend can't dispatch, and turns validation into
    parsing.

- **How a keystroke becomes an action.**
  - **A server-rendered keymap + a small vanilla-JS dispatcher (chosen).**
    `handleIndex` renders the resolved action→key map onto `<body data-keymap>`
    (JSON, auto-escaped in the attribute) and the body-level `#help-overlay`. A
    ~40-line `keydown` listener builds the reverse map and dispatches: it ignores
    keystrokes while a text field is focused (so typing in the composer is never
    hijacked) and while ctrl/meta/alt is held, and routes each action to an
    existing affordance (click the matching nav link, focus the composer, click
    `.abort`, `htmx.ajax` a new session, toggle the overlay). Esc always closes
    the overlay and is **not** rebindable.
  - **htmx/hyperscript or a JS keybinding library.** Rejected: htmx has no clean
    global-keydown-with-context-guard primitive, and a library reintroduces the
    vendored-JS/client-framework cost ADR-0001 and the REGRESSIONS dead-ends
    deliberately avoid. The dispatcher is small, dependency-free, and the
    *keymap* (the part worth testing) is computed and rendered server-side.

- **Validation & escaping.**
  - Pure validation in `internal/config` (chosen): every override names a known
    action; every effective key is a single character (the dispatcher compares
    `event.key`); no two actions resolve to the same key (ambiguous dispatch).
    All keys/labels reach the browser HTML-escaped via `esc`/`html/template`
    (ADR-0001). Editing flows through `Server.editConfig`, so a colliding/invalid
    binding is rolled back, not half-applied.

## Decision

Add a fixed, ordered `config.KeyActions()` set and a persisted
`Config.KeyBindings` override map; resolve them with `Config.Keymap()` and guard
the invariants in `validateKeyBindings` (called from `Config.Validate`). Surface
the keymap three ways: the **help overlay** (`#help-overlay`, body-level so it
survives htmx swaps, toggled by its bound key and closed by Esc), the **Help
page** shortcut table, and a **Keyboard shortcuts** section in the Settings form
(one single-key field per action; blank reverts to default). A small vanilla-JS
`keydown` dispatcher in the page shell reads `<body data-keymap>` and runs each
action against existing affordances, ignoring keystrokes typed into fields.

## Consequences

- Positive: the shortcuts are real and discoverable; the keymap is config-backed,
  pure-validated, and deterministic (sorted JSON, ordered action set); all
  surfaced text is escaped (ADR-0001). No new HTTP route — the overlay/keymap
  ride the existing index render and the Settings POST; editing reuses the
  rollback-on-invalid `editConfig` path. The docs' "config carries key bindings"
  claim is now true.
- Trade-off we accept: keys are constrained to a **single character** (matching
  `event.key`); chords/named keys (Ctrl-K, F1) are out of scope for this slice.
  Esc-closes-overlay is a fixed convention, not a binding.
- Known limitation: a Settings save swaps only `#main`, so a rebind takes effect
  on the **next full page load** (the live `data-keymap`/overlay are rendered by
  the index shell). Consistent with the rest of Settings ("applied on your next
  session"); tracked as TECH_DEBT #13.
- Contract change: `config.Config` grew the additive `keyBindings` map (omitempty,
  older files read clean) — recorded in CONTRACTS. Covered by `internal/config`
  `TestKeymap*`/`TestValidateRejectsBadKeyBindings`/`TestKeyBinding*` and
  `internal/web` `TestIndexRendersKeymapAndOverlay`/`TestSettingsSave*Keybinding*`;
  the JS behaviour by `e2e/tests/keybindings.spec.ts`.
