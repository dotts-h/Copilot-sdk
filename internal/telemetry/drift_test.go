// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package telemetry

import (
	"math"
	"testing"
	"time"
)

// ModelDrifts joins the price-book estimate to GitHub's reported cost per model
// over the REPORTED turns only (issue 0060): an unreported turn has no
// authoritative figure to compare against, so including its estimate would
// manufacture drift. It is counted (est-only coverage) but never compared.

func driftAt(d int) time.Time { return time.Date(2026, 6, d, 10, 0, 0, 0, time.UTC) }

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestModelDriftsJoinsReportedTurnsOnly(t *testing.T) {
	records := []SpendRecord{
		// gpt-5: one reported turn (estimate 5 cr vs reported 4 cr → delta +1) and
		// one unreported turn whose estimate must NOT pollute the comparison.
		{At: driftAt(1), Model: "gpt-5", USD: 0.05, AIU: 4},
		{At: driftAt(2), Model: "gpt-5", USD: 0.10},
		// a wholly-unreported model has no reported figure to drift from — omitted.
		{At: driftAt(3), Model: "claude-sonnet-4-6", USD: 0.02},
	}
	got := ModelDrifts(records)
	if len(got) != 1 {
		t.Fatalf("expected 1 drift row (gpt-5 only), got %d: %+v", len(got), got)
	}
	d := got[0]
	if d.Model != "gpt-5" {
		t.Fatalf("expected the gpt-5 row, got %q", d.Model)
	}
	if !near(d.EstimateCredits, 5) || !near(d.ReportedCredits, 4) || !near(d.Delta, 1) {
		t.Errorf("estimate/reported/delta = %v/%v/%v, want 5/4/1", d.EstimateCredits, d.ReportedCredits, d.Delta)
	}
	if d.ReportedTurns != 1 || d.UnreportedTurns != 1 {
		t.Errorf("coverage = %d reported / %d unreported, want 1/1", d.ReportedTurns, d.UnreportedTurns)
	}
}

func TestModelDriftsDeltaSign(t *testing.T) {
	// Delta is EstimateCredits − ReportedCredits: negative when the price book
	// under-estimates what GitHub actually billed (the epic-0050 audit finding).
	got := ModelDrifts([]SpendRecord{{At: driftAt(1), Model: "gpt-5", USD: 0.02, AIU: 2.5}})
	if len(got) != 1 || !near(got[0].Delta, -0.5) {
		t.Fatalf("expected delta -0.5 (estimate 2 under reported 2.5), got %+v", got)
	}
}

func TestModelDriftsOrderIsDeterministic(t *testing.T) {
	// |delta| desc, then estimate desc, then model asc — a total order over the
	// unique model key, mirroring the sibling reconcile readers.
	records := []SpendRecord{
		{At: driftAt(1), Model: "m-b", USD: 0.05, AIU: 4}, // delta +1, est 5
		{At: driftAt(1), Model: "m-d", USD: 0.03, AIU: 2}, // delta +1, est 3
		{At: driftAt(1), Model: "m-a", USD: 0.02, AIU: 4}, // delta −2, est 2
		{At: driftAt(1), Model: "m-c", USD: 0.03, AIU: 2}, // delta +1, est 3
	}
	got := ModelDrifts(records)
	want := []string{"m-a", "m-b", "m-c", "m-d"}
	if len(got) != len(want) {
		t.Fatalf("expected %d rows, got %d: %+v", len(want), len(got), got)
	}
	for i, w := range want {
		if got[i].Model != w {
			t.Fatalf("row %d = %q, want %q (full order %+v)", i, got[i].Model, w, got)
		}
	}
}

func TestModelDriftsEmpty(t *testing.T) {
	if got := ModelDrifts(nil); len(got) != 0 {
		t.Fatalf("nil records should yield no rows, got %+v", got)
	}
	// All-unreported input: nothing is comparable, so nothing to show.
	unreported := []SpendRecord{{At: driftAt(1), Model: "gpt-5", USD: 0.05}}
	if got := ModelDrifts(unreported); len(got) != 0 {
		t.Fatalf("all-unreported records should yield no rows, got %+v", got)
	}
}

func TestModelDriftDriftedEpsilon(t *testing.T) {
	for _, tc := range []struct {
		delta float64
		want  bool
	}{
		{0.005, true},   // at epsilon — a delta that displays non-zero is real
		{-0.005, true},  // magnitude, not sign
		{0.0049, false}, // below epsilon — float noise, not divergence
		{0, false},
	} {
		d := ModelDrift{Delta: tc.delta}
		if got := d.Drifted(0.005); got != tc.want {
			t.Errorf("Drifted(0.005) with delta %v = %v, want %v", tc.delta, got, tc.want)
		}
	}
}
