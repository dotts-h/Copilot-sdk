# Retro 0006 — epic 0086 close-out + a review of how skills/scripts/docs are working

- Date: 2026-06-12
- Scope: the work since [RETROS 0005](0005-deep-quality-architecture-and-test-hardening.md)
  — **epic 0086** (the code-health follow-ups 0005 deliberately deferred): the
  `normalize.go` tool-map leak sweep (**0089**, PR #155) and the two god-file splits
  (**0087** `workflow.go`, **0088** `server.go`, PR #156). Plus the periodic question this
  retro exists to answer: how are the skills, scripts, and docs serving us, and is a
  **mini-RAG** over the docs corpus worth adopting ([ADR-0050](../adr/0050-no-mini-rag-file-based-docs-and-agentic-search.md): no).

## Context

Epic 0086 was the cleanest kind of epic: every item was pre-scoped by the RETROS 0005
audits (seams named, risk named, parallelism named), and execution matched the plan —
0089 shipped alone (different package, fully parallel-safe, +1 guard test under `-race`),
then 0087+0088 shipped together in one PR (same package, disjoint files, shared import
block — exactly the conflict the epic note said to avoid by doing them back-to-back).
`workflow.go` (1222 LOC, 5 responsibilities) became `run_engine.go` / `run_adapter.go` /
`run_render.go` / `workflow_crud.go`; `server.go` shrank 1015→741 with handlers lifted to
`interactions.go` / `forge_handlers.go`. No symbol renamed, no behavior change, web
coverage held at 89.8%, CODEMAP regenerated in both PRs, TECH_DEBT #17/#18 marked paid.

## What worked (the docs system, graded on this stretch)

- **Audit → TECH_DEBT → epic → execution is a working pipeline.** The 0005 audits named
  the exact seams (`run_engine`/`run_adapter`/`run_render`, `interactions`/`forge_handlers`)
  and 0086's children carried them verbatim — so the splits were mechanical, as A1
  promised. The "record the seams when you find them, split when the trigger fires"
  doctrine paid off within one day of being written.
- **Issue close-outs are carrying real information.** 0089's close-out documents the
  chosen shape (a `toolSession` reverse index + `sweepToolMaps(sid)` under `c.mu`) and
  *why* no ADR was needed — a future reader gets the design without archaeology.
- **The get-next flow in the remote env ran clean** — RETROS 0003/0004's fixes
  (hard push-target branch, no-`gh` CI-poll-and-merge) needed no re-derivation this time.
  Two stumble-free PRs is the first evidence the codified flow is stable, not just patched.
- **Skills as the constitution's enforcement arm keep proving out**: `/code-review`
  pre-push (A3 of 0001) is now routine, and the guard scripts (`check-workflows.sh`,
  `codemap.sh`) ran without incident.

## What drifted (found in this review, fixed in this pass)

- **Epic 0086's own acceptance checkboxes were left unticked** despite `status: closed`
  and all three children closed with checked acceptance. The same class of drift 0005
  found on epic 0069 — children and INDEX right, the epic's own body stale. Ticked now.
  Pattern after two occurrences: **the epic body is the forgotten surface** at close
  time; the closer updates status + INDEX + children but not the epic's checklist.
- **`docs/adr/README.md` was missing rows for ADR-0048 and ADR-0049** — the per-decision
  files landed (PRs #149, #152) but the index regen step was skipped. Added now (plus
  0050). Same lesson at a different address: generated/maintained indexes only stay
  honest when the close-out step names them explicitly.

## Research: is a mini-RAG worth it over our "lots of files"? (No — ADR-0050)

The docs corpus is ~1.24 MB of markdown (~300K tokens; core docs ~290 KB, 50 ADRs, 92
issues). The full reasoning is in [ADR-0050](../adr/0050-no-mini-rag-file-based-docs-and-agentic-search.md);
the short version:

- We already **have** a retrieval layer — it's just hand-built and deterministic:
  CLAUDE.md routes, CODEMAP/CONTEXT/INDEX index, grep/glob search exactly, windowed Read
  loads. Every retro since 0001 confirms this keeps sessions cheap (0005's three scoped
  audits at ~140–160K tokens each over a 49-ADR corpus).
- A mini-RAG buys *semantic* matching at the price of **staleness** (this corpus churns
  every merge), **infra** (third-party embeddings, a vector store, a serving shim,
  chunking for table-heavy markdown), and **lower precision on a controlled vocabulary**
  — CONTEXT.md's ubiquitous language means lexical search is already near-perfect.
- External evidence points the same way: Claude Code itself started with RAG + a local
  vector DB and dropped it because iterative agentic search outperformed it while
  avoiding index staleness/drift/security exposure.
- ADR-0050 records the **revisit triggers** (corpus ≫ context with uncontrolled
  vocabulary; semantic-not-lexical queries; a zero-maintenance first-party retrieval
  primitive) so this isn't re-litigated from scratch each time the corpus grows.

The investment with positive expected value is the one we already make: keep indexes
regenerable (`make codemap`), keep terms controlled (CONTEXT.md before naming), keep
headings greppable.

## Skill / tool usage

- This retro session itself was docs-only research: repo archaeology (git log, issue
  files, doc sizes) + one targeted web search to ground the RAG question — no code
  changes, no sub-agents needed at this scale.
- The mini-RAG research deliberately produced an **ADR, not just a retro section** — the
  decision has revisit triggers and will be asked again; "one fact, one home" says the
  durable answer lives in `docs/adr/`, and the retro just links it.

## Action items

- **A1 — add the epic body to the close-out checklist.** When closing an epic: status,
  INDEX, children, **and the epic's own acceptance checkboxes + links**. Two drifts in
  two retros (0069, 0086) make this a pattern, not noise. ✅ Folded into the
  tracking-issues skill's close-out step (see this PR).
- **A2 — index regen is part of landing an ADR.** A new ADR isn't done until
  `docs/adr/README.md` has its row — same discipline as CODEMAP regen for moved symbols.
  ✅ 0048/0049 rows restored in this pass; folded into the tracking-issues skill alongside A1.
- **A3 — no mini-RAG; revisit only on ADR-0050's named triggers.** *(the ADR is the
  record.)*

## Session-optimization checklist (carry forward)

1. Close an epic completely: status + INDEX + children + the epic's own body.
2. Landing an ADR includes its README index row (treat like CODEMAP regen).
3. For "should we adopt X infra?" questions, write the ADR with revisit triggers —
   retro sections get re-asked; ADRs get cited.
4. Docs questions in this repo: route (CLAUDE.md) → index (CODEMAP/CONTEXT/INDEX) →
   grep → windowed read. Don't reach for retrieval infra the corpus doesn't need.
