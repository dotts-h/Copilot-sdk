package telemetry

import (
	"math"
	"sort"
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
