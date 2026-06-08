package telemetry

import (
	"bytes"
	"encoding/csv"
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

func TestLaneReconcileJoinsBothStoresPerLane(t *testing.T) {
	// LaneReconcile is the per-(workflow, lane) cousin of WorkflowReconcile — one grain
	// finer. It joins the ledger's lane-tagged spend (SpendRecord grouped by WorkflowID +
	// LaneIndex, the LaneShares ledger cousin) against the run history's per-lane credits
	// (RunLane.Credits, the LaneShares grain) so a divergence is locatable at the exact
	// lane, not just the workflow total. Input order is shuffled to prove the join is
	// order-independent.
	spend := []SpendRecord{
		// alpha lane 0: 2.00 cr ($0.02); alpha lane 1: 1.00 cr ($0.01).
		{Model: "gpt-5", USD: 0.02, WorkflowID: "alpha", LaneIndex: 0},
		{Model: "gpt-5", USD: 0.01, WorkflowID: "alpha", LaneIndex: 1},
		// beta lane 0: 5.00 cr ($0.05).
		{Model: "gpt-5", USD: 0.05, WorkflowID: "beta", LaneIndex: 0},
		// plain chat (no workflow) — excluded from the per-lane reconciliation.
		{Model: "gpt-5", USD: 0.10},
	}
	runs := []RunRecord{
		// alpha: its two lanes metered exactly what the ledger attributes (delta 0 each).
		{WorkflowID: "alpha", Name: "Alpha", Lanes: []RunLane{
			{Index: 0, Status: "done", Credits: 2},
			{Index: 1, Status: "done", Credits: 1},
		}},
		// beta: lane 0 metered only 4.00 cr — diverges from the ledger's 5.00, so its
		// delta is +1.00 (a turn metered outside this lane's run).
		{WorkflowID: "beta", Name: "Beta", Lanes: []RunLane{
			{Index: 0, Status: "done", Credits: 4},
		}},
	}
	got := LaneReconcile(spend, runs)
	if len(got) != 3 {
		t.Fatalf("want 3 per-lane reconciliation rows, got %d: %+v", len(got), got)
	}
	// Sorted by absolute delta descending: (beta,0) (|+1.00|) first, then the two
	// agreeing alpha lanes (delta 0) by ledger credits desc — (alpha,0) ledger 2 before
	// (alpha,1) ledger 1.
	if got[0].WorkflowID != "beta" || got[0].LaneIndex != 0 {
		t.Fatalf("row[0] = %+v, want (beta, lane 0) — the divergent lane — first", got[0])
	}
	approx(t, got[0].LedgerCredits, 5)
	approx(t, got[0].RunCredits, 4)
	approx(t, got[0].Delta, 1)
	if got[1].WorkflowID != "alpha" || got[1].LaneIndex != 0 {
		t.Fatalf("row[1] = %+v, want (alpha, lane 0)", got[1])
	}
	approx(t, got[1].LedgerCredits, 2)
	approx(t, got[1].RunCredits, 2)
	approx(t, got[1].Delta, 0)
	if got[2].WorkflowID != "alpha" || got[2].LaneIndex != 1 {
		t.Fatalf("row[2] = %+v, want (alpha, lane 1)", got[2])
	}
	approx(t, got[2].LedgerCredits, 1)
	approx(t, got[2].RunCredits, 1)
	approx(t, got[2].Delta, 0)
}

func TestLaneReconcileOneSidedLanesAppear(t *testing.T) {
	// A (workflow, lane) present in ONE store but not the other still appears, with the
	// other side zero and the delta its full magnitude. A run's SKIPPED lane that metered
	// nothing and has no matching ledger spend is the both-zero case — nothing to
	// reconcile — and is omitted (a row needs a non-zero side, like the per-workflow
	// reader never emits a workflow with both stores at zero).
	spend := []SpendRecord{
		// ledger-only lane: spent at (ledger-only, lane 2) but never recorded a run lane.
		{Model: "gpt-5", USD: 0.07, WorkflowID: "ledger-only", LaneIndex: 2},
	}
	runs := []RunRecord{
		// run-only: lane 3 metered 2.00 cr; lane 4 was skipped (0 cr) with no ledger spend.
		{WorkflowID: "run-only", Lanes: []RunLane{
			{Index: 3, Status: "done", Credits: 2},
			{Index: 4, Status: "skipped"},
		}},
	}
	got := LaneReconcile(spend, runs)
	if len(got) != 2 {
		t.Fatalf("want 2 rows (the skipped both-zero lane omitted), got %d: %+v", len(got), got)
	}
	// (ledger-only, 2) has the larger |delta| (7 vs 2), so it sorts first.
	if got[0].WorkflowID != "ledger-only" || got[0].LaneIndex != 2 {
		t.Fatalf("row[0] = %+v, want (ledger-only, lane 2) first (larger |delta|)", got[0])
	}
	approx(t, got[0].LedgerCredits, 7)
	approx(t, got[0].RunCredits, 0)
	approx(t, got[0].Delta, 7)
	if got[1].WorkflowID != "run-only" || got[1].LaneIndex != 3 {
		t.Fatalf("row[1] = %+v, want (run-only, lane 3)", got[1])
	}
	approx(t, got[1].LedgerCredits, 0)
	approx(t, got[1].RunCredits, 2)
	approx(t, got[1].Delta, -2)
}

func TestLaneReconcileDeterministicOrder(t *testing.T) {
	// Rows with EQUAL |delta| tie-break by ledger credits descending, then workflow id
	// ascending, then lane index ascending — a total order over the unique (workflow,
	// lane) key, regardless of input order. Here every lane reconciles exactly (delta 0),
	// so the full tie-break runs.
	mk := func(wf string, lane int, cr float64) ([]SpendRecord, []RunRecord) {
		return []SpendRecord{{Model: "gpt-5", USD: cr * USDPerCredit, WorkflowID: wf, LaneIndex: lane}},
			[]RunRecord{{WorkflowID: wf, Lanes: []RunLane{{Index: lane, Status: "done", Credits: cr}}}}
	}
	var sp []SpendRecord
	var rn []RunRecord
	for _, e := range []struct {
		wf   string
		lane int
		cr   float64
	}{{"zeta", 0, 1}, {"alpha", 1, 3}, {"gamma", 0, 3}, {"alpha", 0, 3}} {
		s, r := mk(e.wf, e.lane, e.cr)
		sp = append(sp, s...)
		rn = append(rn, r...)
	}
	got := LaneReconcile(sp, rn)
	if len(got) != 4 {
		t.Fatalf("want 4 rows, got %d", len(got))
	}
	// All delta 0 → ledger desc (3,3,3,1); among the ledger-3 lanes, id asc then lane asc:
	// (alpha,0), (alpha,1), (gamma,0); then (zeta,0).
	want := []struct {
		wf   string
		lane int
	}{{"alpha", 0}, {"alpha", 1}, {"gamma", 0}, {"zeta", 0}}
	for i, w := range want {
		if got[i].WorkflowID != w.wf || got[i].LaneIndex != w.lane {
			t.Fatalf("row[%d] = %+v, want (%s, lane %d) — equal-|delta| ties sort by ledger desc, id asc, lane asc", i, got[i], w.wf, w.lane)
		}
	}
}

func TestLaneReconcileEmpty(t *testing.T) {
	if got := LaneReconcile(nil, nil); len(got) != 0 {
		t.Fatalf("empty inputs should reconcile to no rows, got %+v", got)
	}
	// Non-workflow chat spend plus a run that only skipped a lane (zero on both sides) →
	// nothing to reconcile.
	spend := []SpendRecord{{Model: "gpt-5", USD: 0.10, At: time.Now()}}
	runs := []RunRecord{{WorkflowID: "wf", Lanes: []RunLane{{Index: 0, Status: "skipped"}}}}
	if got := LaneReconcile(spend, runs); len(got) != 0 {
		t.Fatalf("no spend or run credits on any lane should reconcile to no rows, got %+v", got)
	}
}

func TestWriteReconcileCSV(t *testing.T) {
	// WriteReconcileCSV serializes the cross-store reconciliation to CSV — the export
	// sibling of WriteCSV/WriteRunsCSV — so the ledger-vs-runs divergence can leave the
	// tool the way spend and runs already do. One file carries BOTH grains: the per-
	// workflow rows (V15) first, then the per-(workflow, lane) rows (V16), each labelled
	// by a leading `grain` column ("workflow" | "lane") so a consumer never double-counts
	// a total against its breakdown. Rows are the readers' own output, so the order is
	// deterministic and the biggest delta reads first within each grain.
	spend := []SpendRecord{
		{Model: "gpt-5", USD: 0.02, WorkflowID: "alpha", LaneIndex: 0},
		{Model: "gpt-5", USD: 0.01, WorkflowID: "alpha", LaneIndex: 1},
		{Model: "gpt-5", USD: 0.05, WorkflowID: "beta", LaneIndex: 0},
		{Model: "gpt-5", USD: 0.10}, // plain chat — excluded from reconciliation
	}
	runs := []RunRecord{
		{WorkflowID: "alpha", Name: "Alpha", Lanes: []RunLane{
			{Index: 0, Status: "done", Credits: 2},
			{Index: 1, Status: "done", Credits: 1},
		}},
		// beta's run meters 4.00 cr vs the ledger's 5.00 — a +1.00 delta on both grains.
		{WorkflowID: "beta", Name: "Beta", Lanes: []RunLane{
			{Index: 0, Status: "done", Credits: 4},
		}},
	}
	var buf bytes.Buffer
	if err := WriteReconcileCSV(&buf, spend, runs); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("CSV is not parseable: %v", err)
	}
	// header + 2 per-workflow rows (alpha, beta) + 3 per-lane rows (alpha/0, alpha/1, beta/0).
	if len(rows) != 6 {
		t.Fatalf("want header + 2 workflow + 3 lane rows, got %d: %+v", len(rows), rows)
	}
	want := []string{"grain", "workflow", "lane", "ledgerCredits", "runCredits", "delta"}
	if len(rows[0]) != len(want) {
		t.Fatalf("header width = %d, want %d: %+v", len(rows[0]), len(want), rows[0])
	}
	for i, h := range want {
		if rows[0][i] != h {
			t.Fatalf("header[%d] = %q, want %q", i, rows[0][i], h)
		}
	}
	// Per-workflow grain first, biggest |delta| leading: beta (+1) before alpha (0); the
	// grain cell is "workflow" and the lane cell is blank at this grain.
	if got := rows[1]; got[0] != "workflow" || got[1] != "beta" || got[2] != "" || got[3] != "5" || got[4] != "4" || got[5] != "1" {
		t.Fatalf("workflow row[1] = %+v, want [workflow, beta, \"\", 5, 4, 1]", got)
	}
	if got := rows[2]; got[0] != "workflow" || got[1] != "alpha" || got[2] != "" || got[5] != "0" {
		t.Fatalf("workflow row[2] = %+v, want workflow/alpha with delta 0 and blank lane", got)
	}
	// Per-lane grain follows, grain "lane" and the lane index filled in.
	if got := rows[3]; got[0] != "lane" || got[1] != "beta" || got[2] != "0" || got[3] != "5" || got[4] != "4" || got[5] != "1" {
		t.Fatalf("lane row[3] = %+v, want [lane, beta, 0, 5, 4, 1]", got)
	}
	if got := rows[4]; got[0] != "lane" || got[1] != "alpha" || got[2] != "0" || got[3] != "2" {
		t.Fatalf("lane row[4] = %+v, want lane/(alpha, lane 0) ledger 2", got)
	}
}

func TestWriteReconcileCSVHeaderOnlyWhenEmpty(t *testing.T) {
	// Nothing to reconcile (no records, or only non-workflow chat spend) exports the
	// header alone — never a bare/empty body — mirroring the spend and runs exports.
	var buf bytes.Buffer
	if err := WriteReconcileCSV(&buf, nil, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("CSV is not parseable: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("empty reconciliation exports the header only, got %d rows", len(rows))
	}
	if rows[0][0] != "grain" {
		t.Fatalf("header should lead with the grain column, got %+v", rows[0])
	}
}
