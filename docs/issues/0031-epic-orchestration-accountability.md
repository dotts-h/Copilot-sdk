---
id: 0031
title: "Epic: orchestration accountability — the Runs surface reaches cost-surface parity (roadmap v6)"
status: open
severity: medium
group:
github:
links:
  adr:
  prs: [59, 61]
  issues: [0034, 0035]
  regression:
assets: []
---

## Charter

Roadmap **v5** (epic 0030: orchestration visibility & polish) is shipped and closed —
sub-agent descriptions, keybinding live-apply, and the generic `AppendOnlyStore[T]` all
landed, and `docs/NEXT_FEATURES.md` (roadmap v3) is fully drained of product-leverage
picks. With **both** differentiators now deep *and* surfaced, the v6 research pass
(NEXT_FEATURES "roadmap v6" section) re-read the code against them and found the next
leverage is not more *depth* within either differentiator but **closing the parity gap
between the two persisted stores' surfaces.**

The **cost** ledger (`SpendStore`) is a mature, *accountable* surface: a trend view with
a window selector, per-model / per-agent / per-workflow shares, a burn-rate forecast,
**and a CSV export** so spend can be analysed outside the app. The **runs** store
(`RunStore`) — the orchestration sibling, added in B3/ADR-0022 — has a read-only Runs
view with a per-workflow roll-up and per-lane breakdown, but its surface lags: it
**cannot be exported**, has no time-window selector, and surfaces only *average* cost in
its summary. A workflow run is the product's unit of orchestration, and unlike a metered
turn it records skipped branches that leave **no** spend record — so the run store holds
data the spend export can't. Yet that data can't leave the tool.

This epic brings the **Runs / orchestration surface up to the cost surface's
accountability bar** — export first, then the smaller parity gaps — all as **pure
readers / presentation-layer compositions over the existing v1 run records** (no schema
change, no new store).

### Teed-up paydown re-evaluated and superseded

The v6 research validated-then-**superseded** the teed-up TECH_DEBT #8 (switch the
append-only stores to a JSONL log for O(1) appends). **ADR-0009 already considered and
rejected JSON Lines** with reasoning that is still valid: JSONL breaks the
temp-file+rename atomicity the codebase standardises on across config/forge/spend, needs
bespoke torn-final-line recovery, and the O(n) full-file rewrite is a non-issue at this
localhost single-user tool's one-record-per-turn volume (the #8 trigger — "when turn
volume makes the per-turn rewrite visible" — is **unmet**). Reversing a sound, accepted
ADR to fix a non-problem (TECH_DEBT #8 is severity *low* / interest *low*) is
negative-value, so #8 stays a candidate, deferred to its trigger. The v6 epic is a
**product** epic instead. See the NEXT_FEATURES "roadmap v6" section.

## Tasks

- [x] **V11 — Runs CSV export** (S; pure reader + route) →
      [0034](0034-runs-csv-export.md) (**shipped**, PR #59; no ADR — a pure additive
      reader + GET route, the orchestration sibling of the spend ledger's
      `WriteCSV`/`/telemetry/export.csv`). `telemetry.WriteRunsCSV` flattens the run
      history to **one row per lane** (run-level columns repeated) so a branched run's
      **skipped** lane — which leaves no spend record — is first-class in the export;
      `GET /runs/export.csv` streams it as an attachment; the Runs page carries an
      "Export CSV" link when history exists. **First child.**

- [x] **V13 — Total cost on the per-workflow summary** (S; pure presentation-layer
      slice) → [0035](0035-runs-summary-total-cost.md) (**shipped**, PR #61; no ADR — a
      pure UI slice over an already-computed field). The Runs summary showed `AvgCredits` but not
      `TotalCredits` (already rolled up on `RunAggregate`); `runSummaryRow` now surfaces
      it and the `runsPage` table gained a "Total cost" column beside "Avg cost", so a
      workflow's *cumulative* orchestrated spend reads beside its average. **Second
      child.**

### Candidate next children (not yet promoted — pick from the v6 research)

- **V12 — Runs time-window selector** (S): mirror the Telemetry trend's 14/30/90-day
  selector on the Runs page, threading a clamped `?window=` so a long history can be
  sliced. Presentation-layer slice over the existing records. **Next up.**
- **V14 — Per-lane cost roll-up** (M): a `LaneShares`-style pure reader keyed by
  (workflow, lane) over the run history, surfacing *"which lane in this workflow costs /
  fails most?"* — the finest orchestration-attribution grain, currently unsurfaced.

## Status

**Open.** First child **V11 (Runs CSV export, 0034)** shipped in this epic's opening
**PR #59** (per the repo convention — an epic is born in its first child's PR, as epic
0030 opened inside V3's PR #56). Second child **V13 (Total cost on the per-workflow
summary, 0035)** shipped next in **PR #61** — a pure presentation-layer slice surfacing
`RunAggregate.TotalCredits` beside the average. The remaining candidate children (V12
window selector, then V14 per-lane roll-up) stay in the v6 research until promoted.

## Notes

Per CONVENTIONS: write the failing test first; keep domain logic pure
(`telemetry`/`ctxforge`/`config` dependency-free — `WriteRunsCSV` is a pure function over
a record slice, the writer the only IO and the caller's); `make lint && make test` (floor
65%) + `make e2e` for UI; fold ADR/CONTRACTS/REGRESSIONS into the feature branch that
motivates them (ADR-0004). V11 needs **no ADR** — a pure additive reader + a new GET
export route, no persisted-contract or cross-package-seam change; noted as a pure-reader
composition in CONTRACTS §3 (the route) and §4 (the runs-store entry).

## Numbering

Highest on disk before this pass: issues → **0033**, epic → **0030**, ADRs → **0022**.
This epic takes **0031**; its first child **V11** takes issue **0034** (next free after
0033) and its second child **V13** takes issue **0035**. **No ADR consumed** — both are
pure additive readers / presentation-layer slices, pre-blessed by the ADR-0009 export
precedent (highest ADR stays 0022).
