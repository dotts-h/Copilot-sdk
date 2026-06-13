// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package telemetry

import (
	"testing"
)

// agentOf / workflowOf are the bucket keys F3 projects per (mirroring shareBy's
// keyOf), so the tests read the same way the web layer wires them.
func agentOf(r SpendRecord) string    { return r.AgentID }
func workflowOf(r SpendRecord) string { return r.WorkflowID }

// spendOn builds a tagged ledger record on a UTC day at a credit cost, so a test
// can lay down a per-bucket daily series the way the persisted ledger holds it.
func spendOn(d string, credits float64, agent, workflow string) SpendRecord {
	return SpendRecord{
		At:         day(d + "T10:00:00Z"),
		Model:      "gpt-5",
		USD:        credits * USDPerCredit,
		AgentID:    agent,
		WorkflowID: workflow,
	}
}

// TestBucketForecastsSplitsAndRunsSlopePerBucket is the core F3 guarantee: a
// mixed-tag ledger is split into per-agent buckets and the SAME Forecast slope
// runs over each bucket's OWN daily series — so a heavy bucket projects a steeper
// rate than a light one, and the result is sorted by spend descending.
func TestBucketForecastsSplitsAndRunsSlopePerBucket(t *testing.T) {
	now := day("2026-06-15T12:00:00Z")
	budget := Budget{AllowanceCredits: 1000}

	var records []SpendRecord
	// heavy: 10 cr/day across the whole trailing window (09..15) → DailyRate 10.
	// light: 1 cr/day across the same window → DailyRate 1.
	for d := 9; d <= 15; d++ {
		day := "2026-06-" + pad(d)
		records = append(records,
			spendOn(day, 10, "heavy", ""),
			spendOn(day, 1, "light", ""),
		)
	}

	got := BucketForecasts(records, budget, now, agentOf, true)
	if len(got) != 2 {
		t.Fatalf("want 2 agent buckets, got %d: %+v", len(got), got)
	}
	// Sorted by spend descending: heavy first.
	if got[0].Key != "heavy" || got[1].Key != "light" {
		t.Fatalf("buckets not sorted by spend desc: %q then %q", got[0].Key, got[1].Key)
	}
	if got[0].Projection.Status != ProjectionOK || got[1].Projection.Status != ProjectionOK {
		t.Fatalf("both active buckets should project OK: %+v", got)
	}
	approx(t, got[0].Projection.DailyRate, 10)
	approx(t, got[1].Projection.DailyRate, 1)
	if got[0].Projection.DailyRate <= got[1].Projection.DailyRate {
		t.Fatalf("heavy bucket must project a steeper rate than light: %v vs %v",
			got[0].Projection.DailyRate, got[1].Projection.DailyRate)
	}
}

// TestBucketForecastsClampsSingleDayBucket guards that a one-day / too-new bucket
// clamps its denominator to the bucket's own ledger age (like A3 account-wide),
// not the full 7-day window — so a single busy day projects a real rate, never a
// near-zero one divided by a mostly-absent week.
func TestBucketForecastsClampsSingleDayBucket(t *testing.T) {
	now := day("2026-06-15T12:00:00Z")
	budget := Budget{AllowanceCredits: 1000}
	// "fresh" has a single day of 12 cr (today); "old" has a week of history.
	records := []SpendRecord{spendOn("2026-06-15", 12, "fresh", "")}
	for d := 9; d <= 15; d++ {
		records = append(records, spendOn("2026-06-"+pad(d), 5, "old", ""))
	}

	got := BucketForecasts(records, budget, now, agentOf, true)
	byKey := map[string]Projection{}
	for _, b := range got {
		byKey[b.Key] = b.Projection
	}
	// fresh: 12 cr / 1 observed day, NOT 12/7.
	approx(t, byKey["fresh"].DailyRate, 12)
	// old: 35 cr / 7 observed days.
	approx(t, byKey["old"].DailyRate, 5)
}

// TestBucketForecastsIdleBucket guards the degenerate per-bucket path: a bucket
// whose only spend predates the trailing window projects ProjectionIdle, never a
// bogus rate/date — exactly the A3 idle case, applied per bucket.
func TestBucketForecastsIdleBucket(t *testing.T) {
	now := day("2026-06-15T12:00:00Z")
	budget := Budget{AllowanceCredits: 1000}
	records := []SpendRecord{
		spendOn("2026-06-15", 8, "active", ""), // in window
		spendOn("2026-06-01", 8, "stale", ""),  // in month, before the window
	}
	got := BucketForecasts(records, budget, now, agentOf, true)
	byKey := map[string]Projection{}
	for _, b := range got {
		byKey[b.Key] = b.Projection
	}
	if byKey["active"].Status != ProjectionOK {
		t.Fatalf("active bucket should be OK, got %v", byKey["active"].Status)
	}
	if byKey["stale"].Status != ProjectionIdle {
		t.Fatalf("stale (window-empty) bucket should be Idle, got %v", byKey["stale"].Status)
	}
}

// TestBucketForecastsNoBudget guards that with no allowance every bucket carries
// ProjectionNoBudget — the web layer renders no trajectory line for them.
func TestBucketForecastsNoBudget(t *testing.T) {
	now := day("2026-06-15T12:00:00Z")
	records := []SpendRecord{spendOn("2026-06-15", 5, "a", ""), spendOn("2026-06-15", 3, "b", "")}
	got := BucketForecasts(records, Budget{AllowanceCredits: 0}, now, agentOf, true)
	if len(got) != 2 {
		t.Fatalf("want 2 buckets, got %d", len(got))
	}
	for _, b := range got {
		if b.Projection.Status != ProjectionNoBudget {
			t.Fatalf("bucket %q should be NoBudget with zero allowance, got %v", b.Key, b.Projection.Status)
		}
	}
}

// TestBucketForecastsEmptyLedger guards the empty case: no records → no buckets.
func TestBucketForecastsEmptyLedger(t *testing.T) {
	now := day("2026-06-15T12:00:00Z")
	if got := BucketForecasts(nil, Budget{AllowanceCredits: 100}, now, agentOf, true); len(got) != 0 {
		t.Fatalf("empty ledger should yield no buckets, got %d", len(got))
	}
}

// TestBucketForecastsWorkflowExcludesEmpty guards that the workflow bucketing skips
// the empty-key (non-workflow) spend when includeEmpty is false — mirroring
// WorkflowShares, so chat spend doesn't masquerade as a workflow trajectory.
func TestBucketForecastsWorkflowExcludesEmpty(t *testing.T) {
	now := day("2026-06-15T12:00:00Z")
	budget := Budget{AllowanceCredits: 1000}
	records := []SpendRecord{
		spendOn("2026-06-15", 5, "builder", "review"), // workflow-owned
		spendOn("2026-06-15", 9, "builder", ""),       // plain chat, no workflow
	}
	got := BucketForecasts(records, budget, now, workflowOf, false)
	if len(got) != 1 || got[0].Key != "review" {
		t.Fatalf("workflow buckets should exclude the empty key, got %+v", got)
	}
}

// TestBucketForecastsDeterministic guards the package-wide purity invariant: the
// same records always bucket into the same order with identical projections,
// regardless of input order.
func TestBucketForecastsDeterministic(t *testing.T) {
	now := day("2026-06-15T12:00:00Z")
	budget := Budget{AllowanceCredits: 1000}
	a := []SpendRecord{
		spendOn("2026-06-14", 4, "x", ""),
		spendOn("2026-06-15", 6, "y", ""),
		spendOn("2026-06-15", 6, "x", ""),
	}
	// Same records, shuffled order.
	b := []SpendRecord{a[2], a[0], a[1]}
	g1 := BucketForecasts(a, budget, now, agentOf, true)
	g2 := BucketForecasts(b, budget, now, agentOf, true)
	if len(g1) != len(g2) {
		t.Fatalf("length differs: %d vs %d", len(g1), len(g2))
	}
	for i := range g1 {
		if g1[i].Key != g2[i].Key || g1[i].Projection != g2[i].Projection || g1[i].Credits != g2[i].Credits {
			t.Fatalf("non-deterministic at %d: %+v vs %+v", i, g1[i], g2[i])
		}
	}
}

// pad renders a day-of-month as a zero-padded two-digit string for a date key.
func pad(d int) string {
	if d < 10 {
		return "0" + string(rune('0'+d))
	}
	return string(rune('0'+d/10)) + string(rune('0'+d%10))
}
