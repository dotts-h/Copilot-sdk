# Retro 0005 — deep quality, architecture & test hardening (the v0.2→v0.7 arc)

- Date: 2026-06-11
- Scope: a retrospective on the whole stretch since [RETROS 0001](0001-quality-and-architecture-hardening.md)
  (which closed the v1 run) — **epics 0013 → 0083**, roadmap **v2 → v14**, releases
  **v0.2.0 → v0.7.0** (~PRs #36–#153). Cost-awareness deepening, the full orchestration
  suite (real parallel lanes, conditional branching, run history, cost⋈run reconciliation,
  rerun/abort), the UI/UX refresh + the playful-motion overhaul, billing fidelity, auth &
  connection, safe-autopilot governance/hooks, first-class sub-agents, designed agent
  output, and orchestration robustness.
- Method: three **scoped, read-only** audits (backend quality/architecture · frontend ·
  test-gap), each grounded in CONVENTIONS invariants + the REGRESSIONS "do-not-touch"
  list (so intentional patterns weren't re-flagged), then a behavior-preserving hardening
  pass. This mirrors RETROS 0001's audit→hardening shape, with A2's scoping lesson applied.

## Context

The codebase entered this review **healthy**: suite green, coverage **88.4%** (floor 65%),
278 test files vs 203 source files. The audits confirmed the architecture is sound — the
`copilot.Client` seam holds (no SDK type escapes into `web`/`telemetry`), domain packages
(`telemetry`/`ctxforge`/`config`) are pure & deterministic, the `forgeMu → s.mu` lock order
is consistent and documented, the three-source spend accounting (session meter / process
meter / ledger) is maintained by one `recordUsage`, and **every** REGRESSIONS entry has a
guard. So this retro is not a rescue; it's compounding maintenance on a strong base.

## What worked (leave as-is)

- **The docs-as-operating-system scaled to 49 ADRs without rotting.** Each audit sub-agent
  grounded findings in named invariants and the REGRESSIONS "do-not-touch" notes correctly
  pre-empted false positives (`isHR` RE2-no-backreferences, the `event.target===this`
  composer guard, the deliberate non-`close(c.events)`, the three-meter split, the
  `trusted()`-wraps-pre-escaped-fragments idiom, the alpha-composite `--hover/--raised`
  tokens outside the OKLCH guard). The frontend audit's headline: most flagged "XSS" paths
  resolved to **safe** because escape-first (ADR-0001) is enforced uniformly.
- **The automated guards earn their keep.** `css_tokens_test.go` computes real OKLCH→sRGB
  AA contrast at `go test` time; `FuzzRenderMarkdown` *found* the NUL-leak (REGRESSIONS #22)
  during the block-AST refactor; the workflow guard blocks the two CI regressions. These
  caught classes of bug the eye never would.
- **The retro→action loop compounded.** RETROS 0001's CODEMAP (A1), scoped-audit (A2), and
  pre-push `/code-review` (A3) were all used here and all paid off — the map made windowed
  reads cheap, scoping kept three audits to ~140–160k tokens each (not the 364k of 0001's
  unscoped pair), and the established `editConfig` rollback pattern was already there to fix
  *against*.
- **Coverage rose by construction, not assertion** (this pass: bootstrap 69→74, convo
  85→87, copilot 72→74; total 88.4→88.6) — every percentage point is a named guard test.

## What we improved this pass (deep dive → fixes)

**Backend — correctness.** `handleAgentSelect`/`handleAgentDelete` mutated `s.config`
directly and only *logged* a `Save` failure, leaving the live config drifted from disk —
the exact anti-pattern REGRESSIONS already warns against ("edit through `Server.editConfig`").
Both now route through `editConfig` (snapshot → mutate → validating Save → roll back on
failure). Surfacing it exposed that `newTestServer()` built an **empty-dir** config, so its
`Save()` *always* failed silently — the persistence tests were implicitly asserting the
buggy non-rollback behavior. Fixed the harness to use a writable temp dir (so `Save`
succeeds on the happy path) and added `TestAgentSelectRollsBackOnSaveFailure` (sabotages the
dir so `Save` genuinely fails, asserts no drift).

**Frontend — accessibility.** A cluster of sub-agent surfaces (`.subagent-model`,
`.sa-activity/.sa-detail`, `.sa-credits` ×2, `.sa-overlay-desc`, `.sa-t-reasoning`,
`.sa-tool-args`, `.sa-empty`) dimmed text via `opacity` — the **identical** failure mode
that dropped `.elicit-desc` and the spend badge below 4.5:1 (REGRESSIONS #20/#21), and the
sub-agent overlay was never axe-scanned. Re-tokenized to `color: var(--dim)` (AA-safe,
full opacity), swapped a hardcoded `rgba(0,0,0,.5)` backdrop for `var(--scrim)`, gave the
ask/plan freeform inputs an `aria-label` (placeholder ≠ label, WCAG 1.3.1/4.1.2), and added
an a11y spec that opens the `<dialog>` overlay and scans it in both themes.

**Tests — categories deepened.** Dark branches/paths now guarded: the SDK→normalized
mapping (`CacheWriteTokens` was missing from the "every category" assertion despite being a
prior bug — REGRESSIONS #3; all four `planChangeText` branches; both `compactionSummary`
and `toolResultText` nil branches; `summarizeArgs` non-map fallback); the four interactive
handler **shutdown** (`<-c.done`) paths (previously every test resolved the gate — the
graceful-cancel lifecycle was unguarded); a bridge `begin/resolve` **concurrency** test
under `-race`; `convo.AddDecision` (zero prior coverage despite committing pending
state); and `bootstrap.seedRuns` / `Build` error-path / `DefaultConfigDir`.

## What's still open (tracked, deliberately deferred)

Kept out of this pass to keep the diff small and reviewable; recorded so they're not lost:

- **Two god files** — `workflow.go` (1222 LOC, 5 responsibilities) and `server.go` (1006).
  Behavior-preserving same-package splits along the seams the backend audit named →
  **TECH_DEBT #17**.
- **`normalize.go` map leak** — `toolNames`/`toolMeta` orphan on a mid-tool error (bounded
  to process lifetime) → **TECH_DEBT #18**.
- **`renderStatline`/`inline()` extraction** — both are long multi-field functions; pure
  cleanups with no behavior change. Lower priority than the splits.
- **CommonMark gaps** — bold/italic inside link text doesn't render; no tilde-fence
  (`~~~`) support. Low value for a localhost agent UI; documented in the renderer.
- **Doc drift fixed in passing:** epic **0069** was `status: open` with empty `links`
  while all six children, the INDEX, and ADRs 0040–0044 said shipped. Closed it and
  populated its links/checkboxes. (The lone drift across 70 issues — the INDEX-regen
  discipline is otherwise holding.)

## Skill / tool usage

- **Scoped parallel sub-agents (sonnet) for the audits and the mechanical test authoring** —
  the A2 lesson applied: named files + named concerns kept each audit proportionate, and the
  three ran concurrently. The test-authoring agent was scoped to non-`web` packages so it
  couldn't conflict with the concurrent backend/frontend edits.
- **Empirical verification over assumption** — ran the full `internal/web` suite after the
  harness change to catch ripple (none), and re-derived the AA fix from the project's own
  `--dim` precedent rather than eyeballing. The new e2e a11y test mirrors an existing
  working overlay-open spec exactly (Playwright isn't runnable in this sandbox — RETROS
  0004 — so an unverifiable e2e change must be a faithful copy of a proven pattern).
- **`/code-review` on the diff before push** (A3) — applied to this hardening diff.

## Action items

- **A1 — pay down the god-file split next time `workflow.go`/`server.go` is touched** (not
  speculatively). Seams are pre-named in TECH_DEBT #17 so the split is mechanical when the
  trigger fires. *(no code change this pass — the record is the commitment.)*
- **A2 — when fixing an error path, check the test harness isn't asserting the bug.** The
  `editConfig` fix only surfaced because the empty-dir harness made `Save` always fail; the
  green suite was encoding the dirty-config behavior. Generalizes RETROS 0002's "a review
  finding against the motivating example is a bug, not a footnote." *(this retro is the
  record.)*
- **A3 — the `opacity`-dims-contrast trap has now bitten four times** (REGRESSIONS #10/#20/
  #21 + this pass's sub-agent cluster). ✅ Folded into CONVENTIONS' naming/style note: dim
  text with the `--dim` token, **never** `opacity`; any new hidden-until-opened surface
  (`<dialog>`, overlay) gets a both-theme axe scan the day it lands.

## Session-optimization checklist (carry forward)

1. Map-first, windowed reads; scope audits (named files + concerns), run them in parallel.
2. Delegate mechanical test authoring to a scoped sonnet agent on non-overlapping packages.
3. When an error-path fix breaks green tests, suspect the harness encodes the bug — fix the
   harness, then guard the real failure path.
4. Dim text with `--dim`, never `opacity`; scan every open-on-demand surface in both themes.
5. An unverifiable e2e change (no browser in-sandbox) must be a faithful copy of a proven
   spec, not a fresh invention.
6. `/code-review` (always) + full `make lint && make test` before push; package-scoped in
   the loop.
