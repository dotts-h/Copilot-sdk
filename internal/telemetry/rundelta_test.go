// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package telemetry

import (
	"testing"
	"time"
)

// runAt builds a one-off RunRecord for the comparison tests: a workflow id, an
// outcome, a started/finished span, and its lanes — enough to exercise the run-level
// totals and the per-lane alignment without the store machinery.
func runAt(id, workflow, outcome string, started time.Time, dur time.Duration, lanes ...RunLane) RunRecord {
	return RunRecord{
		ID:         id,
		WorkflowID: workflow,
		Name:       workflow,
		Mode:       "sequential",
		StartedAt:  started,
		FinishedAt: started.Add(dur),
		Outcome:    outcome,
		Lanes:      lanes,
	}
}

// CompareRuns is KEYED on the workflow: a run and its rerun (same workflow) compare;
// two runs of DIFFERENT workflows are not comparable, and the reader says so cleanly
// (Comparable == false) rather than emitting a meaningless cross-workflow diff. The
// shared workflow id rides along only when comparable.
func TestCompareRunsKeyedOnWorkflow(t *testing.T) {
	base := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	a := runAt("run-a", "build", "finished", base, time.Minute)
	b := runAt("run-b", "build", "finished", base.Add(time.Hour), 2*time.Minute)

	d := CompareRuns(a, b)
	if !d.Comparable {
		t.Fatal("same-workflow runs should be Comparable")
	}
	if d.WorkflowID != "build" {
		t.Errorf("WorkflowID = %q, want build", d.WorkflowID)
	}

	other := runAt("run-c", "ship", "finished", base, time.Minute)
	x := CompareRuns(a, other)
	if x.Comparable {
		t.Error("cross-workflow runs must not be Comparable")
	}
	if x.WorkflowID != "" {
		t.Errorf("cross-workflow WorkflowID = %q, want empty", x.WorkflowID)
	}
	// Even when refused, the two sides are still populated so the caller can name the
	// mismatched workflows in its refusal.
	if x.A.ID != "run-a" || x.B.ID != "run-c" {
		t.Errorf("sides = %q/%q, want run-a/run-c", x.A.ID, x.B.ID)
	}
}

// The run-level deltas are B−A: B (the rerun, the "new" definition) measured against A
// (the baseline). A positive credits/duration delta means the rerun cost more / took
// longer. Outcome is carried verbatim on each side (no delta — it isn't numeric).
func TestCompareRunsRunLevelDeltas(t *testing.T) {
	base := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	a := runAt("run-a", "build", "failed", base, time.Minute,
		RunLane{Index: 0, AgentID: "builder", Status: "failed", Credits: 1.0, Pauses: 1, PausedMs: 5000},
	)
	b := runAt("run-b", "build", "finished", base.Add(time.Hour), 90*time.Second,
		RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 2.5, Pauses: 0},
	)

	d := CompareRuns(a, b)
	if d.A.Outcome != "failed" || d.B.Outcome != "finished" {
		t.Errorf("outcomes = %q/%q, want failed/finished", d.A.Outcome, d.B.Outcome)
	}
	if d.A.Credits != 1.0 || d.B.Credits != 2.5 {
		t.Errorf("credits = %v/%v, want 1/2.5", d.A.Credits, d.B.Credits)
	}
	if d.CreditsDelta != 1.5 {
		t.Errorf("CreditsDelta = %v, want 1.5", d.CreditsDelta)
	}
	if d.DurationDelta != 30*time.Second {
		t.Errorf("DurationDelta = %v, want 30s", d.DurationDelta)
	}
	if d.A.Pauses != 1 || d.B.Pauses != 0 || d.PausesDelta != -1 {
		t.Errorf("pauses = %d/%d delta %d, want 1/0 delta -1", d.A.Pauses, d.B.Pauses, d.PausesDelta)
	}
}

// Lanes align BY INDEX (step N of A against step N of B), in ascending order — a
// deterministic pairing regardless of slice order. Each lane's credits/pauses delta
// is B−A; status and agent are carried verbatim per side.
func TestCompareRunsLanesAlignByIndex(t *testing.T) {
	base := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	// Lanes deliberately out of slice order to prove alignment is by Index, not position.
	a := runAt("run-a", "build", "finished", base, time.Minute,
		RunLane{Index: 1, AgentID: "sdet", Status: "done", Credits: 2.0, Pauses: 0},
		RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 1.0, Pauses: 2},
	)
	b := runAt("run-b", "build", "finished", base, time.Minute,
		RunLane{Index: 0, AgentID: "builder", Status: "failed", Credits: 1.5, Pauses: 1},
		RunLane{Index: 1, AgentID: "sdet", Status: "done", Credits: 2.0, Pauses: 0},
	)

	d := CompareRuns(a, b)
	if len(d.Lanes) != 2 {
		t.Fatalf("len(Lanes) = %d, want 2", len(d.Lanes))
	}
	if d.Lanes[0].Index != 0 || d.Lanes[1].Index != 1 {
		t.Fatalf("lane indices = %d,%d, want 0,1 (ascending)", d.Lanes[0].Index, d.Lanes[1].Index)
	}
	l0 := d.Lanes[0]
	if !l0.A.Present || !l0.B.Present {
		t.Error("lane 0 should be present on both sides")
	}
	if l0.A.Status != "done" || l0.B.Status != "failed" {
		t.Errorf("lane 0 statuses = %q/%q, want done/failed", l0.A.Status, l0.B.Status)
	}
	if l0.CreditsDelta != 0.5 {
		t.Errorf("lane 0 CreditsDelta = %v, want 0.5", l0.CreditsDelta)
	}
	if l0.PausesDelta != -1 {
		t.Errorf("lane 0 PausesDelta = %d, want -1", l0.PausesDelta)
	}
}

// A lane present in only ONE run (the runs branched differently, or one has more
// steps) carries Present=false on the missing side, its fields zero — the caller
// renders the gap rather than a bogus zero-delta. The delta is over the present side
// alone (a missing side contributes zero).
func TestCompareRunsMismatchedLaneCount(t *testing.T) {
	base := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	a := runAt("run-a", "build", "finished", base, time.Minute,
		RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 1.0},
	)
	b := runAt("run-b", "build", "finished", base, time.Minute,
		RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 1.2},
		RunLane{Index: 1, AgentID: "sdet", Status: "done", Credits: 2.0},
	)

	d := CompareRuns(a, b)
	if len(d.Lanes) != 2 {
		t.Fatalf("len(Lanes) = %d, want 2 (union of indices)", len(d.Lanes))
	}
	l1 := d.Lanes[1]
	if l1.Index != 1 {
		t.Fatalf("lane[1].Index = %d, want 1", l1.Index)
	}
	if l1.A.Present {
		t.Error("lane 1 should be absent on A")
	}
	if !l1.B.Present {
		t.Error("lane 1 should be present on B")
	}
	if l1.B.Credits != 2.0 || l1.CreditsDelta != 2.0 {
		t.Errorf("lane 1 B.Credits=%v delta=%v, want 2/2", l1.B.Credits, l1.CreditsDelta)
	}
}

// Pure: comparing the same pair of records twice yields an identical delta — same
// inputs, same output, no hidden state.
func TestCompareRunsDeterministic(t *testing.T) {
	base := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	a := runAt("run-a", "build", "finished", base, time.Minute,
		RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 1.0},
		RunLane{Index: 2, AgentID: "shipper", Status: "skipped", Credits: 0},
	)
	b := runAt("run-b", "build", "finished", base, 2*time.Minute,
		RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 1.1},
		RunLane{Index: 1, AgentID: "sdet", Status: "done", Credits: 2.0},
	)
	d1 := CompareRuns(a, b)
	d2 := CompareRuns(a, b)
	if len(d1.Lanes) != len(d2.Lanes) {
		t.Fatalf("lane counts differ: %d vs %d", len(d1.Lanes), len(d2.Lanes))
	}
	for i := range d1.Lanes {
		if d1.Lanes[i] != d2.Lanes[i] {
			t.Errorf("lane %d differs between runs: %+v vs %+v", i, d1.Lanes[i], d2.Lanes[i])
		}
	}
	// Union of indices {0,1,2} → three aligned lanes, ascending.
	if len(d1.Lanes) != 3 || d1.Lanes[0].Index != 0 || d1.Lanes[2].Index != 2 {
		t.Errorf("lanes = %d entries, indices %v; want 3 ascending [0,1,2]", len(d1.Lanes), laneIdxs(d1.Lanes))
	}
}

func laneIdxs(ls []LaneDelta) []int {
	out := make([]int, len(ls))
	for i, l := range ls {
		out[i] = l.Index
	}
	return out
}
