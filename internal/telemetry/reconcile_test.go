package telemetry

import (
	"testing"
	"time"
)

func TestWorkflowReconcileJoinsBothStores(t *testing.T) {
	// WorkflowReconcile joins the spend ledger and the run history per workflow,
	// surfacing the credits each store attributes to the workflow and their delta —
	// so orchestrated spend is reconcilable across the two stores, not just
	// accountable on each. Ledger credits come from workflow-attributed SpendRecords
	// (the WorkflowShares grain, the empty-workflow chat bucket excluded); run credits
	// come from each workflow's recorded RunRecords (the RunAggregates.TotalCredits
	// grain). Input order is shuffled to prove the join is order-independent.
	spend := []SpendRecord{
		// alpha: two metered turns under the ledger — 2.00 + 1.00 = 3.00 cr ($0.03).
		{Model: "gpt-5", USD: 0.02, WorkflowID: "alpha"},
		{Model: "gpt-5", USD: 0.01, WorkflowID: "alpha"},
		// beta: one metered turn — 5.00 cr ($0.05).
		{Model: "gpt-5", USD: 0.05, WorkflowID: "beta"},
		// plain chat (no workflow) — excluded from the per-workflow reconciliation.
		{Model: "gpt-5", USD: 0.10},
	}
	runs := []RunRecord{
		// alpha: one recorded run metering 3.00 cr across its lanes — agrees with the
		// ledger (delta 0).
		{WorkflowID: "alpha", Name: "Alpha", Lanes: []RunLane{
			{Index: 0, Status: "done", Credits: 2},
			{Index: 1, Status: "done", Credits: 1},
		}},
		// beta: a recorded run metering only 4.00 cr — diverges from the ledger's 5.00
		// (a turn metered outside this run), so delta = +1.00.
		{WorkflowID: "beta", Name: "Beta", Lanes: []RunLane{
			{Index: 0, Status: "done", Credits: 4},
		}},
	}
	got := WorkflowReconcile(spend, runs)
	if len(got) != 2 {
		t.Fatalf("want 2 reconciliation rows, got %d: %+v", len(got), got)
	}
	// Sorted by absolute delta descending: beta (|+1.00|) before alpha (0).
	if got[0].WorkflowID != "beta" {
		t.Fatalf("row[0] = %+v, want beta (the divergent workflow) first", got[0])
	}
	approx(t, got[0].LedgerCredits, 5)
	approx(t, got[0].RunCredits, 4)
	approx(t, got[0].Delta, 1) // ledger 5 − runs 4
	if got[1].WorkflowID != "alpha" {
		t.Fatalf("row[1] = %+v, want alpha", got[1])
	}
	approx(t, got[1].LedgerCredits, 3)
	approx(t, got[1].RunCredits, 3)
	approx(t, got[1].Delta, 0) // the two stores agree
}

func TestWorkflowReconcileOneSidedWorkflowsAppear(t *testing.T) {
	// A workflow present in ONE store but not the other still appears, with the other
	// side zero and the delta its full magnitude — the "a turn metered outside a
	// recorded run" / "a run whose lanes metered under a different attribution" cases
	// the reconciliation exists to surface.
	spend := []SpendRecord{
		// ledger-only: spent but never recorded a run.
		{Model: "gpt-5", USD: 0.07, WorkflowID: "ledger-only"},
	}
	runs := []RunRecord{
		// run-only: a recorded run whose cost never reached the ledger under this id.
		{WorkflowID: "run-only", Lanes: []RunLane{{Index: 0, Status: "done", Credits: 2}}},
	}
	got := WorkflowReconcile(spend, runs)
	if len(got) != 2 {
		t.Fatalf("want 2 rows (both one-sided workflows), got %d: %+v", len(got), got)
	}
	// ledger-only has the larger |delta| (7 vs 2), so it sorts first.
	if got[0].WorkflowID != "ledger-only" {
		t.Fatalf("row[0] = %+v, want ledger-only first (larger |delta|)", got[0])
	}
	approx(t, got[0].LedgerCredits, 7)
	approx(t, got[0].RunCredits, 0)
	approx(t, got[0].Delta, 7)
	ro := got[1]
	if ro.WorkflowID != "run-only" {
		t.Fatalf("row[1] = %+v, want run-only", ro)
	}
	approx(t, ro.LedgerCredits, 0)
	approx(t, ro.RunCredits, 2)
	approx(t, ro.Delta, -2) // ledger 0 − runs 2
}

func TestWorkflowReconcileDeterministicOrder(t *testing.T) {
	// Rows with EQUAL |delta| tie-break by ledger credits descending, then workflow id
	// ascending — a stable total order over the unique workflow key, regardless of
	// input order. Here both workflows reconcile exactly (delta 0), so the tie-break
	// runs: gamma (ledger 3) before alpha (ledger 3) by id; both before zeta (ledger 1).
	mk := func(wf string, cr float64) ([]SpendRecord, []RunRecord) {
		return []SpendRecord{{Model: "gpt-5", USD: cr * USDPerCredit, WorkflowID: wf}},
			[]RunRecord{{WorkflowID: wf, Lanes: []RunLane{{Index: 0, Status: "done", Credits: cr}}}}
	}
	build := func() ([]SpendRecord, []RunRecord) {
		var sp []SpendRecord
		var rn []RunRecord
		for _, e := range []struct {
			wf string
			cr float64
		}{{"zeta", 1}, {"alpha", 3}, {"gamma", 3}} {
			s, r := mk(e.wf, e.cr)
			sp = append(sp, s...)
			rn = append(rn, r...)
		}
		return sp, rn
	}
	sp, rn := build()
	got := WorkflowReconcile(sp, rn)
	if len(got) != 3 {
		t.Fatalf("want 3 rows, got %d", len(got))
	}
	// All delta 0 → ledger desc (3,3,1) then id asc (alpha < gamma): alpha, gamma, zeta.
	if got[0].WorkflowID != "alpha" || got[1].WorkflowID != "gamma" || got[2].WorkflowID != "zeta" {
		t.Fatalf("equal-|delta| ties must sort by ledger desc then id asc; got %+v", got)
	}
}

func TestWorkflowReconcileEmpty(t *testing.T) {
	if got := WorkflowReconcile(nil, nil); len(got) != 0 {
		t.Fatalf("empty inputs should reconcile to no rows, got %+v", got)
	}
	// Only non-workflow chat spend and no runs → nothing to reconcile.
	spend := []SpendRecord{{Model: "gpt-5", USD: 0.10, At: time.Now()}}
	if got := WorkflowReconcile(spend, nil); len(got) != 0 {
		t.Fatalf("non-workflow spend with no runs should reconcile to no rows, got %+v", got)
	}
}
