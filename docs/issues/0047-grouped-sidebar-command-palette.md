---
id: 0047
title: "Navigation → grouped left sidebar + ⌘K command palette (roadmap v9, item V22)"
status: open
severity: medium
group: 0045
github:
links:
  adr: [0026]
  prs: []
  issues: [0045]
  regression:
---

## Summary

The web shell's navigation is a **flat top bar of 13 items** (Chat · Sessions · Telemetry · Skills ·
Instructions · Agents · Workflows · Runs · MCP · Snippets · Models · Settings · Help) in
`internal/web/templates/index.html` (`<header class="topbar"><nav class="nav">…`). Thirteen
undifferentiated links is past the count where a top bar scans well (NN/g: that's left-sidebar
territory) — there is no grouping, no hierarchy, and no fast keyboard path to a page. This is the
**navigation-IA** gap epic [0045](0045-epic-ui-ux-refresh.md) (roadmap v9 — UI/UX refresh) named as
the highest-impact follow-on to the V21 token/theme foundation.

**V22 regroups the nav into a left sidebar** with four labelled groups + a pinned Help, and adds a
**⌘/Ctrl-K command palette** so grouping never slows a power user. It takes **ADR-0026** for the
decisions (where grouping lives, group order/membership, narrow-viewport collapse, the palette's
data source, and the ⌘K binding). Built on V21's tokens — **no build step, no framework, no new
server route, no schema change**.

## Repro
1. Open the app — the nav is a single flat row of 13 links with no grouping or hierarchy.
   - **Expected:** the pages are grouped by purpose (Primary / Build / Observe / Config / Help) in a
     scannable left sidebar, with config/help deferred to the bottom (progressive disclosure), and a
     ⌘K palette to jump to any page from the keyboard.
   - **Actual (before V22):** 13 flat top-bar items; no grouping, no command palette.

## Proposed resolution

- **`internal/web/pages.go`:** extend `pageNames` with a `group` field (**one source**,
  server-rendered groups) and order it by group: Primary (Chat, Sessions) · Build (Agents, Workflows,
  Skills, Instructions, Snippets) · Observe (Runs, Telemetry) · Config (Models, MCP, Settings) · Help.
- **`internal/web/server.go`:** `handleIndex` folds `pageNames` into a `[]navGroup`
  (`{Label, Items []navItem, PinnedStart}`) so the template renders labelled groups; the Config group
  carries `PinnedStart` so it (and Help after it) sit at the **bottom** of the column.
- **`internal/web/palette.go` (new render fn):** a `commandPalette()` mirroring `helpOverlay` — a
  body-level `role="dialog" aria-modal="true"` overlay with a filter `<input>` and a server-rendered
  list of `{slug,label,group}` items (no new route; client-side filter).
- **`internal/web/templates/index.html`:** the `<header>` banner becomes the left **sidebar**
  (still a single `<header>` containing `<nav class="nav">`, so the existing `header nav a` selectors
  and `navClick`/`cmdNav` seams are unchanged); the cmdk overlay is added beside the help overlay; a
  small vanilla-JS palette (`toggleCmdk`/`cmdkFilter`/`cmdkNav`) **reuses the existing keymap
  dispatcher**, and ⌘K is wired as a fixed modifier chord ahead of the dispatcher's modifier
  early-return.
- **`internal/web/static/app.css`:** the single committed CSS file gains the `.sidebar` /
  `.nav-group` / `.nav-group-label` layout (tokens only — no raw hex/rgba) and the `.cmdk*` overlay
  styles; a narrow-viewport media query collapses the sidebar to a compact wrapping top bar (CSS-only,
  no JS router) so the links stay in the DOM and reachable.
- **ADR-0026** (written first, ADR-0004): grouping in `pageNames`; group order/membership; CSS-only
  narrow collapse; client-side-filtered server-rendered palette list (no route); ⌘K fixed (not in the
  configurable single-key keymap of ADR-0014). CONTEXT gains **sidebar**, **nav group**, **command
  palette**. **No CONTRACTS / CODEMAP change** (no new route; `navGroup`/`commandPalette` are
  internal, unexported).

## Tests (failing-first)

- **Go unit (`server_test.go`):** the index renders `class="…sidebar"`, the four group labels in
  order (Primary → Build → Observe → Config) with Help last, each item under its group, and the
  command-palette overlay (`id="cmdk-overlay"`, `aria-modal`, a `cmdk-item` per page, the
  `toggleCmdk` function, and the ⌘K wiring).
- **e2e `nav.spec.ts` (new):** the sidebar renders the four groups in order with the right items and
  Help last; clicking a sidebar item swaps `#main`; the existing theme toggle + cost footer stay
  reachable.
- **e2e `palette.spec.ts` (new):** ⌘/Ctrl-K opens the palette (`aria-modal`), typing filters the
  list, Enter navigates to the match (swaps `#main`), Esc closes and returns focus to the composer;
  the palette is keyboard-reachable.
- **e2e `a11y.spec.ts` (extended):** the both-theme axe scan now covers the opened palette (the
  always-visible sidebar is already covered by every page scan); `ux.spec.ts` (no horizontal scroll at
  desktop+mobile, nav usable on a narrow viewport) and `theme.spec.ts` stay green.

## Notes
- **ADR-0026:** the navigation-IA child. Decisions: a `group` field on `pageNames` (one source) over a
  separate grouping table; the Primary/Build/Observe/Config/Help order with config+help pinned bottom;
  a CSS-only narrow collapse (the sidebar reflows to a wrapping top bar) over a JS drawer; a
  client-side filter over a server-rendered `{slug,label,group}` list over a fetched endpoint (**no new
  route**); ⌘K as a **fixed** modifier chord (the ADR-0014 keymap is reserved for unmodified single
  keys, which the dispatcher already gates on).
- **Differentiator:** neither — a **presentation/IA** child of epic 0045. It makes the growing page set
  scannable and adds a power-user keyboard path.
- **Scope held:** the telemetry dashboard (V23, KPI cards + server-rendered SVG sparklines) and the
  View-Transition/component polish pass (V24) are **separate children**, each with its own ADR.
- **Numbering:** issue **0047** (next free after 0046), **ADR-0026** (next after 0025); epic stays
  **0045**.
</content>
</invoke>
