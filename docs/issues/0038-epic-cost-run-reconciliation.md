---
id: 0038
title: "Epic: cost⋈run reconciliation — converge the two persisted stores (roadmap v7)"
status: closed
severity: medium
group:
github:
links:
  adr:
  prs: [66, 74]
  issues: [0039, 0040, 0041]
  regression:
---

## Charter

Roadmap **v6** (epic 0031: orchestration accountability) is shipped and closed — the
Runs / orchestration surface reached **cost-surface parity** (windowed, exportable, with
total & per-workflow & per-lane roll-ups: V11/V13/V12/V14). With **both** persisted
surfaces now mature *and* at parity, the v7 research pass (NEXT_FEATURES "roadmap v7"
section) re-read the code against the two differentiators and found the next leverage is
no longer *within* either surface but in **converging them**.

The **cost** ledger (`SpendStore`) and the **run history** (`RunStore`) are still **two
separate stores answering overlapping questions**: a workflow's spend lives in **both** —
as `telemetry.WorkflowShares` over metered turns **and** as
`telemetry.RunAggregates.TotalCredits` over recorded runs — **reconciled nowhere**. The
two figures can **diverge** (a turn metered outside a recorded run; a run whose lanes
metered under a different attribution) and a user has no way to see — or trust — that they
agree. So orchestrated spend is *accountable* on each surface but not *reconcilable*
across them.

This epic **converges the two stores**: a pure cross-store reader that joins the two
roll-ups per workflow and surfaces the **delta** (ledger spend vs. recorded-run spend),
rendered as a per-workflow "ledger vs runs" comparison — the natural convergence of the
now-mature cost + orchestration surfaces. All children are **pure cross-record readers /
presentation-layer compositions over the existing v1/v2 records** (no schema change, no
new store); the reader takes two record slices and returns ids (the web layer resolves
labels under `forgeMu`), so **no cross-package seam, no new ADR** unless a genuine seam
appears.

### Teed-up paydown re-evaluated and deferred

TECH_DEBT #8 (switch the append-only stores to a JSONL log for O(1) appends) stays
**deferred** — **ADR-0009 already considered and rejected JSON Lines** (it abandons the
temp-file+rename atomicity the codebase standardises on, needs bespoke torn-line
recovery), and the #8 volume trigger ("when the per-turn rewrite makes itself visible") is
**unmet** at this localhost single-user tool's one-record-per-turn volume. Reversing a
sound, accepted ADR to fix a non-problem (severity *low* / interest *low*) is
negative-value. The v7 epic is a **product/convergence** epic instead.

## Tasks

- [x] **V15 — cost⋈run reconciliation reader + Telemetry "Ledger vs runs"** (M; pure
      cross-store reader + UI composition) → [0039](0039-cost-run-reconciliation.md)
      (**shipped**, PR #66; no ADR — a pure cross-record reader returning ids, no
      cross-package seam).
      `telemetry.WorkflowReconcile(spend []SpendRecord, runs []RunRecord) []WorkflowRecon`
      joins the two roll-ups per workflow to `WorkflowRecon{WorkflowID, LedgerCredits,
      RunCredits, Delta}`, sorted by absolute delta descending (the biggest discrepancy
      first; ties → ledger credits desc, then workflow id asc — a total deterministic
      order). The Telemetry page renders a **"Ledger vs runs"** per-workflow comparison
      table below "Cost by workflow", resolving ids → labels under `forgeMu` and ambering
      a non-trivial delta. **First child** — the epic is born in its PR.
- [x] **V16 — per-lane cost⋈run reconciliation reader + Telemetry "Ledger vs runs by lane"**
      (M; pure cross-store reader + UI composition) → [0040](0040-per-lane-cost-run-reconciliation.md)
      (**shipped**, PR #74; no ADR — a pure cross-record reader returning ids, no cross-package seam).
      `telemetry.LaneReconcile(spend []SpendRecord, runs []RunRecord) []LaneRecon{WorkflowID,
      LaneIndex, LedgerCredits, RunCredits, Delta}` joins the **same** two roll-ups one grain
      finer — per `(workflow, lane)` — so a divergence the per-workflow row only totals is
      locatable at the exact step. Ledger side groups lane-tagged spend (`SpendRecord` by
      `WorkflowID + LaneIndex`, ADR-0018); run side sums per-lane credits (`RunLane.Credits`,
      the `LaneShares` grain). Sorted by absolute delta descending (ties → ledger credits
      desc, then workflow id asc, then lane index asc — a total deterministic order); a lane
      zero on both sides (a skipped run lane with no ledger spend) is omitted. The Telemetry
      page renders a **"Ledger vs runs by lane"** table below the per-workflow "Ledger vs
      runs", resolving ids → labels under `forgeMu` and ambering a non-trivial delta. **Second
      child.**
- [x] **V17 — reconciliation CSV export reader + `GET /telemetry/reconcile.csv`** (S; pure
      writer + export route) → [0041](0041-reconciliation-csv-export.md)
      (**shipped**, PR #__; no ADR — a pure writer over the existing readers, pre-blessed by
      the ADR-0009 CSV-export precedent, no cross-package seam).
      `telemetry.WriteReconcileCSV(w io.Writer, spend []SpendRecord, runs []RunRecord) error`
      serializes the cross-store reconciliation to CSV — the export sibling of
      `WriteCSV`/`WriteRunsCSV` — so the ledger-vs-runs divergence **leaves the tool** the way
      spend and runs already do. One file carries **both grains** (the per-workflow rows then the
      per-`(workflow, lane)` rows, each labelled by a leading `grain` column so a consumer never
      double-counts; header `grain,workflow,lane,ledgerCredits,runCredits,delta`, the readers' own
      deterministic order).
      Streamed by a new `GET /telemetry/reconcile.csv` route (`handleReconcileExport`, mirroring
      `handleSpendExport`/`handleRunsExport`), surfaced as an "Export CSV" link beside the
      "Ledger vs runs" heading with a **disjoint `reconcile-export`** marker class. **Third and
      final child** — on its merge the reconciliation surface is exhausted (workflow + lane
      grain, on-page and exportable).

## Status

**Closed — reconciliation surface exhausted (workflow + lane grain, on-page and exportable).**
First child **V15 (cost⋈run reconciliation, 0039)** shipped in this epic's opening **PR #66** —
per the repo convention an epic is born in its first child's PR (cf. epic 0031 in V11's PR #59).
`telemetry.WorkflowReconcile` joins the spend ledger's and the run history's per-workflow
roll-ups and surfaces the **delta**, rendered as a "Ledger vs runs" comparison on the Telemetry
page (ids → labels under `forgeMu`, a non-trivial delta ambered). Second child **V16 (per-lane
cost⋈run reconciliation, 0040, PR #74)** joined the same two stores one grain finer (per
`(workflow, lane)`), rendered as a "Ledger vs runs by lane" table, so a divergence the
per-workflow row only totals is locatable at the exact step. Third and final child **V17
(reconciliation CSV export, 0041, PR #__)** added `telemetry.WriteReconcileCSV` + `GET
/telemetry/reconcile.csv` — the export sibling of `WriteCSV`/`WriteRunsCSV` — so the divergence
**leaves the tool** for outside analysis. From a fresh value×fit pass the convergence is **done**:
orchestrated spend is reconcilable at the **workflow** grain (V15) and the **lane** grain (V16)
on-page, and **exportable** (V17); the per-**session** grain is unsupported (`RunRecord` carries no
session id), and the forecast-annotation alternative was dropped as an altitude mismatch. **Epic
CLOSED — roadmap v7 done.** TECH_DEBT #8 stays deferred to its (still-unmet) volume trigger; the
next epic (roadmap v8) is scoped from a fresh value×fit pass against the two differentiators.

## Notes

Per CONVENTIONS: write the failing test first; keep domain logic pure
(`telemetry`/`ctxforge`/`config` dependency-free — `WorkflowReconcile` is a pure function
over two record slices, returning ids); `make lint && make test` (floor 65%) + `make e2e`
for UI; fold ADR/CONTRACTS/REGRESSIONS into the feature branch that motivates them
(ADR-0004). V15 needs **no ADR** — a pure cross-record reader returning ids, no
persisted-contract or cross-package-seam change; noted as a pure-reader composition in
CONTRACTS §3 (the Telemetry page) and §4 (the reconciliation reader joining
`WorkflowShares` + `RunAggregates`).

## Numbering

Highest on disk before this pass: issues → **0037**, epic → **0031**, ADRs → **0022**.
This epic takes **0038**; its first child **V15** takes issue **0039** (next free after
0037). **No ADR consumed** — a pure cross-record reader, pre-blessed by the same
cost ⋈ orchestration convergence rationale as ADR-0022 / the `*Shares` readers (highest
ADR stays 0022).
