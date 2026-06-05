# Next-features research — my-orchestra

> Research deliverable, not a commitment. Grounds candidate next features in the
> current codebase (read 2026-06-05), the tech-debt register, and the product's
> two differentiators: **cost-awareness** (the meter) and **orchestration** (the
> name). Each item names the seam/files it touches, a rough effort (S/M/L), and
> why now. Promote the ones you pick into `docs/issues/` via `tracking-issues`
> and record any architectural choice as an ADR.

## Where the product already is

The Chat path is at parity and then some: streaming text/reasoning split, the
tool timeline, inline permissions, **ask_user**, **plan review**, **elicitation**,
**subagent** start/end, **compaction**, queued input, attachments, slash commands,
forge CRUD for skills/instructions/agents, SDK-backed session pick/resume/delete,
an editable Settings page, a live statusline + Telemetry page, a Wails desktop
shell, and a full Go + Playwright test pyramid. Every normalized `copilot.Event`
is handled in `session.go`. So "next features" is no longer about finishing the
chat loop — it's about turning passive surfaces into active ones and filling the
few remaining control surfaces.

The open obligations already tracked live in `docs/TECH_DEBT.md` (per-session
statusline totals, desktop installers, Wails-alpha pin, on-disk `SKILL.md`
model, single-session doc drift). Those are paydown, not product. The features
below are net-new, ranked by value × fit.

---

## Tier 1 — make cost *active* (the core differentiator)

Today the meter only **observes**. `telemetry.Budget.Remaining/FractionUsed`
(`internal/telemetry/credits.go:214`) are pure reads; nothing enforces, warns,
or projects. The README leads with "a coding session never surprises you on the
bill" — yet nothing actually intervenes. This is the highest-leverage gap.

### 1.1 Budget guardrails (soft warn + hard cap)  — **M**
- **What:** Soft threshold (e.g. 80% of allowance → ambient warning banner +
  statusline turns amber) and an optional hard cap that pauses the next turn
  with an inline "this would exceed your budget — proceed / raise cap?" control,
  reusing the existing permission-form pattern (`permBridge`, `/perm/{id}`).
- **Why now:** the budget type, the live meter, and the inline-approval UX
  primitive all already exist; this wires them together.
- **Touches:** `internal/telemetry` (threshold/projection helpers, kept pure),
  `internal/web/session.go` (check on `EvUsage`/before `Send`), `config.Config`
  (cap + thresholds, atomic save), a guard test per branch.

### 1.2 Pre-flight turn cost estimate  — **S/M**
- **What:** Before a turn runs, show an estimate ("~N credits at current
  context") in the composer, derived from `PriceBook` × the live context-window
  fill (`EvContextWindow` already arrives). Makes the abort decision informed.
- **Touches:** `telemetry.Price` (already pure), `renderStatline`/composer.

### 1.3 Persisted spend history + trends  — **M/L**
- **What:** The `Meter` is in-memory only, so all telemetry dies on restart.
  Persist per-session/per-day spend (append-only JSON, atomic write like config)
  and add a small trend view to the Telemetry page (spend over time, per-model
  share, CSV export).
- **Why now:** "cost-aware" is undercut by amnesia across restarts; this is the
  difference between a live gauge and an accountable ledger.
- **Touches:** new `internal/telemetry` store (or `internal/config` sibling),
  Telemetry page partial, a schema entry in CONTRACTS + a migration note.

## Tier 2 — make it an *orchestra* (the name's promise)

Subagent events (`EvSubagentStart/End`) are already normalized and rendered as
background activity, but there is **no control surface** to compose or run
multiple agents. The product is named for orchestration it doesn't yet expose.

### 2.1 Multi-agent run / handoff surface  — **L** ✅ shipped (ADR-0013, issue 0010)
- **What:** A forge **Workflow** (an ordered list of (agent, task) steps + a
  `mode`) runs as **lanes**: pick an agent, hand off to the next on completion
  (sequential), or fan out to parallel agents — each watched as its own lane in a
  dedicated `#lanes` panel on the chat page (the subagent rendering was the seed).
- **Shipped:** `ctxforge.Workflow`/`WorkflowStep` (pure type + `Validate` +
  whole-forge step→agent referential integrity + `CompileWorkflow`), a pure
  `workflowRun` state machine (unit-tested for both modes, no client), a Workflows
  CRUD nav page with a **▶ run** control, lanes routed by `SessionID` (sole-running
  fallback for the mock), per-lane metered cost, and a seeded sequential demo
  workflow. **Sequential ships fully; parallel is in the model/engine/wiring** with
  the demo covering sequential (TECH_DEBT #12). Lead-with-an-ADR done: ADR-0013.
- **Touched:** `internal/ctxforge` (`workflow.go`), the seam
  (`CreateSession`/`Send` per step), `internal/web` (`workflow.go`, the run-branch
  in `session.go`, lanes templates/CSS), `bootstrap` seed + `demo.go`.

### 2.2 MCP server management page + curated defaults  — **M** ✅ shipped (ADR-0010, issue 0006)
- **What:** Skills/Instructions/Agents have full CRUD pages; **MCP servers do
  not** — `settings.go:15` says outright they "are not exposed here," yet
  `ctxforge.MCPServer` is a first-class forge entity that `Compile` already wires
  into the session. Add the nav page + add/edit/toggle/delete, mirroring the
  existing forge-CRUD pattern and validated builders. **And** ship a curated set
  of well-known servers baked in by default.
- **Bake-in approach (decided):** seed a handful of well-known stdio servers
  (filesystem, git, fetch, …) into the forge **disabled by default**, plus a
  **preflight** that checks whether each server's `Command` resolves on `PATH`
  (`exec.LookPath`) and marks the unavailable ones in the UI instead of letting
  them fail at session start.
  - **Why disabled + preflight, not just a config seed:** MCP servers here are
    **stdio = external processes** (`MCPStdioServerConfig`, `sdkclient.go:145`),
    so a seeded `npx …` entry still needs the command present on the host. Auto-
    enabling would surprise-fail when node/the binary is absent and clashes with
    the project's offline single-binary value (htmx is vendored precisely to
    avoid runtime fetches). Seeding *config* is cheap; baking in *capability*
    without the host dep is a separate, larger move — see the follow-up below.
  - **Secrets caveat:** the highest-value servers (web fetch/search, GitHub) need
    API keys. `MCPServer.Env` exists but there's no secrets UI/handling; the
    curated defaults should prefer key-free servers, and a secrets story is its
    own scoped item before shipping key-requiring ones.
  - **Follow-up (not now):** embed a first-party Go MCP server (sidecar exec or,
    if the SDK ever exposes non-stdio registration, in-process) for a true
    out-of-the-box baseline with no external runtime. Tracked, not built.
- **Why now:** it's the one forge entity with no UI; MCP is how users extend the
  agent's tools, so this unblocks real customization. Lowest-novelty, high-utility.
- **Touches:** `internal/web/pages.go` (nav + the e2e `pages.length` count —
  see REGRESSIONS testing note), `forms.go`, new routes in `hub.go`, CONTRACTS
  route table; `cmd/my-orchestra` `seedForge` (curated entries) + a preflight
  helper (`exec.LookPath`). Reuses the rollback-on-invalid save path.

## Tier 3 — polish that compounds

### 3.1 Diff review lane  — **M** ✅ shipped (ADR-0012, issue 0009)
WEB_UI_PLAN UX principle #5 ("diffs get a review lane") was only partially met:
file edits rendered as the same bare permission prompt as a shell command, even
though the runtime hands us the proposed change. Now a file-write permission
(`PermissionRequestWrite` carries a unified `Diff`/`FileName`/`Intention`) renders
the **diff review lane**: a collapsible, side-numbered **inline** unified diff with
a diffstat and the approve/reject attached, posting to the same `/perm/{id}` flow.
The diff is parsed server-side by a pure, unit-tested `parseUnifiedDiff`
(`internal/web/diff.go`) and HTML-escaped (ADR-0001). Inline (not side-by-side) and
the SDK-permission seam (not a new gate) were the decisions — see ADR-0012.

### 3.2 Per-session telemetry totals  — **S/M**  *(TECH_DEBT #2)* ✅ shipped (ADR-0011, issue 0008)
Statusline credits/tokens were meter-global, not per-session. Scoped a per-session
meter (`Server.sessionMeter`, same price book as the account-wide meter) recorded
alongside the global meter on each `EvUsage`; `renderStatline` reads it so the
statusline reflects *this* conversation. The topbar cost footer, the hard-cap
projection, and the Telemetry month-to-date rows stay account-wide (budget
enforcement/accounting must be cumulative — the remaining ledger-derived step is
TECH_DEBT #9). Pairs naturally with 1.3 (the persisted ledger already tags
`SessionID`).

### 3.3 Keybinding surface  — **S** ✅ shipped (ADR-0014, issue 0011)
The docs claimed `config.Config` carried key bindings, but it didn't — and the
web UI had no shortcuts at all. Shipped a real, config-backed keymap: a fixed
ordered action set in code (`config.KeyActions()`) with persisted **overrides**
(`Config.KeyBindings`, `omitempty`), resolved by `Config.Keymap()` and
pure-validated (known id, single-char key, no duplicate). Surfaced three ways —
a body-level **help overlay** (toggled by its key, closed by Esc, survives htmx
swaps), the **Help page** shortcut table, and a **Keyboard shortcuts** section in
Settings — and dispatched by a small vanilla-JS `keydown` handler that reads
`<body data-keymap>`, ignores keystrokes typed into fields, and routes each
action to an existing affordance. Escape-first throughout (ADR-0001); editing
reuses `editConfig` rollback-on-invalid.

### 3.4 Prompt/snippet library  — **M**
Saved, reusable prompts insertable from the composer (a lighter cousin of skills,
which are system-message context, not one-shot prompts). Persist in the forge or
config; surface via the autocomplete that already powers slash commands.

## Tier 4 — platform / distribution (from the debt register)

- **Desktop installers** (.dmg/.msi/.deb/AppImage) via `wails3 package` —
  TECH_DEBT #5; only raw binaries ship today.
- **Wails v3 stable migration** when it lands — TECH_DEBT #6 (currently pinned to
  `v3.0.0-alpha.98`, which also forced the Go 1.25 floor).
- **On-disk `SKILL.md` folder model** — TECH_DEBT #1; deferred pending real demand
  for per-skill resources/folders the SDK may not yet expose.

---

## Recommended sequencing

1. **1.2 → 1.1 → 1.3** — turn cost from a gauge into a guardrail-and-ledger; this
   is the product's reason to exist and every piece reuses existing primitives.
2. ~~**2.2 (MCP page)** — closes the one missing forge CRUD; small and unblocking.~~
   ✅ shipped (ADR-0010): MCP nav page + add/edit/toggle/delete, curated stdio
   servers seeded disabled with an `exec.LookPath` preflight badging unavailable ones.
3. **3.2 + 3.1** — per-session totals and the diff lane; visible polish.
   3.2 ✅ shipped (ADR-0011): a per-session `Meter` scopes the statusline to *this*
   conversation; budget gauge / cap / Telemetry stay account-wide.
   3.1 ✅ shipped (ADR-0012): a file-write permission renders an inline diff review
   lane (collapsible, side-numbered, diffstat) with approve/reject on the existing
   `/perm` flow; the diff is parsed by a pure `parseUnifiedDiff` and escaped. 2.1 next.
4. ~~**2.1 (orchestration)** — the big bet; do it once 1.x has hardened the
   multi-run cost accounting it will lean on, and lead with an ADR.~~ ✅ shipped
   (ADR-0013): a forge **Workflow** runs as **lanes** — sequential handoff (each
   lane's output feeds the next) or parallel fan-out — each a sub-run on the seam's
   session lifecycle, watched in a `#lanes` panel; per-lane cost folds into the
   existing meters/ledger. Sequential is end-to-end (demo + e2e); parallel is in the
   model/engine (TECH_DEBT #12).
5. **3.3 (keybinding surface)** ✅ shipped (ADR-0014): a config-backed keymap
   (fixed action set + persisted overrides, pure-validated) surfaced in a help
   overlay + the Help page + a Settings section, dispatched by a small vanilla-JS
   `keydown` handler. **The validated roadmap is now down to 3.4 (prompt/snippet
   library)** — build it next, or kick off a fresh next-features research pass.

Each item: write the failing test first, keep domain logic pure, run
`make lint && make test` (coverage floor 65%), and fold its ADR/CONTRACTS/
REGRESSIONS updates into the same feature branch (ADR-0004).
