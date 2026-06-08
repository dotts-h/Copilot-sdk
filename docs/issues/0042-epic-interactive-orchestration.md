---
id: 0042
title: "Epic: interactive orchestration — the Runs surface goes from read-only to actionable (roadmap v8)"
status: open
severity: medium
group:
github:
links:
  adr: [0023]
  prs: [76]
  issues: [0043]
  regression:
---

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

## Status

**Open — first child shipped (V18, PR #76).** V18 (rerun, 0043) opened the epic and the
"interactive orchestration" theme: the first action on a surface that was read-only through
all of v4–v7. Per the repo convention the epic is born in its first child's PR. After V18,
re-rank the epic from a fresh value×fit pass — candidate later children include: an
**abort/cancel** control on an in-flight run from the Runs/Chat surface; **rerun a single
failed lane**; or a **cost-spike/anomaly** reader (the deferred Candidate B from the v8
pass, a pure reader needing no ADR) if the interactive surface is judged exhausted.

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
