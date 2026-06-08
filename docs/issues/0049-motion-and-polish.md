---
id: 0049
title: "Motion & polish — htmx View-Transition page swaps + a token-driven component pass (roadmap v9, item V24)"
status: open
severity: low
group: 0045
github:
links:
  adr: [0028]
  prs: []
  issues: [0045]
---

## Summary

The web UI's **functional** surface is mature (roadmaps v4–v8) and its **presentation** is, after V21
(tokens + theme), V22 (sidebar + ⌘K), and V23 (KPI/SVG dashboard), nearly there. The remaining gap is
**motion & polish**: page navigation **hard-cuts** (an instant `#main` innerHTML swap), interactive
controls **snap** between their hover/focus states, and the card surfaces sit flat. This is the
**motion/polish** child epic [0045](0045-epic-ui-ux-refresh.md) (roadmap v9 — UI/UX refresh) named as
its fourth and last child.

**V24 adds motion & polish without a build step, a framework, or any new JS:**

- **View-Transition page swaps (per-navigation).** Opt the sidebar nav links into the browser View
  Transitions API with htmx's per-swap `transition:true` (one `{{range .Nav}}` loop; the ⌘K palette
  inherits it via `navClick`), scoped to the `#main` panel with a `view-transition-name` so only the
  page content cross-fades (the sidebar stays put). Degrades to an instant swap where unsupported, and
  is silenced under `prefers-reduced-motion`. **Not** `globalViewTransitions` — see below.
- **Global opt-in tried and rejected.** The epic's first-named approach (`globalViewTransitions`) wraps
  **every** swap, including this app's `hx-swap-oob` streaming updates (`/send` and run responses push
  OOB timeline/status/lanes mid-stream), and **dropped workflow-run / chat-turn completion swaps** in
  the e2e suite (`run-status.done` never appeared; a turn's `.abort` never cleared). Recorded as a
  dead-end in REGRESSIONS. Per-navigation opt-in touches no streaming/OOB swap.
- **A token-driven component pass.** New `--speed`/`--ease` motion tokens and theme-aware
  `--shadow`/`--shadow-lg` elevation tokens; an eased transition on the interactive controls (so
  hover/focus matches the page cross-fade); a 1px `:active` sink on the solid primary buttons; a
  resting shadow on the card surfaces (KPI cards, run records, the live-run panel, session rows). The
  pass changes **no color pairing**, so the both-theme axe contrast scan is unaffected.
- **A settle-aware `navTo`.** A view transition defers the swap a frame, making navigation async; the
  `navTo` e2e helper now waits for htmx's next `htmx:afterSettle` so a synchronous post-nav read (a
  baseline `.count()`) sees the new page — one helper change, no production code, no hard wait.

It takes **ADR-0028** for the decisions (per-navigation opt-in vs. the rejected global flag; the
reduced-motion guard for the View-Transition pseudo-elements; the settle-aware nav helper; the
contrast-neutral pass scope; keeping Open Props / `@layer` deferred). **No server route, no schema
change, no `copilot.Client` change, no new JS** — the change is templates + CSS + an e2e spec/helper.

## Acceptance

- [x] The sidebar nav links opt into a View Transition with per-swap `transition:true` (the ⌘K palette
      inherits it); `#main` carries a `view-transition-name` so the cross-fade is scoped to the panel.
- [x] `globalViewTransitions` is NOT used — it wraps the `hx-swap-oob` streaming updates and dropped
      run/turn completion swaps (recorded in REGRESSIONS); only navigation opts in.
- [x] Motion is opt-out: the global reset plus an explicit `::view-transition-*` guard collapse all
      motion to ~0 under `prefers-reduced-motion`; navigation still settles.
- [x] A settle-aware `navTo` (waits for `htmx:afterSettle`) keeps the now-async nav deterministic for
      synchronous post-nav reads.
- [x] A token-driven component pass (motion + elevation tokens, eased controls, button press, card
      shadows) that changes no color pairing.
- [x] `motion.spec.ts` asserts the per-nav opt-in (scoped to `#main`), that the streaming swaps carry
      no transition, reduced-motion navigation, and the eased-then-zeroed control transition; the
      full e2e suite (incl. the both-theme `a11y.spec` axe scan) stays green.
- [x] Go gates green (`make lint && make test`, coverage floor 65%) — unchanged (templates + CSS +
      test).

## Resolution

**Shipped** on branch `claude/next-features-research-8aBvS-Hq8Tb` (pending merge). `index.html`'s nav
links carry the per-swap `transition:true` opt-in; `app.css` gains the `--speed`/`--ease`/`--shadow`/
`--shadow-lg` tokens, the `view-transition-name: main` + tuned `::view-transition-old/new(main)`
cross-fade, the reduced-motion guard for the transition pseudo-elements, the interactive-control
transition, the `:active` button sink, and the card elevation. `motion.spec.ts` guards the behaviors
and `helpers.ts` gains a settle-aware `navTo`; the full e2e suite (135 passed, the one flaky run a
load-induced demo-completion timeout that passes on retry) and the Go suite stay green. The global
opt-in was tried first and rejected (it dropped run/turn completion swaps over `hx-swap-oob`) —
recorded as a dead-end in REGRESSIONS. Takes **ADR-0028**. No route, no schema, no `copilot.Client`
change, no new JS. **On merge, epic 0045 (roadmap v9 — UI/UX refresh) is exhausted** — all four
children (V21–V24) shipped.
