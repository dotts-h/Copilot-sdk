# my-orchestra — Skills Program Plan

**Prepared for:** Horia · **Date:** 2026-06-04 · **Repo:** `Copilot-sdk` (module `github.com/dotts-h/copilot-sdk`, app *my-orchestra*)
**Purpose:** A validated, per-skill brief to feed the skill-creator. Built from (a) deep review of this project's sessions — the roadmap memory, `docs/ARCHITECTURE.md`, `docs/REGRESSIONS.md`, `docs/WEB_UI_PLAN.md`, git history — and (b) online research on skill authoring, ADRs, agentic Playwright QA, and modern UX/UI design skills.

---

## 0. Executive summary (phone-skimmable)

15 skills, in 4 groups. Each is scoped so triggers don't collide (the rubber-duck below maps every boundary).

| # | Skill (gerund slug) | Role | Placement |
|---|---------------------|------|-----------|
| 1 | `mapping-codebases` | Codebase map | Global → repo output |
| 2 | `registering-contracts` | Interface/contract registry | Global → repo output |
| 3 | `recording-decisions` | ADR / decision log | Global → repo output |
| 4 | `maintaining-conventions` | Constitution / conventions | Global → repo output |
| 5 | `logging-learnings` | Learnings + dead-ends log | Global → repo output |
| 6 | `practicing-tdd` | Test-first development | Global |
| 7 | `hardening-tests` | SDET — validate/harden the suite | Global |
| 8 | `authoring-tests` | Write e2e / api / perf / a11y tests | Global |
| 9 | `exploring-quality` | Exploratory QA (2 phases) | Global |
| 10 | `auditing-code-quality` | Patterns / antipatterns | Global |
| 11 | `improving-architecture` | Structural architecture work | Global |
| 12 | `managing-tech-debt` | Tech-debt register | Global |
| 13 | `designing-ui-ux` | Combined UX + UI design | Global |
| 14 | `tracking-issues` | Issues: hybrid local+GitHub, grouped | **Project** (`.claude/skills/`) |
| 15 | `governing-qa-framework` | Playwright framework health | **Project** (`.claude/skills/`) |

**Your 4 decisions, applied:** (1) 5 focused doc skills + shared `docs/` convention; (2) **hybrid** issue storage — local markdown source of truth mirrored to GitHub; (3) **one combined** design skill; (4) **split** placement — methodology global, project-bound skills committed in-repo.

**One placement nuance to confirm (§8):** your opening note said the doc skills should be *global*; the "Split" option text bucketed them as in-repo. I've recommended **global methodology with repo-side output** for the 5 doc skills (best of both — reusable on every project, docs always land in the repo). Flag if you'd rather commit them in-repo.

---

## 1. Why these, grounded in this project

This repo already practices most of what these skills encode — the skills *systematize and make repeatable* what the sessions did ad hoc:

- **TDD/SDET is already the culture.** `ARCHITECTURE.md#testing-philosophy-tdd--sdet`, fuzz target on pricing, 16×100 concurrency test, every SDK event→normalized-event mapping tested, coverage floor 65%. The skills turn this from tribal knowledge into invokable procedure.
- **A learnings log already exists** (`REGRESSIONS.md`) with the rule "*every fix names the test that guards it*." `logging-learnings` generalizes it (adds dead-ends/what-we-tried).
- **Contracts already exist but are scattered** — the `copilot.Client` seam (`ARCHITECTURE.md`), the SDK-event→normalized-event table, the SSE event→fragment table and routes (`WEB_UI_PLAN.md`). `registering-contracts` consolidates them into one stable-promises doc.
- **A Playwright suite already exists** (`e2e/`: e2e · api · a11y · ux · perf) plus a vendored Playwright skill in `node_modules`. `authoring-tests`, `exploring-quality`, and `governing-qa-framework` build directly on it.
- **The UI is "good but not great"** (your words). `designing-ui-ux` targets exactly the htmx/CSS surface in `internal/web/static/`.

---

## 2. Conventions every skill follows (from Anthropic's skill-authoring guide)

Bake these into each SKILL.md so the skill-creator output is consistent:

- **Frontmatter:** `name` (lowercase-hyphen, ≤64 chars, gerund), `description` (≤1024 chars, **third person**, states *what it does* **and** *when to use it*, includes trigger keywords). Optional `allowed-tools`.
- **Body ≤500 lines.** Push detail into `references/*.md` (one level deep — never reference-of-a-reference). Long reference files get a table-of-contents header.
- **Progressive disclosure:** SKILL.md is a table of contents; `references/` loaded on demand; `scripts/` executed (not read) for deterministic ops.
- **Read project facts, don't hardcode them.** Every methodology skill begins by reading `CLAUDE.md` + `docs/CONVENTIONS.md` for repo-specific values (Go toolchain `export PATH=$PATH:/home/ori913/go-install/go/bin`, `make lint/test/e2e`, coverage floor, branch/`--no-ff` flow). This is what makes a *global* skill adapt to *this* repo.
- **Workflows as checklists** the skill copies into its reply and ticks off.
- **Feedback loops:** run validator → fix → repeat (e.g. run tests, fix, re-run).
- **No time-sensitive text; consistent terminology; concrete examples; forward-slash paths.**
- **Build ≥3 evaluations per skill before writing prose** (eval-driven). I've drafted starter evals per skill below.

---

## 3. Shared knowledge-base layout (the 5 doc skills write here)

```
docs/
├── KNOWLEDGE.md            # index + cross-link map (the 5 doc skills keep this current)
├── CODEBASE_MAP.md         # ← mapping-codebases
├── CONTRACTS.md            # ← registering-contracts
├── CONVENTIONS.md          # ← maintaining-conventions (CLAUDE.md stays the short authoritative pointer)
├── adr/
│   ├── 0001-htmx-over-spa.md      # backfilled from WEB_UI_PLAN decisions
│   ├── 0002-hard-cut-tui.md
│   └── ...                         # ← recording-decisions (MADR-lite)
├── REGRESSIONS.md          # existing; ← logging-learnings extends into learnings + dead-ends
├── issues/                 # ← tracking-issues (local source of truth)
│   ├── INDEX.md
│   ├── assets/             # screenshots
│   └── NNNN-*.md
├── ARCHITECTURE.md         # existing deep-dive (kept; CODEBASE_MAP links to it)
└── WEB_UI_PLAN.md          # existing; decisions migrate into adr/
```

Cross-link rule: every artifact links *up* to KNOWLEDGE.md and *across* to the ADR/issue/regression it relates to. Isolated items may stand alone; connected items must link.

---

## 4. Per-skill specifications

> Format per skill: **Purpose · Placement · `description` draft · Workflow/body · Outputs · references/ · scripts/ · Boundary (rubber-duck) · Project grounding · Starter evals.**

### Group A — Documentation / knowledge (global methodology, repo-side output)

#### 1. `mapping-codebases`
- **Purpose:** Produce/refresh a navigable map of the codebase — modules, entry points, data-flow, the key seams, "where does X live."
- **`description` draft:** *"Generates and maintains a CODEBASE_MAP.md that documents module layout, entry points, data flow, and architectural seams. Use when onboarding to a repo, after structural changes, or when asked to map, document, or explain how the codebase is organized."*
- **Body/workflow:** detect language/build; enumerate packages with one-line responsibilities; trace the primary data path (here: `cmd → web/Hub → copilot.Client → SDKClient → Events() → convo reducer → SSE fragments`); identify seams/boundaries; mark "pure core vs thin edges"; link out to ARCHITECTURE.md for depth; update the module table on change.
- **Outputs:** `docs/CODEBASE_MAP.md` (+ KNOWLEDGE.md entry).
- **references/:** `map-template.md`, `tracing-data-flow.md`.
- **scripts/:** `module-inventory.sh` (lists packages + LOC + exported symbols, language-aware).
- **Boundary:** *map = the territory* (what exists/where). Contracts = the promises. Architecture skill = the judgement/changes. This one is descriptive, not prescriptive.
- **Grounding:** seed from the existing `ARCHITECTURE.md#module-map`.
- **Evals:** (a) fresh repo → correct module table; (b) after adding a package → table updated; (c) "where is pricing?" answerable from the map.

#### 2. `registering-contracts`
- **Purpose:** One registry of the stable promises between components — interfaces, event vocabularies, routes, schemas, invariants — so changes to them are deliberate.
- **`description` draft:** *"Generates and maintains a CONTRACTS.md registry of interfaces, event/message schemas, HTTP routes, and invariants between components. Use when adding or changing an API, interface, event type, or route, or when asked to document or audit contracts."*
- **Body/workflow:** extract interface signatures (`copilot.Client`), the SDK-event→normalized-`Event` table, the normalized-`Event`→SSE-fragment table, HTTP routes, SSE event names, config/forge JSON schemas, and invariants (determinism of `Forge.Compile`, pricing totality, escaping of all model text). Each entry: producer, consumer, shape, stability note. Flag drift when code and registry disagree.
- **Outputs:** `docs/CONTRACTS.md`.
- **references/:** `contract-entry-template.md`, `detecting-drift.md`.
- **scripts/:** `extract-interfaces.sh` (grep/AST for `type X interface`, route registrations, SSE `event:` names).
- **Boundary:** consolidates tables currently split across ARCHITECTURE/WEB_UI_PLAN. Doesn't *change* contracts (that's ADR + architecture); it records and guards them.
- **Grounding:** the three tables already exist — this is mostly consolidation + a drift check.
- **Evals:** (a) lists all `copilot.Client` methods; (b) every SSE event name in code appears; (c) a renamed route is flagged as drift.

#### 3. `recording-decisions` (ADRs)
- **Purpose:** Capture each significant decision and its rationale as a numbered, immutable record.
- **`description` draft:** *"Creates and maintains Architecture Decision Records (MADR-lite: context, decision, options, consequences) under docs/adr/. Use when a non-trivial technical decision is made or reversed, or when asked to record, revisit, or supersede a decision."*
- **Body/workflow:** MADR-lite template (Context · Considered options · Decision · Consequences · Status). One decision per file, `NNNN-kebab-title.md`, monotonic numbering, `Status: proposed|accepted|superseded-by-NNNN`. Never edit an accepted ADR's decision — supersede it. Maintain `docs/adr/README.md` index. Link from/to contracts, codebase-map, issues.
- **Outputs:** `docs/adr/NNNN-*.md` + index.
- **references/:** `madr-template.md`, `nygard-vs-madr.md`.
- **scripts/:** `new-adr.sh <title>` (allocates next number, stamps template).
- **Boundary:** ADRs = *why we chose*. Conventions = *the rules we now follow*. Learnings = *what we tried that failed*. An ADR may spawn a convention; a dead-end may spawn an ADR.
- **Grounding:** backfill the obvious ones first — htmx-over-SPA, hard-cut-TUI, generic `bridge[T]`, cookie-keyed multi-session, Go-as-language (all already argued in the memory/docs).
- **Evals:** (a) "record the htmx decision" → valid MADR file + index update; (b) superseding sets both files' status; (c) numbering never collides.

#### 4. `maintaining-conventions` (constitution)
- **Purpose:** The living rulebook — coding standards, workflow, gates, and project facts the other skills read.
- **`description` draft:** *"Maintains CONVENTIONS.md — the project's coding standards, workflow, quality gates, and environment facts (build commands, toolchain paths, coverage floors). Use when establishing or changing a team convention, or when a skill needs the canonical project rules."*
- **Body/workflow:** sections for Architecture rules (no SDK imports outside `SDKClient`; pure domain logic; determinism; atomic persistence), Workflow (branch from main, failing-test-first, `--no-ff`, push, delete branch), Gates (`make lint`/`make test` -race + coverage floor 65%, fuzz smoke), Environment facts (Go PATH, make targets). Keep `CLAUDE.md` as the short authoritative pointer that links here.
- **Outputs:** `docs/CONVENTIONS.md` (+ keep CLAUDE.md in sync).
- **references/:** `convention-categories.md`.
- **Boundary:** Conventions = enforceable *rules now in force*. ADRs = the *decisions* that justify them (link rule→ADR). Code-quality skill *applies* conventions to code; it doesn't define them.
- **Grounding:** harvest directly from `CONTRIBUTING.md` + the memory's "Convention:" lines + Makefile.
- **Evals:** (a) a TDD run can read the gate commands from here; (b) adding a rule links to its ADR; (c) CLAUDE.md stays a thin pointer.

#### 5. `logging-learnings`
- **Purpose:** A running record of bugs fixed, things tried, and dead-ends — so we don't relearn or re-break.
- **`description` draft:** *"Maintains a learnings + dead-ends log (extends REGRESSIONS.md): fixed bugs with their guarding test, approaches that failed and why, and gotchas. Use after fixing a bug, hitting a dead-end, or discovering a non-obvious gotcha."*
- **Body/workflow:** two registers — *Fixed* (symptom · root cause · fix · **guarding test**, keeping the existing rule) and *Dead-ends* (what we tried · why it failed · what to do instead). Plus "Testing gotchas." Enforce: a fix without a guard goes to "Known gaps."
- **Outputs:** extends `docs/REGRESSIONS.md` (rename/section, your call) + KNOWLEDGE.md.
- **references/:** `entry-templates.md`.
- **Boundary:** Learnings = *empirical history* (what happened). Tech-debt = *open obligations* (what we still owe). A known gap may become a tech-debt item.
- **Grounding:** the file already exists and is high quality — this skill just keeps it fed and adds the dead-ends register.
- **Evals:** (a) a fix with no test → lands in Known gaps; (b) a dead-end is recorded with the "instead"; (c) entry cross-links its ADR/issue.

### Group B — Engineering process (global)

#### 6. `practicing-tdd`
- **Purpose:** Drive feature/bug work test-first through the project's red-green-refactor loop.
- **`description` draft:** *"Drives development test-first: write a failing test, make it pass, refactor, run the gates. Use when implementing a feature, fixing a bug, or changing behavior in a tested codebase."*
- **Body/workflow (checklist):** read CONVENTIONS for gates → write the smallest failing test (unit via `MockClient`/table-driven where it fits the seam) → run, see red → minimal code → green → refactor → `make lint && make test` (race + coverage) → if a bug, hand the guard to `logging-learnings` → branch/`--no-ff`/push.
- **references/:** `red-green-refactor.md`, `testing-through-the-seam.md` (MockClient patterns), `go-table-tests.md`.
- **Boundary:** TDD *writes the unit/feature test that drives code*. `authoring-tests` writes the *browser/api/perf* layers. `hardening-tests` *audits* whatever exists. No overlap if TDD stays at the unit/seam level.
- **Grounding:** the seam + MockClient design is purpose-built for this (`ARCHITECTURE.md#the-copilotclient-seam`).
- **Evals:** (a) feature request → test precedes code in the transcript; (b) gates run before "done"; (c) bug fix produces a guard entry.

#### 7. `hardening-tests` (SDET)
- **Purpose:** Verify, validate, and harden the *test suite itself* — find weak assertions, coverage gaps, flakes, non-determinism.
- **`description` draft:** *"Audits and hardens an existing test suite: finds coverage gaps, weak assertions, flaky and non-deterministic tests, and missing edge/property/fuzz cases, then strengthens them. Use when asked to review, harden, or improve test quality, or before relying on a suite."*
- **Body/workflow:** coverage-gap analysis (`go test -coverprofile`); assertion-strength review (does the test fail if behavior changes?); **mutation testing** (`gremlins`/`go-mutesting`) to find tests that pass against broken code; flake hunt (`go test -count=20 -race`); determinism checks (ordering, time, concurrency — extend the meter's 16×100 pattern); ensure each fixed bug has a guard (cross-check REGRESSIONS).
- **references/:** `mutation-testing.md`, `flake-hunting.md`, `assertion-strength.md`, `property-and-fuzz.md`.
- **scripts/:** `mutation-run.sh`, `flake-hunt.sh <pkg> <n>`.
- **Boundary:** TDD/authoring *create* tests; SDET *attacks and strengthens* them. SDET never adds product code.
- **Grounding:** project already has fuzz + concurrency + contract tests — SDET makes that a routine pass, not a one-off.
- **Evals:** (a) introduce a silent mutant → SDET surfaces an unguarded path; (b) a sleep-based flake is found and fixed to web-first/poll; (c) coverage gap on `convo` reported.

#### 8. `authoring-tests`
- **Purpose:** Write, validate, and enhance the integration/e2e/api/perf/a11y test layers.
- **`description` draft:** *"Writes and enhances end-to-end, API/contract, performance, and accessibility tests (Playwright + Go contract/bench). Use when adding or extending e2e, api, perf, or a11y coverage, or when a feature needs browser/integration tests."*
- **Body/workflow:** pick the layer (e2e · api · a11y · ux · perf, matching `e2e/`); for browser tests follow **planner → generator → healer** (explore → spec in `specs/` → generate into `e2e/tests/` → heal to green) using role/test-id locators and web-first assertions; for api use the `api_test.go` contract style; for perf use `bench_test.go` + the Playwright perf layer; respect the demo's shared-session gotcha (relative assertions). Hands hardening to `hardening-tests`.
- **references/:** `playwright-planner-generator-healer.md`, `api-contract-tests.md`, `perf-benchmarks.md`, `a11y-axe.md`, `demo-gotchas.md`.
- **scripts/:** `init-agents.sh` (`npx playwright init-agents --loop=claude`), `run-layer.sh <e2e|api|a11y|ux|perf>`.
- **Boundary:** *creates the higher test layers*; TDD owns unit/seam; SDET hardens; `governing-qa-framework` owns the *config/standards* of the harness (not individual tests).
- **Grounding:** mirrors the existing 5-layer split and the bundled Playwright planner/generator/healer agents.
- **Evals:** (a) new page → e2e spec+test green via the agent loop; (b) new route → api contract test; (c) a11y layer catches an injected contrast regression.

#### 9. `exploring-quality` (2-phase exploratory QA)
- **Purpose:** Find the bugs nobody wrote a test for — breadth first, then depth.
- **`description` draft:** *"Runs exploratory QA in two phases — phase 1 generates and runs many breadth probing scripts from the codebase (route/HTTP sweeps, smoke flows, input fuzzing) headless; phase 2 deep-dives the live app via the Playwright browser (MCP/CLI) following curiosity. Produces a findings report. Use for exploratory testing, bug hunts, or pre-release sweeps."*
- **Body/workflow:**
  - **Phase 1 (breadth, scripted):** derive a surface inventory from the code (routes from `internal/web`, SSE events, forms, slash-commands), generate many small probing scripts (curl/HTTP status sweeps, malformed-payload tolerance, rapid-fire SSE, slash-command fuzz), run headless, collect anomalies.
  - **Phase 2 (depth, browser):** launch the demo server, drive the real browser via `playwright-cli`/Playwright-MCP (accessibility-tree snapshots, not pixels), follow leads from phase 1, exercise streaming/queueing/permissions/plan-mode/elicitation, run axe-core, capture screenshots for anything visual.
  - **Output:** a ranked findings report → feeds `tracking-issues` (with screenshots) and `authoring-tests` (regressions to lock in).
- **references/:** `phase1-breadth-scripts.md`, `phase2-browser-deepdive.md`, `playwright-cli-cheatsheet.md` (from the vendored skill), `findings-report-template.md`.
- **scripts/:** `surface-inventory.sh`, `breadth-sweep.sh`, `launch-demo.sh` (`./my-orchestra -demo`).
- **Boundary:** exploratory *discovers*; `authoring-tests` *codifies* the keepers; `tracking-issues` *files* them. Exploratory writes no permanent tests itself.
- **Grounding:** the demo server + vendored `playwright-cli` skill + `api_test.go` probing style already exist; sandbox can't bind servers, so phase-1/2 run locally or in CI.
- **Evals:** (a) phase 1 flags a 500/odd status on a route; (b) phase 2 reproduces it in-browser with a screenshot; (c) a finding becomes a filed issue + a new e2e test.

#### 10. `auditing-code-quality`
- **Purpose:** Apply a patterns/antipatterns standard to code — readability, simplicity, idiom, the project's "pure core / thin edges."
- **`description` draft:** *"Reviews code against a patterns/antipatterns catalog (Go idioms + this project's seam discipline, pure-core/thin-edges, error handling, naming) and proposes focused cleanups. Use when reviewing code quality, refactoring for clarity, or enforcing coding standards — complements the built-in /code-review and /simplify."*
- **Body/workflow:** run against a diff or package; check the catalog (no SDK leakage past `SDKClient`; dependency-free domain logic; error surfacing not punting; consistent naming; dead-code; golangci-lint cleanliness); propose minimal diffs; defer bug-hunting to `/code-review`.
- **references/:** `go-patterns.md`, `antipatterns-catalog.md`, `project-idioms.md`.
- **Boundary:** **Explicitly complementary to the built-in `/code-review` (bug-finding) and `/simplify` (mechanical cleanup).** This skill is the *standards reference + opinionated style pass*. Architecture skill handles *structure*; this one handles *line/function level*.
- **Grounding:** `.golangci.yml` + CONTRIBUTING conventions are the seed catalog.
- **Evals:** (a) SDK import in `internal/web` → flagged; (b) a punted error → flagged with the surface-it fix; (c) no overlap-noise with /simplify on a clean diff.

#### 11. `improving-architecture`
- **Purpose:** Evaluate and improve structure — boundaries, coupling, dependency direction, seam integrity — and route changes through ADRs.
- **`description` draft:** *"Assesses architecture for module boundaries, coupling, dependency direction, and drift from stated design goals, then proposes structural improvements as ADRs. Use when evaluating or refactoring architecture, resolving cross-cutting coupling, or planning a structural change."*
- **Body/workflow:** read CODEBASE_MAP + CONTRACTS + design goals; check dependency direction (UI never imports SDK; domain stays pure); find drift (e.g. docs say "single in-memory session" but `Hub` is now multi-session — a known doc-vs-code gap); propose refactors sized as ADRs; never silently restructure — record the decision.
- **references/:** `dependency-rules.md`, `coupling-smells.md`, `refactor-as-adr.md`.
- **Boundary:** *strategic/structural*; `auditing-code-quality` is *tactical/line-level*; `managing-tech-debt` *tracks* what this finds. Architecture proposes → ADR records → tech-debt schedules.
- **Grounding:** the `copilot.Client` seam is the central invariant; the multi-session/doc drift is a ready first target.
- **Evals:** (a) detects the ARCHITECTURE.md "single session" drift; (b) a proposed refactor emits an ADR; (c) flags any new SDK import past the seam.

#### 12. `managing-tech-debt`
- **Purpose:** Maintain a prioritized ledger of known shortcuts and their cost/interest.
- **`description` draft:** *"Maintains a tech-debt register (TECH_DEBT.md): catalogs shortcuts and gaps with severity, effort, and interest, prioritizes them, and links each to its ADR/issue. Use when recording, prioritizing, or planning paydown of technical debt."*
- **Body/workflow:** each item — description, location, severity, effort, "interest" (cost of leaving it), linked ADR/issue, suggested trigger to pay it down. Prioritize (e.g. interest×likelihood). Pull candidates from `logging-learnings` "Known gaps", `improving-architecture` findings, and the roadmap's deferred items.
- **Outputs:** `docs/TECH_DEBT.md`.
- **references/:** `debt-register-template.md`, `prioritization.md`.
- **Boundary:** debt = *open, prioritized obligations*. Learnings = *closed history*. Issues = *actionable units of work* (a debt item links to its issue when scheduled).
- **Grounding:** ready-made backlog: markdown rendering, editable settings, session pick/continue, statusline per-session totals, multi-session doc drift (all in the memory).
- **Evals:** (a) a "Known gap" becomes a ranked debt item; (b) item links its ADR; (c) re-prioritization is stable/deterministic.

#### 13. `designing-ui-ux` (combined)
- **Purpose:** Make the interface modern and excellent — UX (heuristics, flows, IA, a11y) and UI (visual hierarchy, tokens, typography, motion) — and implement + verify the changes.
- **`description` draft:** *"Audits and improves interface UX and UI: applies Nielsen/Krug heuristics, information architecture, and WCAG 2.2 for UX; visual hierarchy, design tokens, typography, spacing, and motion (Refactoring UI) for UI. Can implement changes in the app's HTML/CSS and verify them in the browser. Use when improving look, feel, usability, or accessibility."*
- **Body/workflow:** **Audit** (Nielsen's 10 + Krug "don't make me think"; severity-scored issues; axe-core a11y; visual-hierarchy/spacing/contrast/typography review — Refactoring-UI principles; ban generic system fonts where a brand voice is wanted) → **Design** (consolidate a design-token set: color/spacing/type scale/radii/shadows; persist to a `DESIGN.md` + CSS custom properties so choices don't drift) → **Implement** (edit `internal/web/static/app.css` + templates/fragments; keep htmx server-rendered, no framework) → **Verify** (browser via `playwright-cli`/MCP, axe-core, responsive widths; screenshot before/after) → feed issues/regressions.
- **references/:** `ux-heuristics.md` (Nielsen+Krug+severity), `wcag-2.2-pour.md`, `refactoring-ui.md`, `design-tokens.md`, `htmx-implementation-notes.md`.
- **scripts/:** `axe-scan.sh`, `screenshot-states.sh` (drives demo across pages/widths).
- **Boundary:** combined per your choice. Shares the browser-driver with `exploring-quality` (which *finds* problems) and the a11y layer with `authoring-tests` (which *locks in* fixes); this skill *designs and implements* the improvement. Persisted tokens live in `DESIGN.md` (not CONVENTIONS).
- **Grounding:** existing palette (terracotta `#d98c5f`, copilot blue `#6ea8fe`, slate), `app.css`, the a11y baseline already in `e2e/tests/a11y.spec.ts`; known UI gaps: markdown rendering, statusline density, responsive topbar.
- **Evals:** (a) audit returns severity-scored heuristic findings; (b) a contrast fix passes axe + keeps the test green; (c) tokens persisted and reused, not re-invented.

### Group C — Cross-cutting (project-committed)

#### 14. `tracking-issues` (hybrid)
- **Purpose:** File, group, and connect issues without losing context — local source of truth, mirrored to GitHub, with screenshots and cross-links.
- **`description` draft:** *"Files and links issues using a hybrid store — markdown under docs/issues/ as source of truth, mirrored to GitHub Issues via gh. Groups related issues under epics/labels, attaches screenshots, and cross-links to ADRs, PRs, and the learnings log. Use when capturing a bug/task, grouping related work, or linking an issue to a decision."*
- **Body/workflow:** local `docs/issues/NNNN-title.md` with frontmatter (`id, title, status, severity, group/epic, links: {adr, pr, regression, issues[]}, assets[]`); screenshots in `docs/issues/assets/`; **isolated** issues stand alone, **connected** issues set `group:` and back-link the epic file; mirror to GitHub via `gh issue create/edit` (labels = group, body embeds the markdown + image links, task-list links sub-issues); keep `docs/issues/INDEX.md`. Two-way: a finding from `exploring-quality` lands here with its screenshot.
- **references/:** `issue-template.md`, `grouping-and-epics.md`, `gh-sync.md`.
- **scripts/:** `new-issue.sh <title>`, `sync-github.sh` (push/pull state), `attach-shot.sh`.
- **Placement:** **project** (`.claude/skills/`) — it writes into this repo's `docs/issues/` and uses this repo's `gh` remote.
- **Boundary:** issues = *actionable units*. ADRs = decisions (link). Tech-debt = the ledger (a debt item may spawn an issue). Learnings = history (a fixed issue updates REGRESSIONS).
- **Grounding:** repo already uses GitHub PRs (#11–15) + `gh`; design goal #3 ("context is reproducible") favors the local-markdown source of truth.
- **Evals:** (a) new issue creates local file + GitHub issue with matching id link; (b) grouping links epic both ways; (c) screenshot renders in both places.

#### 15. `governing-qa-framework`
- **Purpose:** Keep the Playwright/test framework itself at standard — config, fixtures, locator strategy, CI wiring, agent setup.
- **`description` draft:** *"Audits and upgrades the Playwright test framework against current best practices: config (projects, reporters, trace/video, retries), fixtures, role/test-id locator strategy, web-first assertions, no hard waits, parallelization, CI wiring, and planner/generator/healer agent setup. Use when setting up, auditing, or modernizing the test framework (not when writing individual tests)."*
- **Body/workflow:** audit `e2e/playwright.config.ts` (projects, reporter, `trace: 'on-first-retry'`, retries, `workers`/`fullyParallel` — noting the demo's single-shared-session constraint), fixtures/helpers (`e2e/tests/helpers.ts`), locator discipline (role/test-id, web-first `expect`, ban `waitForTimeout`), CI (`.github/workflows/e2e.yml`, `make e2e`), and the agent toolchain (`npx playwright init-agents --loop=claude`, regenerate on PW upgrade). Output: a framework-health report + fixes.
- **references/:** `playwright-config-standards.md`, `locator-strategy.md`, `ci-integration.md`, `agents-setup.md`.
- **scripts/:** `framework-audit.sh` (greps for `waitForTimeout`, missing trace, etc.).
- **Placement:** **project** (`.claude/skills/`) — it's bound to this repo's `e2e/` and CI.
- **Boundary:** owns the *harness/standards*; `authoring-tests` owns *the tests*; `exploring-quality` *drives the browser ad hoc*. Framework skill rarely writes a test — it writes config/fixtures/CI.
- **Grounding:** `e2e/` already split into 5 layers with axe-core, trace artifacts, and `e2e.yml` — this keeps it modern (the bundled PW version already ships the planner/generator/healer agents).
- **Evals:** (a) flags a `waitForTimeout`; (b) verifies trace-on-retry + reporter set; (c) confirms `init-agents` present and current.

---

## 5. Overlap / boundary matrix (the rubber-duck, consolidated)

The biggest risk with 15 skills is muddy triggers. Each pair below is deliberately separated:

- **TDD vs authoring-tests vs SDET vs exploring-quality:** *write unit/seam test-first* / *write e2e·api·perf·a11y layers* / *attack & harden any suite* / *find untested bugs (no permanent tests)*.
- **code-quality vs architecture vs tech-debt:** *line/function patterns* / *structure & boundaries* / *the prioritized ledger of what's owed*.
- **code-quality vs built-in /code-review & /simplify:** this skill is the *standards catalog + style pass*; `/code-review` finds *bugs*; `/simplify` does *mechanical cleanup*. Explicitly complementary.
- **map vs contracts vs architecture:** *territory* / *promises* / *judgement+change*.
- **ADR vs conventions vs learnings vs tech-debt:** *why we chose* / *rules now in force* / *what we tried/failed* / *open obligations*.
- **issues vs everything:** issues are the *actionable units* others link to; screenshots + grouping live here.
- **designing-ui-ux vs exploring-quality vs authoring-tests(a11y):** *design+implement* / *discover* / *lock-in*. They share the browser driver + axe-core but trigger on different intents.
- **qa-framework vs authoring-tests:** *the harness/standards* vs *the tests in it*.

---

## 6. Recommended build order

1. **Foundations first (they feed the rest):** `maintaining-conventions`, `mapping-codebases`, `registering-contracts`, `recording-decisions`, `logging-learnings`. Backfill the obvious ADRs and the CONTRACTS consolidation immediately — every other skill reads these.
2. **Core loop:** `practicing-tdd`, then `authoring-tests`, then `hardening-tests`.
3. **Discovery + tracking:** `exploring-quality` + `tracking-issues` (they pair).
4. **Standards & structure:** `auditing-code-quality`, `improving-architecture`, `managing-tech-debt`.
5. **Surface quality:** `governing-qa-framework`, `designing-ui-ux`.

---

## 7. Feeding the skill-creator

For each skill, hand the creator: the **name**, the **`description` draft** (verbatim — it's tuned for triggering), the **body/workflow outline**, the **references/ list**, and the **scripts/ list** above. Then, per Anthropic's guidance: write the **≥3 evals first**, keep the body **≤500 lines**, references **one level deep**, descriptions **third-person with what+when**, and have each methodology skill **open by reading `CLAUDE.md` + `docs/CONVENTIONS.md`** so the global skill adapts to this repo. Test each with Opus + Sonnet (and Haiku if you'll run it there).

---

## 8. Open decision to confirm on phone

**Doc-skill placement.** You said in your brief the doc skills should be *global*; the "Split" option I offered bucketed them as in-repo. My recommendation (above) is **global methodology, repo-side output** for skills 1–5 — reusable on every project, while the generated docs always live in this repo. If you'd rather *commit the doc skills themselves* into `.claude/skills/` (shareable with collaborators via git, not reused elsewhere), say so and I'll flip skills 1–5 to project placement. Everything else follows your four answers exactly.

---

## 9. Operational skills shipped in-repo (addendum, 2026-06-07)

Beyond the 15 methodology skills above, two **operational** skills now live in
`.claude/skills/` (committed in-repo, promotable to the `claude-skills` plugin for
cross-project reuse) — they encode lessons from a build/infra session:

| Skill | Role | Origin lesson |
|-------|------|---------------|
| `cut-release` | Publish a tagged release with end-to-end version verification | A dispatched release published mis-tagged `main` because the workflow's version step shadowed the input — verify outward-facing actions, don't fire blind. |
| `seed-project-infra` | Seed a new repo with the portable kit (single-run CI, the workflow guard, a correct release workflow, the doc skeleton, the doctrine) | Conventions that live only as prose drift and don't travel; make the defaults executable + portable. |

These pair with the **self-enforcing workflow guard** (`scripts/check-workflows.sh`, wired
into CI lint + `make lint`) — the "enforce with hooks, not memory" principle in code. Next
operational skills to spec: `pick-next-child` (roadmap → next issue+branch, replacing a
hand-carried planning prompt) and `deepen-module` (the deep/shallow + deletion-test lens).

---

— End of plan —
