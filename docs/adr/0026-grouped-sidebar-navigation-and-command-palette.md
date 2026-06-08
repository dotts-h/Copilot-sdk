# 0026. Grouped left-sidebar navigation and a ⌘K command palette

- Status: accepted
- Date: 2026-06-08
- Deciders: Horia
- Related: the **second child of the UI/UX refresh epic**
  ([0045](../issues/0045-epic-ui-ux-refresh.md), roadmap v9) — the navigation-IA follow-on to the
  V21 token/theme foundation ([ADR-0025](0025-design-token-foundation-and-light-dark-theming.md)),
  on which it builds (the sidebar + palette are styled with the semantic tokens, never raw
  literals). Keeps the **no build chain / single committed CSS file** doctrine
  (`internal/web/static/app.css`) and the **minimal-JS, no-framework** posture — the palette reuses
  the keymap dispatch of
  [ADR-0014](0014-keybinding-surface-config-backed-keymap-with-minimal-js-dispatch.md) rather than
  adding a second key system. Touches `internal/web/pages.go` (the `pageNames` source),
  `internal/web/server.go` (`navGroup` / `handleIndex`), `internal/web/palette.go` (the new
  `commandPalette` render), `internal/web/templates/index.html` (the sidebar + the palette overlay +
  the small palette script), `internal/web/static/app.css`, the e2e suite
  (`nav.spec.ts` / `palette.spec.ts`, the extended `a11y.spec.ts`), `docs/CONTEXT.md` (the
  **sidebar** / **nav group** / **command palette** terms), and
  [issue 0047](../issues/0047-grouped-sidebar-command-palette.md). **No server route, no persisted
  schema, no `copilot.Client` change** — CONTRACTS unchanged; `navGroup` / `commandPalette` are
  unexported, so CODEMAP is unchanged.

## Context

A v9 research pass (recorded under epic 0045) reviewed the web UI against modern front-end practice.
After V21 cleared the theming/a11y foundation, the standout remaining IA gap is the **navigation**:
the shell renders a **flat top bar of 13 links** (`<header class="topbar"><nav class="nav">{{range
.Nav}}…{{end}}`) — Chat · Sessions · Telemetry · Skills · Instructions · Agents · Workflows · Runs ·
MCP · Snippets · Models · Settings · Help. Thirteen undifferentiated items is past the count where a
horizontal bar scans well (NN/g: that's left-sidebar territory), there is no grouping or hierarchy,
and there is no fast keyboard path to a page (the configurable keymap covers a handful of actions,
not arbitrary navigation).

This child regroups the nav into a **left sidebar** with labelled groups and adds a **⌘/Ctrl-K
command palette** so the grouping never slows a power user — without adding a build step, a CSS
framework, a JS framework, or a client-side router. It is deliberately scoped to navigation IA; the
telemetry dashboard (KPI cards + server-rendered SVG sparklines) and the motion/polish pass are
**separate children** of epic 0045.

The decisions an ADR must settle: **where the grouping lives**, **the group order + membership**,
**how the sidebar collapses on a narrow viewport**, **the palette's data source**, and **how ⌘K is
bound** relative to the existing configurable keymap.

## Considered options

- **Where the grouping lives.**
  - **Extend `pageNames` with a `group` field — one source, server-rendered groups (chosen).**
    `pageNames` is already the single ordered list of `{slug,label}` pages that drives the nav,
    `isNavSlug`, the `/slug` autocomplete, and `commandHelp`. Adding a third field `group` (and
    ordering the slice by group) keeps **one source of truth**: `handleIndex` folds consecutive
    same-group entries into a `[]navGroup` the template ranges over, and every other consumer of
    `pageNames` is unaffected (they ignore the new field). This is the project's "one fact, one home"
    doctrine applied to navigation.
  - *A separate grouping table (a `map[group][]slug` beside `pageNames`).* Rejected — a second list
    that must be kept in lockstep with `pageNames` (add a page, edit two places; the classic drift
    this repo's doctrine exists to prevent). The `group` field colocates the fact with the page it
    describes.

- **Group order + membership.**
  - **Primary (Chat, Sessions) · Build (Agents, Workflows, Skills, Instructions, Snippets) · Observe
    (Runs, Telemetry) · Config (Models, MCP, Settings) · Help — with Config + Help pinned to the
    bottom (chosen).** The grouping is by **user intent**: *Primary* is the daily driver (chat and
    its session history); *Build* is the forge — everything you author that compiles into a session
    (agents, workflows, skills, instructions, snippets); *Observe* is the read surfaces (run history,
    cost telemetry); *Config* is the rarely-touched settings (model, MCP servers, app settings); and
    *Help* is reference. Config and Help are **deferred to the bottom** of the column (progressive
    disclosure — NN/g: low-frequency, low-stakes destinations sink, so the eye lands on the work
    first). This is a strict re-grouping of the *same 13 pages* — no page is added or removed.
  - *Alphabetical, or frequency-only, or a flat-but-styled bar.* Rejected — alphabetical scatters
    related pages (Agents far from Workflows); a flat bar is the very problem; intent-grouping is the
    NN/g recommendation for a sidebar of this size.

- **How the sidebar collapses on a narrow viewport.**
  - **A CSS-only collapse: the same single `<header>`/`<nav>` reflows from a left column (wide) to a
    compact wrapping top bar (narrow) via a media query — no JS, the links stay in the DOM (chosen).**
    On a wide viewport the banner is a fixed-width left column with vertical groups and visible group
    labels; below the breakpoint the body switches to a column, the banner returns to a horizontal
    `flex-wrap` bar, and the group **labels hide** so it reads as a compact flat strip (the pre-V22
    behaviour, which already passed the no-overflow + usable-nav e2e guards). Because every link stays
    rendered and reachable (never `display:none` behind a toggle), the `ux.spec` "nav usable on a
    narrow viewport" guard holds with no test change, and there is no horizontal overflow (the bar
    wraps). **No JS, no router, no drawer state.**
  - *A JS hamburger drawer (toggle a `display:none` menu).* Rejected — it adds JS state and hides the
    links behind a control (a closed drawer fails a "click Settings" flow unless the test first opens
    it), for no gain over a reflow on a localhost tool. The CSS-only reflow keeps the markup identical
    across breakpoints.

- **The palette's data source.**
  - **A server-rendered list of `{slug,label,group}` items in the shell, filtered client-side — no
    new route (chosen).** The 13 pages are a tiny, static set already known at index render; emitting
    them once into the (hidden) palette overlay and filtering with a few lines of vanilla JS needs
    **zero** new server surface. This matches the existing help-overlay pattern (server-rendered into
    the shell, toggled client-side) and the no-route constraint that V21 also held.
  - *A fetched endpoint (`GET /palette` returning JSON/HTML, filtered server-side as you type).*
    Rejected — a round-trip per keystroke (or a new route + a fetch dependency) for a 13-item list
    that fits in the initial HTML. A new route is a CONTRACTS entry this feature does not need.

- **How ⌘K is bound (vs the configurable keymap).**
  - **⌘K is a fixed modifier chord, handled ahead of the configurable-keymap dispatch (chosen).** The
    ADR-0014 keymap is, by design, a map of **single unmodified keys** to actions, and its dispatcher
    *already* returns early when any of Ctrl/Meta/Alt is held (so a bound `t` never fires as ⌘T). ⌘K
    is therefore added as an explicit special-case **before** that early-return: a fixed
    `(metaKey||ctrlKey) && key==='k'` that opens the palette. Keeping it fixed avoids inventing a
    modifier-chord grammar for the keymap config (and a UI to edit it) for one global binding, and it
    matches the cross-app convention (⌘K is the de-facto command-palette chord). The palette's *items*
    then reuse the dispatcher's existing `navClick(slug)` seam, so the palette is not a second key
    system — just a new opener and a filtered list over the same navigation.
  - *Route ⌘K through the configurable keymap (ADR-0014).* Rejected for now — the keymap's value
    model is single-character keys; representing a modifier chord means widening that model and its
    Settings editor for a single binding. Recorded as a possible future extension if more chords are
    wanted, not adopted here.

## Decision

Add a `group` field to `pageNames` and order the slice by group: **Primary** (Chat, Sessions),
**Build** (Agents, Workflows, Skills, Instructions, Snippets), **Observe** (Runs, Telemetry),
**Config** (Models, MCP, Settings), **Help** (Help). In `handleIndex`, fold `pageNames` into a
`[]navGroup{Label, Items, PinnedStart}` (a new group starts whenever the `group` string changes;
the Config group carries `PinnedStart`). The shell's single `<header>` banner becomes a left
**sidebar** — still a `<header>` that contains `<nav class="nav">`, so the existing `header nav a`
selectors, the `navClick(slug)` / `cmdNav` seams, and the theme-toggle + `#cost-footer` are
unchanged; the nav renders labelled groups and the `PinnedStart` group is pushed to the bottom of
the column (`margin-top:auto`). A narrow-viewport media query reflows the column to a compact
wrapping top bar (group labels hidden), CSS-only. Add a body-level **command palette** overlay
(`commandPalette()` in a new `internal/web/palette.go`, mirroring `helpOverlay`):
`role="dialog" aria-modal="true"`, a labelled filter `<input>`, and a server-rendered `cmdk-item`
per page carrying `data-slug` / `data-label` / `data-group`. A small vanilla-JS palette
(`toggleCmdk` / `cmdkFilter` / `cmdkNav`) filters the list client-side and navigates the top match
via the existing `navClick`; ⌘/Ctrl-K opens it as a **fixed** chord handled before the keymap
dispatcher's modifier early-return, and Esc closes it and returns focus to the composer (the
help-overlay pattern). All styling uses the V21 tokens in the single committed CSS file. No build
step, no CSS/JS framework, no server route, no schema change.

## Consequences

- Positive: the 13 pages read as **five intent-grouped clusters** in a scannable left sidebar with
  config/help deferred to the bottom, and a **⌘K palette** gives a fast keyboard path to any page —
  the IA scales as the page set grows, with **no build chain and no framework**. The sidebar is the
  *same* `<header>`/`<nav>`, so the keymap dispatch, the `/slug` composer navigation, and the
  theme/cost chrome are reused, not reinvented.
- One-fact-one-home: a page's group lives **once**, on its `pageNames` row; the palette's items and
  the sidebar's groups both derive from that one list. Adding a page is still a one-line edit (slug,
  label, **group**).
- A11y (held in both themes): the always-visible sidebar is scanned by the existing per-page
  both-theme axe sweep, and the palette is added to that sweep in its open state; the nav is one
  labelled `<nav>` landmark and the palette is a labelled modal dialog with managed focus and
  Esc-to-close. The single-`<header>` / single-`main#main` landmark invariant is preserved (the
  banner is still the one `<header>`). The `ux.spec` no-overflow + usable-nav guards and the
  `theme.spec` toggle stay green (the links never leave the DOM; the toggle is untouched).
- Minimal-JS held: the palette adds one opener chord and a client-side filter that reuse the existing
  `navClick(slug)` dispatch — **no second key system, no client-side router, no framework.** ⌘K is a
  fixed chord; routing it through the configurable keymap is recorded as a possible future extension.
- Scope held: this is the **navigation-IA** child only. The telemetry dashboard (V23) and the
  View-Transition/component polish pass (V24) are **separate children** of epic 0045, each taking its
  own ADR. The deferred **Open Props** primitives + CSS **`@layer`** (recorded in ADR-0025) remain
  deferred-additive; this child does not adopt them.
- Contract surface: **none.** No new route, no `copilot.Client` change, no persisted schema — the
  palette is client-side over server-rendered HTML. CONTRACTS is unchanged; CODEMAP is unchanged
  (`navGroup` and `commandPalette` are unexported).
</content>
