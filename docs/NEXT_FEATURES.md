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

### 2.1 Multi-agent run / handoff surface  — **L**
- **What:** Define a small workflow — pick an agent, hand off to another on
  completion, or fan out to parallel agents — and watch each as its own lane in
  the timeline (the subagent rendering is the seed). Start with sequential
  handoff (lowest risk), then parallel.
- **Why now:** differentiates from a plain chat UI and cashes the name; the
  forge already compiles distinct agent personas deterministically.
- **Touches:** `internal/ctxforge` (a workflow/handoff type + `Validate`),
  the seam (`Send`/session lifecycle for sub-runs), `internal/web` (lanes,
  a workflow page). Record the model as an ADR before building.

### 2.2 MCP server management page + curated defaults  — **M**
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

### 3.1 Diff review lane  — **M**
WEB_UI_PLAN UX principle #5 ("diffs get a review lane") is only partially met:
file edits render a diff inside the tool card, but approval is per-tool, not a
dedicated review affordance. A collapsible side-by-side/inline diff with the
approve/reject attached would make file-writing agents feel trustworthy.
Touches `render.go` (tool result rendering) + the permission form.

### 3.2 Per-session telemetry totals  — **S/M**  *(TECH_DEBT #2)*
Statusline credits/tokens are meter-global, not per-session. Scope a per-session
meter so the footer reflects *this* conversation. Pairs naturally with 1.3.

### 3.3 Keybinding surface  — **S**
`config.Config` already carries key bindings but they aren't surfaced or editable
in the web UI. A help overlay (`/help` exists as a note) + a Settings section
that reads/writes the existing schema. Low effort, good discoverability.

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
2. **2.2 (MCP page)** — closes the one missing forge CRUD; small and unblocking.
3. **3.2 + 3.1** — per-session totals and the diff lane; visible polish.
4. **2.1 (orchestration)** — the big bet; do it once 1.x has hardened the
   multi-run cost accounting it will lean on, and lead with an ADR.

Each item: write the failing test first, keep domain logic pure, run
`make lint && make test` (coverage floor 65%), and fold its ADR/CONTRACTS/
REGRESSIONS updates into the same feature branch (ADR-0004).
