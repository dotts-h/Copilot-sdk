# 0028. Motion & polish: htmx per-navigation View Transitions (NOT global), plus a token-driven component pass

- Status: accepted
- Date: 2026-06-08
- Deciders: Horia
- Related: the **fourth and final child of the UI/UX refresh epic**
  ([0045](../issues/0045-epic-ui-ux-refresh.md), roadmap v9) — the motion/polish capstone on the V21
  token/theme foundation ([ADR-0025](0025-design-token-foundation-and-light-dark-theming.md)), the V22
  navigation regroup ([ADR-0026](0026-grouped-sidebar-navigation-and-command-palette.md)), and the V23
  telemetry dashboard ([ADR-0027](0027-telemetry-kpi-dashboard-server-rendered-inline-svg.md)). Keeps
  the **no build chain / single committed CSS file** doctrine (`internal/web/static/app.css`) and the
  **minimal-JS, no-framework** posture — the page motion is htmx config + CSS, with **zero new JS**.
  Touches `internal/web/templates/index.html` (the nav links' per-swap `transition:true` opt-in),
  `internal/web/static/app.css` (the motion/elevation tokens, the View-Transition rules + a
  reduced-motion guard, the interactive-control transition + card elevation), the e2e suite
  (`motion.spec.ts`, plus a settle-aware `navTo` in `helpers.ts` so the async swap stays deterministic,
  and the both-theme `a11y.spec.ts`), `docs/CONTEXT.md` (the **View Transition** / **component polish
  pass** terms), `docs/REGRESSIONS.md` (the global-opt-in dead-end), and
  [issue 0049](../issues/0049-motion-and-polish.md). **No server route, no persisted schema, no
  `copilot.Client` change** — CONTRACTS and CODEMAP are unchanged (the change is templates + CSS + a
  test).

## Context

A v9 research pass (recorded under epic 0045) reviewed the web UI against modern front-end practice.
V21 cleared the theming/a11y foundation, V22 regrouped the navigation, and V23 turned the Telemetry
page into a KPI/SVG dashboard. The remaining child is **presentation polish**: page navigation
**hard-cuts** (an instant innerHTML swap of `#main`), interactive controls **snap** between their
hover/focus states, and the card surfaces sit flat on the page. Modern **vanilla CSS** closes all
three with no framework and no build step — htmx exposes the browser **View Transitions API**
(`document.startViewTransition`) behind a one-line config flag, and the V21 token layer already
carries the palette a component pass needs.

The hard constraints hold: **no build chain, a single committed CSS file, htmx + server-rendered
templates, minimal JS / no framework.** The decisions an ADR must settle: **how page swaps opt into
View Transitions and how that interacts with the high-frequency SSE streaming swaps**, **how motion
honors `prefers-reduced-motion`**, **the scope of the component pass**, and **whether to adopt the
deferred Open Props / `@layer` options here**.

## Considered options

- **How page swaps opt into View Transitions.**
  - **Per-navigation opt-in — `transition:true` on the sidebar nav links only (chosen, after the
    global opt-in was tried and rejected).** The nav links are a single `{{range .Nav}}` loop in
    `index.html`, so one edit opts **all** navigation into `document.startViewTransition` — the `#main`
    page swap cross-fades, scoped to the panel by `view-transition-name: main` so the sidebar shell
    stays put. The ⌘K palette routes through these same links (`navClick`), so it inherits the
    transition for free. Navigation is the **only** swap that should cross-fade, and it is centralized
    in one template, so opt-in is one site, not "dozens."
  - *`htmx.config.globalViewTransitions: true` (the global flag, the epic's first-named approach).*
    **Tried and rejected on the evidence** (recorded in REGRESSIONS). The global flag wraps **every**
    swap in a view transition — and this app streams through **`hx-swap-oob`**: a `/send` or a
    workflow-run response pushes out-of-band updates into `#timeline`/`#status`/`#lanes` *mid-stream*,
    and SSE listeners swap a `delta` per token plus live `cost`/`lanes`/`status`. The per-swap
    `transition:false` modifier can opt out the **SSE listeners** and the **command menu**, but it
    **cannot** reach the OOB swaps embedded in a POST response. Empirically (the full e2e suite), the
    global flag made workflow runs and chat turns **fail to reach their terminal state** — the
    completion swap collided with the constant stream and was dropped (`run-status.done` never
    appeared; a turn's `.abort` never cleared). The browser also runs **one** view transition at a
    time, so an OOB-heavy stream starves/aborts transitions. Global is the wrong fit for an
    OOB-streaming UI; per-navigation opt-in touches none of the streaming/OOB swaps.
  - *A JS/CSS animation library or hand-rolled crossfade.* Rejected — a framework/build dependency for
    motion the browser supplies natively behind htmx's per-swap flag.

- **Keeping the async swap deterministic for the test suite.**
  - **A settle-aware `navTo` test helper (chosen).** A view transition **defers** the swapped DOM
    update by a frame (it runs inside `startViewTransition`'s update callback), so navigation is now
    **async**: a synchronous read right after a nav click (e.g. a baseline `.count()` before adding a
    row) would see the **old** page. Rather than relax the feature, the `navTo` helper waits for
    htmx's next `htmx:afterSettle` (counted via an init script), which fires **after** the swap's DOM
    update — even inside the transition callback — so every caller reads the settled page. One helper
    change makes the whole suite robust to async navigation; no production code and no hard waits (the
    project bans `waitForTimeout`).
  - *Per-test fixes (await a landmark before each post-nav read).* Rejected — the racy pattern recurs
    across ~5 tests (MCP/Workflows/Snippets adds, the budget gate, `ux.spec`); fixing the shared
    helper once is correct test hygiene and covers future callers, vs. scattering the same wait.

- **How motion honors `prefers-reduced-motion`.**
  - **Extend the existing global reduced-motion reset, plus an explicit guard for the View-Transition
    pseudo-elements (chosen).** ADR-0025's reset already collapses `transition-duration`/
    `animation-duration` to ~0 on `*, *::before, *::after`, which covers the new interactive-control
    transitions for free. But the **View-Transition pseudo-elements** (`::view-transition-old/new/group`)
    live on the document root and are **not** matched by `*`, so a page swap would still animate; an
    explicit `animation: none !important` on them under the reduced-motion media query silences the
    cross-fade. Motion is therefore **opt-out** — an a11y requirement, not a nicety — and the e2e suite
    asserts both halves (a control's transition is non-zero normally and ~0 under reduced motion).
  - *Rely only on the existing `*` reset.* Rejected — it cannot reach the root-level transition
    pseudo-elements, so the page cross-fade would ignore the user's preference.

- **The scope of the component pass.**
  - **A token-driven, contrast-neutral pass: one shared motion language on interactive controls,
    resting elevation on card surfaces, and a tactile button press (chosen).** Add `--speed`/`--ease`
    motion tokens and theme-aware `--shadow`/`--shadow-lg` elevation tokens (rgba via `light-dark()`,
    matching the existing `--hover`/`--raised` overlays). A single rule gives the interactive controls
    (nav links, buttons, toggles, the effort/window selectors, palette items, export links) an eased
    transition on their **already-animated** properties (color/border/background/box-shadow/filter/
    transform) — never layout — so hover/focus eases in the same motion language as the page
    cross-fade; the solid primary buttons sink 1px on `:active`; the card surfaces (KPI cards, run
    records, the live-run panel, session rows) get a subtle resting shadow. The pass changes **no
    color token and no text/background pairing**, so the both-theme axe contrast scan is unaffected.
  - *A deeper restyle (new palette, radii overhaul, literal cleanup of the older component rules).*
    Rejected for this child — it risks the both-theme contrast baseline (the older rules carry tinted
    `rgba` backgrounds with carefully-tuned AA foregrounds, e.g. the diff lane and the perm/ask/plan
    forms) for marginal polish. The contrast-neutral pass is the high-value, low-risk capstone; a
    literal-cleanup paydown can be its own future item.

- **Adopt the deferred Open Props / CSS `@layer` here?**
  - **No — keep both deferred-additive (chosen).** ADR-0025 recorded Open Props (a primitives library)
    and `@layer` (cascade structuring) as deferred, additive options needing no markup change. Folding
    either in now would either **vendor an external CSS file** (against the single-committed-file
    doctrine, for primitives the hand-tuned tokens already cover) or **restructure the whole cascade**
    (`@layer` is only worth it as a deliberate structural pass, not bundled into a polish child where a
    mis-ordered layer could silently change specificity across 650 lines). Both remain available as
    future additive paydown.

- **No new route / schema (confirm).**
  - **None — the motion is templates + CSS over the existing swaps (chosen + confirmed).** V24 adds
    **no server route, no schema change, no `copilot.Client` change**, and **no new JS** — it is a
    per-swap `transition:true` on the nav links plus CSS.

## Decision

Opt **page navigation** into the browser View Transitions API with the per-swap `transition:true`
modifier on the sidebar nav links (one `{{range .Nav}}` loop in `index.html`; the ⌘K palette inherits
it via `navClick`), scoping the cross-fade to the `#main` panel with a `view-transition-name: main`.
**Not** `htmx.config.globalViewTransitions` — the global flag wraps the app's `hx-swap-oob` streaming
updates too and dropped workflow-run / chat-turn completion swaps (recorded in REGRESSIONS); per-swap
opt-in touches only navigation, which never streams. Tune the cross-fade to a shared `--speed`/`--ease`
token, and guard the View-Transition pseudo-elements under `prefers-reduced-motion` with `animation:
none !important` (the existing global reset already neutralizes the element transitions). Make the now
**async** nav swap deterministic for tests with a settle-aware `navTo` (waits for htmx's next
`htmx:afterSettle`). Run a contrast-neutral component pass on the V21 tokens: add `--speed`/`--ease`/
`--shadow`/`--shadow-lg` tokens, an eased transition on the interactive controls, a 1px `:active` sink
on the solid primary buttons, and a resting `--shadow` on the card surfaces. No build step, no
framework, no new JS, no server route, no schema change. Open Props and `@layer` stay deferred-additive.

## Consequences

- Positive: page navigation **cross-fades** instead of hard-cutting, interactive controls **ease**
  their hover/focus states, and cards read as **raised** — a coherent motion language across page and
  element — all with **zero new JS, no framework, no build step**. The transition is scoped to `#main`,
  so the sidebar stays put.
- Streaming integrity held **by construction**: only the nav links opt in, so no SSE/OOB swap is ever
  wrapped — the global flag, which *would* wrap them (and broke run/turn completion), is explicitly
  rejected (REGRESSIONS). `motion.spec` pins that the streaming swaps carry no `transition:` modifier,
  so a future edit can't silently re-introduce the global behavior.
- Test determinism: a view transition defers the swap a frame, so navigation is async; the settle-aware
  `navTo` waits for `htmx:afterSettle` (no hard wait, no production code), keeping the ~5 baseline-count
  tests (and any future post-nav read) deterministic.
- A11y (both themes): motion is **opt-out** — the global reset plus the explicit View-Transition-pseudo
  guard collapse all motion to ~0 under `prefers-reduced-motion`, asserted by `motion.spec`; the
  component pass changes no color pairing, so the both-theme axe contrast scan is unaffected and
  navigation still settles under reduced motion.
- One-fact-one-home: the motion speed/ease and elevation live once as tokens (`--speed`/`--ease`/
  `--shadow`/`--shadow-lg`); the navigation opt-in is one template loop; the async-nav wait is one
  helper.
- Scope held: this is the **motion/polish** capstone. The deferred **Open Props** primitives + CSS
  **`@layer`** (recorded in ADR-0025) remain deferred-additive; this child does not adopt them, and a
  deeper literal-cleanup restyle is left as future paydown.
- Contract surface: **none.** No new route, no `copilot.Client` change, no persisted schema. CONTRACTS
  and CODEMAP are unchanged (templates + CSS + a test). **On merge, epic 0045 (roadmap v9) is
  exhausted** — all four children (V21–V24) shipped.
