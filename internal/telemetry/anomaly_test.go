// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package telemetry

import "testing"

// mkRun builds a RunRecord of the given workflow with a single lane carrying the
// given credits — enough to exercise the credit-cohort math (Credits() sums lanes).
func mkRun(id, workflow string, credits float64) RunRecord {
	return RunRecord{
		ID: id, WorkflowID: workflow, Name: workflow,
		Lanes: []RunLane{{Index: 0, Credits: credits}},
	}
}

// A run that costs far more than its workflow's typical run is flagged, with the
// factor it exceeded the baseline by. Quiet cohort-mates are not flagged.
func TestDetectAnomaliesFlagsOutlier(t *testing.T) {
	records := []RunRecord{
		mkRun("r1", "wf", 1),
		mkRun("r2", "wf", 1),
		mkRun("r3", "wf", 1),
		mkRun("r4", "wf", 1),
		mkRun("r5", "wf", 12), // the runaway: 12× the median
	}
	got := DetectAnomalies(records, AnomalyOpts{Factor: 3, MinCohort: 3})
	if len(got) != 1 {
		t.Fatalf("exactly the outlier should be flagged, got %d: %+v", len(got), got)
	}
	a := got[0]
	if a.RunID != "r5" {
		t.Errorf("the 12-credit run is the anomaly, got %q", a.RunID)
	}
	if a.Baseline != 1 {
		t.Errorf("baseline is the cohort median (1), got %v", a.Baseline)
	}
	if a.Factor != 12 {
		t.Errorf("factor is credits/baseline (12), got %v", a.Factor)
	}
}

// The factor threshold is inclusive (>=): a run exactly at Factor× the baseline
// is flagged — the boundary value is the first anomalous one.
func TestDetectAnomaliesFactorInclusive(t *testing.T) {
	records := []RunRecord{
		mkRun("a", "wf", 2), mkRun("b", "wf", 2), mkRun("c", "wf", 2), mkRun("d", "wf", 6),
	}
	got := DetectAnomalies(records, AnomalyOpts{Factor: 3, MinCohort: 3})
	if len(got) != 1 || got[0].RunID != "d" {
		t.Fatalf("a run exactly at 3× the median should be flagged, got %+v", got)
	}
}

// A workflow with fewer than MinCohort runs has too little history to call any of
// its runs anomalous — no flags, even with a large spread.
func TestDetectAnomaliesNeedsCohort(t *testing.T) {
	records := []RunRecord{mkRun("a", "wf", 1), mkRun("b", "wf", 100)}
	if got := DetectAnomalies(records, AnomalyOpts{Factor: 3, MinCohort: 3}); len(got) != 0 {
		t.Fatalf("a sub-cohort workflow yields no anomaly, got %+v", got)
	}
}

// A free (all-zero) cohort has a zero baseline — no ratio to compute, so nothing
// is flagged (no division by zero, no spurious infinite factor).
func TestDetectAnomaliesZeroBaseline(t *testing.T) {
	records := []RunRecord{mkRun("a", "wf", 0), mkRun("b", "wf", 0), mkRun("c", "wf", 0)}
	if got := DetectAnomalies(records, AnomalyOpts{Factor: 3, MinCohort: 3}); len(got) != 0 {
		t.Fatalf("a zero-baseline cohort yields no anomaly, got %+v", got)
	}
}

// Anomalies are scoped per workflow: a run is compared only against runs of the
// same workflow, and the result is deterministic (worst factor first).
func TestDetectAnomaliesPerWorkflowAndSorted(t *testing.T) {
	records := []RunRecord{
		mkRun("x1", "alpha", 1), mkRun("x2", "alpha", 1), mkRun("x3", "alpha", 1), mkRun("x4", "alpha", 5),
		mkRun("y1", "beta", 2), mkRun("y2", "beta", 2), mkRun("y3", "beta", 2), mkRun("y4", "beta", 20),
	}
	got := DetectAnomalies(records, AnomalyOpts{Factor: 3, MinCohort: 3})
	if len(got) != 2 {
		t.Fatalf("one anomaly per workflow, got %d: %+v", len(got), got)
	}
	// beta's run (10×) outranks alpha's (5×): worst first.
	if got[0].RunID != "y4" || got[1].RunID != "x4" {
		t.Errorf("anomalies should be sorted worst-factor-first, got %q then %q", got[0].RunID, got[1].RunID)
	}
}

// DefaultAnomalyOpts supplies the sane single-user defaults so callers needn't
// re-derive them.
func TestDefaultAnomalyOpts(t *testing.T) {
	o := DefaultAnomalyOpts()
	if o.Factor <= 1 || o.MinCohort < 2 {
		t.Errorf("defaults should be a meaningful multiple over a real cohort, got %+v", o)
	}
}
