---
id: 0086
title: "Epic: Code-health follow-ups — structural splits + a normalize map-leak sweep (RETROS 0005)"
status: open
severity: low
group:
depends_on: []
github:
links:
  adr: []
  prs: []
  issues: [0087, 0088, 0089]
  regression:
assets: []
---

## Charter

The [RETROS 0005](../RETROS/0005-deep-quality-architecture-and-test-hardening.md) audits
(backend quality/architecture + test-gap) surfaced a small set of **behavior-preserving**
maintenance items that were deliberately kept out of the hardening pass to keep its diff
small and reviewable. They are tracked here as `get-next`-able children so the gains aren't
lost — each is a structural cleanup, not a feature, and **none changes runtime behavior**.

Recorded in TECH_DEBT #17 (the two god files) and #18 (the map leak).

## Children

- [ ] **0087 · Split `workflow.go`** ([0087](0087-split-workflow-god-file.md), M) — the
      1222-LOC file bundles five responsibilities (pure run engine, seam adapter, demo lane
      simulator, lane renderer, Workflows CRUD); split into `run_engine.go` /
      `run_adapter.go` / `run_render.go` along the audit's named seams. Same-package move,
      no behavior change; the pure `workflowRun` zero-client tests must pass byte-identically.
- [ ] **0088 · Split `server.go`** ([0088](0088-split-server-god-file.md), M) — lift the
      perm/ask/plan/elicit interaction handlers into `interactions.go` and the forge
      toggle/delete handlers into `forge_handlers.go`; the `Server` struct + send/budget
      gating stay. Same-package move, no behavior change.
- [x] **0089 · Sweep orphaned tool maps in `normalize.go`** ([0089](0089-normalize-toolmap-leak-sweep.md), S) —
      `c.toolNames`/`c.toolMeta` orphan an entry on a mid-tool error (start with no matching
      complete); add a sweep in `DeleteSession`/`Close` so a long-lived multi-session
      instance doesn't accumulate. Different package (`copilot`). **Done** — a `toolSession`
      reverse index + `sweepToolMaps(sid)` reclaim a session's orphans on teardown (TECH_DEBT #18 paid).

## Sequencing & parallelism

**0089 is independent** (package `internal/copilot`) and can run **fully in parallel** with
the other two. **0087 and 0088 both touch `internal/web`** but disjoint files — they can run
in parallel too, but a single session doing them back-to-back (or coordinating the import
block) avoids a trivial merge conflict. Recommended parallel split: one session on 0089
(copilot), one session on 0087→0088 (web), since they share no files.

## Acceptance (epic)

- [ ] Each child is a pure structural change: `make lint && make test` (floor 65%) green
      with **no** test rewrites for behavior (only file-location / import churn), and the
      CODEMAP regenerated (`make codemap`) where top-level decls move files.
- [ ] No ADR (no decision changes — these are mechanical splits and a leak sweep).
- [ ] Born in its own PR, SemVer **patch** (cleanup, not feature).

## Notes

These are **low-severity, low-interest** items — pick them up when the relevant file is next
touched, or as parallel filler work, not ahead of product roadmap items. The seams for the
splits are named in RETROS 0005 and the backend audit. No behavior change means the existing
test suites are the guard; do not add behavior tests, only keep the current ones green.
</parameter>
</invoke>
