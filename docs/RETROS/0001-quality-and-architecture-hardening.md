# Retro 0001 — quality & architecture hardening

- Date: 2026-06-06
- Scope: the v1 feature run (items 1.1–3.4, PRs #28–#34) → roadmap-v2 research
  (PR #35) → the two hardening PRs (#36 quality/architecture, #37 tests).
- First retro (no prior baseline).

## Context

After the validated v1 backlog shipped and closed (epics 0001/0005/0007), a
research pass produced roadmap v2 (epic 0013, ADR-0016) and two **behavior-
preserving** hardening PRs: targeted refactors from two read-only audits (#36)
and test-gap fills + a Playwright framework pass (#37). Both merged green.

## What worked

- **The docs-as-operating-system.** CONVENTIONS + CONTRACTS + REGRESSIONS + ADRs
  + TECH_DEBT + issues are load-bearing, not decoration. The audit sub-agents
  grounded findings in named invariants; the REGRESSIONS **"do NOT touch"** list
  stopped "fixes" of intentional patterns (the deliberate non-`close(c.events)`,
  RE2-no-backreferences `isHR`, the two-meter recording); ADR-0009 had already
  pre-written the A1 follow-up. Decision-first (ADR-0016 before building) loads
  the next session.
- **Parallel, read-only audits.** Code-quality + architecture run concurrently
  gave two independent, corroborating passes — and disagreed usefully
  (`recordUsage`: quality said "do it," architecture said "propose-only," so it
  was deferred into A1).
- **Tight loop + PR hygiene.** Per-logical-unit commits, `make lint && make test`
  between steps, package-scoped tests during the loop, a two-PR split (quality vs
  tests) for reviewability, subscribe→timer-poll→merge-on-green→unsubscribe with
  no Bash `sleep`.
- **Coverage rose rather than being asserted on faith** (web 84→87, copilot
  55→66, telemetry 91→96, convo 96→98, cmd 26→31).

## What cost too much

- **Full-file reads for discovery.** Two cases: *edit reads* of files being
  largely rewritten (`sdkclient.go` 947, `forge.go` 524) were defensible;
  *discovery reads* (opening files just to locate a few symbols, and re-reading
  large docs) were the avoidable sink. → **Action A1.**
- **Audit spend.** The two audit sub-agents reported ~178k + ~186k ≈ **364k**
  subagent tokens combined. Valuable, but for a codebase already known healthy a
  *scoped* audit (named files + named concerns, "medium" breadth) would deliver
  most of the value for less. → **Action A2.**
- **No committed codebase map**, so structure is re-derived by reading every
  session — a cost paid forever until amortized into an artifact. → **Action A1.**
- **Skipped self-review on the diff.** Audited *before* coding (good) but never
  ran `code-review`/`simplify` on the *resulting* diff before pushing. → **Action A3.**

## Skill / tool usage

- `tracking-issues` ✓ (epic 0013 + issues 0014/0015), `governing-qa-framework` ✓
  (audit green; added the seed spec). `deep-research` correctly **not** used
  (codebase-internal, not web fan-out). Methodology audits were stood in by
  general sub-agents (the invocable list lacks them), which correctly read the
  docs first. **Miss:** `code-review`/`simplify` on the diff pre-push, and
  `verify`/`run` for UI features (didn't apply to refactors; will for A1/B1).

## Action items

- **A1 — codebase map (DONE):** `docs/CODEMAP.md` + `make codemap` (a per-package
  `type`/`func` index). Read the map to navigate; reserve full reads for files
  you're editing through. Optional later: a generated dependency/call graph
  (needs tooling/network; stales faster than text).
- **A2 — scope audits:** named files + named concerns + "medium" breadth on a
  known-healthy codebase; reserve full fan-out for genuinely unknown territory.
  Prefer windowed reads (`Grep` symbol → `Read` offset/limit) and ask sub-agents
  for the exact line ranges you'll edit so you don't re-open files.
- **A3 — pre-push self-review (DONE):** CONVENTIONS now lists `code-review`
  (always) and `verify`/`run` (UI) as a pre-push step, not ad hoc.
- **A4 — persist process learnings (DONE):** this RETROS series, so lessons
  compound like REGRESSIONS does for bugs.

## Session-optimization checklist (carry forward)

1. Map-first, windowed-reads-second; full reads only when editing most of a file.
2. Delegate discovery to Explore/sub-agents; keep the conclusion + line ranges,
   not file dumps.
3. Scope audits; don't "sweep everything" on a healthy codebase.
4. Self-review the diff (`/code-review`, `/simplify`) before push.
5. Package-scoped tests in the loop; full `make test` only before push.
6. Keep the KICKOFF "READ FIRST" list tight (now: CODEMAP + the 5 core docs).
