// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package telemetry

import (
	"encoding/csv"
	"io"
	"math"
	"sort"
	"strconv"
)

// This file converges the two persisted stores. The spend ledger (history.go) and
// the run history (runs.go) answer overlapping questions: a workflow's spend lives
// in BOTH — as WorkflowShares over metered turns AND as RunAggregates.TotalCredits
// over recorded runs — reconciled nowhere. The two figures can diverge (a turn
// metered outside a recorded run; a run whose lanes metered under a different
// attribution) and a user has no way to see — or trust — that they agree.
// WorkflowReconcile is the pure cross-store reader that joins the two roll-ups per
// workflow and surfaces the delta, so orchestrated spend is not just accountable on
// each surface but reconcilable across them. It is a cousin of WorkflowShares +
// RunAggregates: it takes both record slices, returns ids (the web layer resolves
// labels), and adds no cross-package seam — keeping the package dependency-free.

// WorkflowRecon reconciles one workflow's spend across the two persisted stores:
// the ledger credits (from workflow-attributed SpendRecords, the WorkflowShares
// grain) versus the run credits (from recorded RunRecords, the
// RunAggregates.TotalCredits grain), and their Delta. Delta is LedgerCredits −
// RunCredits — zero when the two stores agree, positive when the ledger metered
// more than the recorded runs (a turn metered outside a run), negative when the
// runs metered more than the ledger attributes (a run metered under a different
// attribution). A workflow present in only ONE store appears with the other side
// zero (and Delta its full magnitude).
type WorkflowRecon struct {
	WorkflowID    string
	LedgerCredits float64 // workflow-attributed spend from the ledger (WorkflowShares grain)
	RunCredits    float64 // metered cost of the workflow's recorded runs (RunAggregates grain)
	Delta         float64 // LedgerCredits − RunCredits (0 = the stores agree)
}

// WorkflowReconcile joins the spend ledger and the run history per workflow,
// surfacing the credits each store attributes to the workflow and their delta — so
// orchestrated spend is reconcilable across the two stores, not just accountable on
// each. Ledger credits come from workflow-attributed spend records (the empty-
// workflow "chat" bucket is excluded, like WorkflowShares — it has no run to
// reconcile against); run credits sum each workflow's recorded runs' metered cost
// (RunRecord.Credits, a skipped lane adding zero, the RunAggregates.TotalCredits
// grain). A workflow seen in either store yields a row (the other side zero). Sorted
// by absolute delta DESCENDING so the biggest discrepancy — the thing a
// reconciliation view exists to surface — reads first (ties broken by ledger credits
// descending, then workflow id ascending: a total order over the unique workflow key
// for full determinism regardless of input/map order). Empty inputs → empty slice.
// Pure: same records → same result.
func WorkflowReconcile(spend []SpendRecord, runs []RunRecord) []WorkflowRecon {
	// Ledger side: sum USD per workflow then convert once (mirroring WorkflowShares /
	// shareBy, which totals USD per group and divides at the end), so the ledger figure
	// is bit-for-bit the same as the "Cost by workflow" share it sits beside.
	ledgerUSD := map[string]float64{}
	for _, r := range spend {
		if r.WorkflowID == "" {
			continue // non-workflow chat spend has no run to reconcile against
		}
		ledgerUSD[r.WorkflowID] += r.USD
	}
	// Run side: sum each workflow's recorded runs' metered credits (already in credits).
	runCredits := map[string]float64{}
	for _, r := range runs {
		if r.WorkflowID == "" {
			continue
		}
		runCredits[r.WorkflowID] += r.Credits()
	}
	seen := make(map[string]struct{}, len(ledgerUSD)+len(runCredits))
	for id := range ledgerUSD {
		seen[id] = struct{}{}
	}
	for id := range runCredits {
		seen[id] = struct{}{}
	}
	out := make([]WorkflowRecon, 0, len(seen))
	for id := range seen {
		lc := ledgerUSD[id] / USDPerCredit
		rc := runCredits[id]
		out = append(out, WorkflowRecon{
			WorkflowID:    id,
			LedgerCredits: lc,
			RunCredits:    rc,
			Delta:         lc - rc,
		})
	}
	// |delta| desc, then ledger credits desc, then workflow id asc — a total order over
	// the unique workflow key, so the result is fully deterministic regardless of map
	// iteration order.
	sort.Slice(out, func(i, j int) bool {
		di, dj := math.Abs(out[i].Delta), math.Abs(out[j].Delta)
		if di != dj {
			return di > dj
		}
		if out[i].LedgerCredits != out[j].LedgerCredits {
			return out[i].LedgerCredits > out[j].LedgerCredits
		}
		return out[i].WorkflowID < out[j].WorkflowID
	})
	return out
}

// LaneRecon reconciles one (workflow, lane)'s spend across the two persisted stores at
// the FINEST grain — one step below WorkflowRecon's per-workflow total. LedgerCredits is
// the lane-tagged spend the ledger attributes to this lane (SpendRecord grouped by
// WorkflowID + LaneIndex — the lane attribution of ADR-0018, the ledger cousin of
// LaneShares); RunCredits is what the run history's matching lane metered
// (RunLane.Credits, a skipped lane adding zero — the LaneShares grain); Delta is
// LedgerCredits − RunCredits (zero when the two stores agree). A (workflow, lane) present
// in only ONE store appears with the other side zero — so a divergence the per-workflow
// row only totals is locatable at the exact lane.
type LaneRecon struct {
	WorkflowID    string
	LaneIndex     int
	LedgerCredits float64 // lane-tagged spend from the ledger (the LaneShares ledger cousin)
	RunCredits    float64 // metered cost of the lane's recorded runs (the LaneShares grain)
	Delta         float64 // LedgerCredits − RunCredits (0 = the stores agree)
}

// LaneReconcile joins the spend ledger and the run history per (workflow, lane) — the
// per-lane cousin of WorkflowReconcile, one grain finer. Ledger credits group workflow-
// attributed spend by (WorkflowID, LaneIndex) (the empty-workflow chat bucket excluded,
// like WorkflowShares — a non-workflow turn has no lane to reconcile against; a turn the
// run owned but that didn't route to a lane persists at LaneIndex 0, ADR-0018, so the
// ledger figure reflects the records as stored); run credits sum each recorded run lane's
// metered cost by (WorkflowID, lane Index) (a skipped lane adding zero, the LaneShares
// grain). A (workflow, lane) seen with non-zero credits in EITHER store yields a row (the
// other side zero if one-sided); a lane that metered zero on BOTH sides — e.g. a skipped
// run lane with no matching ledger spend, common at this grain — has nothing to reconcile
// and is omitted so skipped lanes don't pad the table. Sorted by absolute delta DESCENDING
// so the biggest discrepancy reads first
// (ties broken by ledger credits descending, then workflow id ascending, then lane index
// ascending: a total order over the unique (workflow, lane) key for full determinism
// regardless of input/map order). Empty inputs → empty slice. Pure: same records → same
// result.
func LaneReconcile(spend []SpendRecord, runs []RunRecord) []LaneRecon {
	type key struct {
		workflow string
		lane     int
	}
	// Ledger side: sum USD per (workflow, lane) then convert once (mirroring shareBy /
	// WorkflowReconcile), so the per-lane ledger figure is bit-for-bit the per-lane slice
	// of the "Cost by workflow" share.
	ledgerUSD := map[key]float64{}
	for _, r := range spend {
		if r.WorkflowID == "" {
			continue // non-workflow chat spend has no lane to reconcile against
		}
		ledgerUSD[key{r.WorkflowID, r.LaneIndex}] += r.USD
	}
	// Run side: sum each recorded run lane's metered credits by (workflow, lane).
	runCredits := map[key]float64{}
	for _, r := range runs {
		if r.WorkflowID == "" {
			continue
		}
		for _, l := range r.Lanes {
			runCredits[key{r.WorkflowID, l.Index}] += l.Credits
		}
	}
	seen := make(map[key]struct{}, len(ledgerUSD)+len(runCredits))
	for k := range ledgerUSD {
		seen[k] = struct{}{}
	}
	for k := range runCredits {
		seen[k] = struct{}{}
	}
	out := make([]LaneRecon, 0, len(seen))
	for k := range seen {
		lc := ledgerUSD[k] / USDPerCredit
		rc := runCredits[k]
		if lc == 0 && rc == 0 {
			continue // a lane with no spend either way (e.g. a skipped lane) — nothing to reconcile
		}
		out = append(out, LaneRecon{
			WorkflowID:    k.workflow,
			LaneIndex:     k.lane,
			LedgerCredits: lc,
			RunCredits:    rc,
			Delta:         lc - rc,
		})
	}
	// |delta| desc, then ledger credits desc, then workflow id asc, then lane index asc —
	// a total order over the unique (workflow, lane) key, so the result is fully
	// deterministic regardless of map iteration order.
	sort.Slice(out, func(i, j int) bool {
		di, dj := math.Abs(out[i].Delta), math.Abs(out[j].Delta)
		if di != dj {
			return di > dj
		}
		if out[i].LedgerCredits != out[j].LedgerCredits {
			return out[i].LedgerCredits > out[j].LedgerCredits
		}
		if out[i].WorkflowID != out[j].WorkflowID {
			return out[i].WorkflowID < out[j].WorkflowID
		}
		return out[i].LaneIndex < out[j].LaneIndex
	})
	return out
}

// WriteReconcileCSV serializes the cross-store reconciliation to CSV — the export
// sibling of WriteCSV (spend) and WriteRunsCSV (runs) — so the ledger-vs-runs
// divergence the Telemetry page surfaces can LEAVE the tool and be analysed in a
// spreadsheet, the convergence's natural last reach (CONTRACTS §3/§4). One file
// carries BOTH grains the page shows as two tables: the per-workflow rows
// (WorkflowReconcile, V15) then the per-(workflow, lane) rows (LaneReconcile, V16). A
// leading `grain` column ("workflow" | "lane") labels which — so the two grains stay
// self-documenting in one file and a consumer never double-counts a workflow total
// against its lane breakdown by summing blindly (filter `grain` first); the `lane`
// cell is blank on a workflow row and the lane index on a lane row. Column order is
// fixed; credits use csvFloat (the same precision-rounded format as the sibling
// writers). Rows are the readers' own output, so ordering is deterministic (biggest
// |delta| first within each grain) and a chat-only or empty input yields the header
// alone. Pure: the writer is the only IO and the caller owns it.
func WriteReconcileCSV(w io.Writer, spend []SpendRecord, runs []RunRecord) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"grain", "workflow", "lane", "ledgerCredits", "runCredits", "delta"}); err != nil {
		return err
	}
	for _, r := range WorkflowReconcile(spend, runs) {
		row := []string{"workflow", r.WorkflowID, "", csvFloat(r.LedgerCredits), csvFloat(r.RunCredits), csvFloat(r.Delta)}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	for _, r := range LaneReconcile(spend, runs) {
		row := []string{"lane", r.WorkflowID, strconv.Itoa(r.LaneIndex), csvFloat(r.LedgerCredits), csvFloat(r.RunCredits), csvFloat(r.Delta)}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
