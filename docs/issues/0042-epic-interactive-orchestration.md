---
id: 0042
title: "Epic: interactive orchestration — the Runs surface goes from read-only to actionable (roadmap v8)"
status: closed
severity: medium
group:
github:
links:
  adr: [0023, 0024]
  prs: [76, 77]
  issues: [0043, 0044]
  regression:
---

> **Closed 2026-06-09 — exhausted.** Both children shipped: V18 rerun-from-Runs
> ([0043](0043-rerun-workflow-from-runs-page.md), PR #76, ADR-0023) and V19 abort-in-flight
> ([0044](0044-abort-in-flight-run.md), PR #77, ADR-0024). The teed-up V20 (rerun a single
> failed lane) and the deferred Candidate B (cost-spike/anomaly reader) were **not** filed as
> children here: the next value×fit pass became **roadmap v10**, where the anomaly reader is
> folded into epic [0050](0050-epic-billing-fidelity.md) P3 (estimate-vs-reported drift) and the
> interactive-orchestration thread is considered complete for v8. Closing rather than carrying an
> open epic with no open child.

## Charter

Roadmap **v7** (epic 0038: cost⋈run reconciliation) is shipped and closed — orchestrated
spend is now reconcilable at the workflow + lane grain, on-page and exportable
(V15/V16/V17). With that, the reconciliation surface — and, after v4–v7, the whole
**observability** surface — is exhausted. The v8 research pass (NEXT_FEATURES "roadmap v8"
section) re-read the code against the two differentiators and found the standout remaining
gap is not more depth or more readers: it is that **the orchestration surface is entirely
read-only**. A user can *see* that a run failed or cost too much — across run history,
per-workflow + per-lane aggregates, ledger⋈runs reconciliation, and CSV export — but
cannot **act** on any of it. Every orchestration control (the sole run-trigger) lives only
on the Workflows page.

This epic makes the orchestration surface **interactive**, starting with the highest-value,
lowest-risk action: **rerun a recorded run from the Runs page**. Where v4–v7 were pure
readers, this epic's children are **actions with side effects** (they spawn live
orchestration via the `copilot.Client` seam), so — unlike the reconciliation epic — the
first child takes an ADR (**ADR-0023**) for the action's semantics and seam. The action is
kept behind a single shared trigger (`launchWorkflow`), so there remains exactly one
orchestration path.

### Teed-up paydown re-evaluated and deferred

TECH_DEBT #8 (switch the append-only stores to a JSONL log for O(1) appends) stays
**deferred** — **ADR-0009 already considered and rejected JSON Lines** (it abandons the
temp-file+rename atomicity the codebase standardises on, needs bespoke torn-line
recovery), and the #8 volume trigger ("when the per-turn rewrite makes itself visible") is
**unmet** at this localhost single-user tool's one-record-per-turn volume. Reversing a
sound, accepted ADR to fix a non-problem (severity *low* / interest *low*) is
negative-value. The v8 epic is a **product/interactivity** epic instead.

## Tasks

- [x] **V18 — rerun a recorded run from the Runs page** (M; the first action on the
      orchestration surface) → [0043](0043-rerun-workflow-from-runs-page.md) (**shipped**,
      PR #76; ADR-0023). A `↻ rerun` control on each recorded run re-executes that run's
      workflow — looked up by `WorkflowID` and run as its **current** definition (a
      re-execution, not a historical replay) — through the same `launchWorkflow` trigger as
      the Workflows-page run, so the new run rolls up under the **same** per-workflow totals
      / aggregates / reconciliation (coherent with V13/V15). The control is gated on the
      workflow still existing (`CanRerun`) and refused while the server is busy. **First
      child** — the epic is born in its PR.
- [x] **V19 — abort an in-flight run from the Chat lanes panel** (M; the dual of rerun) →
      [0044](0044-abort-in-flight-run.md) (**shipped**, PR #77; ADR-0024). A `⏹ stop run`
      control on the lanes panel stops a running workflow: each still-running lane's backing
      session is aborted over the existing `copilot.Client.Abort` seam, the unsettled lanes
      settle **failed** (detail `⏹ aborted`) and the run records as a **failed** outcome —
      *a stopped run is a failed run*, **no new lane status / schema change**. Completes the
      basic interactive control set (start → rerun → stop). The single completion path
      (`runFrags`) was made **idempotent** (`run.recorded`) to close a double-record race
      where `laneError` (called from a lane goroutine that bypasses the reducer guard)
      re-enters it after the abort already settled the run. **Second child.**

## Status

**Open — first two children shipped (V18 rerun PR #76; V19 abort PR #77).** V18 (rerun,
0043) opened the "interactive orchestration" theme — the first action on a surface that was
read-only through all of v4–v7. V19 (abort, 0044, ADR-0024) added its **dual**, completing
the basic interactive control set (**start → rerun → stop**). The interactive surface still
has a teed-up higher-grain child (**V20 — rerun a single failed lane**, L, likely an ADR for
the partial-rerun semantics + lane lineage); the next session's fresh value×fit pass decides
whether the epic stays open for V20 or closes as exhausted and pivots to the deferred
**Candidate B** (cost-spike/anomaly reader, a pure reader needing no ADR). Per the repo
convention each child is born in its PR.

## Notes

Per CONVENTIONS: write the failing test first; keep domain logic pure
(`telemetry`/`ctxforge`/`config` dependency-free). Unlike the v7 reader epic, V18 is an
**action**: it takes **ADR-0023** (written first, ADR-0004) for the rerun semantics
("re-execute the current workflow definition, not replay the recorded one") and the shared
trigger seam, and keeps the web→orchestration trigger behind one clean `launchWorkflow`
boundary (no new seam to the runtime — it reuses the existing `copilot.Client` lifecycle).
`make lint && make test` (floor 65%) + `make e2e` for UI; verify any e2e selector against
the Go-rendered HTML first and keep new marker classes DISJOINT (the V16/V17 lesson —
`rerun` is disjoint from `button.run` / `a.export`). Fold ADR/CONTRACTS/CONTEXT into the
feature branch (ADR-0004).

## Numbering

Highest on disk before this pass: issues → **0041**, epic → **0038**, ADRs → **0022**.
This epic takes **0042**; its first child **V18** takes issue **0043** (next free after
0041). **ADR-0023 consumed** — the first **action** child (vs. the v7 reader epic which
consumed none), for the rerun semantics + the shared trigger seam.
