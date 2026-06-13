// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package telemetry

import (
	"sort"
	"time"
)

// This file adds a KEYED side-by-side comparison of two workflow runs — the
// Braintrust experiment-comparison pattern at run grain (issue 0094). Where
// RunAggregate rolls a workflow's whole history into one summary and LaneShare
// attributes cost per lane, RunDelta pits two individual runs of THE SAME workflow
// against each other: outcome, total credits, wall-clock duration and human pauses,
// with B−A deltas, and the two runs' lanes aligned BY INDEX. It is the local
// equivalent of "did the rerun do better, and what did it cost?" — explicitly a
// keyed-run comparison, NOT a structural span-tree diff (the industry diffs keyed
// runs, not trace trees). A pure reader: it returns ids and values, no display
// labels (the web layer resolves agent/workflow ids), and does no IO. — issue 0094.

// RunSide is one run's run-level snapshot within a RunDelta: which run, its outcome,
// and the totals the comparison pits against the other side. A is the baseline, B the
// run measured against it.
type RunSide struct {
	ID       string
	Outcome  string
	Credits  float64
	Duration time.Duration
	Pauses   int
}

// LaneSide is one run's lane at a given step index: present-or-not, the agent that
// ran it, its terminal status and its metered credits/pauses. Present is false when
// the run had no lane at that index (the two runs branched differently, or one has
// more steps) — its other fields are then zero, so the caller renders the gap rather
// than mistaking an absent lane for a free one.
type LaneSide struct {
	Present bool
	AgentID string
	Status  string
	Credits float64
	Pauses  int
}

// LaneDelta is one step's side-by-side slice of a RunDelta: the two runs' lane at the
// same Index (display "step Index+1") paired. CreditsDelta and PausesDelta are B−A
// over the present sides (an absent side contributes zero). Comparable directly with
// == (all fields are comparable) so tests can assert determinism.
type LaneDelta struct {
	Index        int
	A            LaneSide
	B            LaneSide
	CreditsDelta float64
	PausesDelta  int
}

// RunDelta is a keyed side-by-side comparison of two runs OF THE SAME workflow: each
// run's run-level snapshot (A the baseline, B the comparison), the B−A run-level
// deltas, and the two runs' lanes aligned by index. Comparable is false when the two
// runs are of different workflows — a comparison the caller refuses cleanly rather
// than rendering a meaningless cross-workflow diff; WorkflowID then stays empty (the
// sides still carry each run's id so the refusal can name the mismatch).
type RunDelta struct {
	WorkflowID    string
	Comparable    bool
	A             RunSide
	B             RunSide
	CreditsDelta  float64
	DurationDelta time.Duration
	PausesDelta   int
	Lanes         []LaneDelta
}

// CompareRuns builds the keyed side-by-side delta of two runs. It is KEYED on the
// workflow: when a and b are runs of the same workflow it sets Comparable and fills
// the shared WorkflowID, the run-level B−A deltas (credits, duration, pauses), and the
// per-lane deltas aligned by index; when they are runs of different workflows it
// returns Comparable=false with the two sides populated (so the caller can name the
// mismatch) and no lane alignment — comparing lanes across workflows would be
// meaningless. Lanes are aligned by their Index field (not slice position) over the
// UNION of the two runs' indices, ascending, so a run that branched differently still
// pairs cleanly with a gap on the missing side. Pure: same (a, b) → same RunDelta.
func CompareRuns(a, b RunRecord) RunDelta {
	d := RunDelta{
		A: runSide(a),
		B: runSide(b),
	}
	if a.WorkflowID != b.WorkflowID {
		// Different workflows: not a keyed comparison. Leave Comparable false and
		// WorkflowID empty; the sides are already populated for the refusal message.
		return d
	}
	d.Comparable = true
	d.WorkflowID = a.WorkflowID
	d.CreditsDelta = d.B.Credits - d.A.Credits
	d.DurationDelta = d.B.Duration - d.A.Duration
	d.PausesDelta = d.B.Pauses - d.A.Pauses
	d.Lanes = alignLanes(a.Lanes, b.Lanes)
	return d
}

// runSide snapshots a run's run-level totals for one side of a comparison.
func runSide(r RunRecord) RunSide {
	return RunSide{
		ID:       r.ID,
		Outcome:  r.Outcome,
		Credits:  r.Credits(),
		Duration: r.Duration(),
		Pauses:   r.TotalPauses(),
	}
}

// alignLanes pairs two runs' lanes by their Index field over the union of indices,
// ascending. A lane present in only one run yields a LaneDelta whose missing side has
// Present=false (zero fields); each delta is B−A over the present sides. Indexing by
// the Index field (not slice position) mirrors LaneShares, so an out-of-order or
// sparse lane slice still aligns correctly. Pure.
func alignLanes(as, bs []RunLane) []LaneDelta {
	byIdx := map[int]*LaneDelta{}
	order := []int{}
	at := func(idx int) *LaneDelta {
		ld, ok := byIdx[idx]
		if !ok {
			ld = &LaneDelta{Index: idx}
			byIdx[idx] = ld
			order = append(order, idx)
		}
		return ld
	}
	for _, l := range as {
		at(l.Index).A = laneSide(l)
	}
	for _, l := range bs {
		at(l.Index).B = laneSide(l)
	}
	sort.Ints(order)
	out := make([]LaneDelta, 0, len(order))
	for _, idx := range order {
		ld := byIdx[idx]
		ld.CreditsDelta = ld.B.Credits - ld.A.Credits
		ld.PausesDelta = ld.B.Pauses - ld.A.Pauses
		out = append(out, *ld)
	}
	return out
}

// laneSide snapshots one lane as a present side of a LaneDelta.
func laneSide(l RunLane) LaneSide {
	return LaneSide{
		Present: true,
		AgentID: l.AgentID,
		Status:  l.Status,
		Credits: l.Credits,
		Pauses:  l.Pauses,
	}
}
