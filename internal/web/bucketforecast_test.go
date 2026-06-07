package web

import (
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// TestTelemetryPageShowsBucketTrajectory guards the F3 surface: beside each
// per-agent / per-workflow share bar the Telemetry page renders that bucket's burn
// trajectory — a per-bucket rate + month-projection sentence in a `trajectory`
// cell, resolving ids → names like the share bars. A steady recent spend against an
// allowance must render the OK trajectory ("on pace for …").
func TestTelemetryPageShowsBucketTrajectory(t *testing.T) {
	s, store := newSpendServer(t)
	s.allowance = 10_000 // headroom, so each active bucket projects OK
	s.forge.Agents = []ctxforge.Agent{{ID: "builder", Name: "Builder", Model: "gpt-5"}}
	s.forge.Workflows = []ctxforge.Workflow{{ID: "review", Name: "Review", Mode: ctxforge.WorkflowSequential}}
	// A week of steady spend ending today, attributed to one agent and one workflow.
	for n := 0; n < 7; n++ {
		if err := store.Append(telemetry.SpendRecord{
			At: recentDay(n), Model: "gpt-5", USD: 0.02, AgentID: "builder", WorkflowID: "review", LaneIndex: 0,
		}); err != nil {
			t.Fatal(err)
		}
	}
	html := s.telemetryPartial()
	// Structure: a trajectory cell exists for the agent and the workflow bucket.
	if c := strings.Count(html, `class="trajectory`); c < 2 {
		t.Fatalf("expected a trajectory cell per agent+workflow bucket, got %d:\n%s", c, html)
	}
	// The OK projection frames a per-bucket pace, never an exhaustion date.
	if !strings.Contains(html, "on pace for") {
		t.Errorf("trajectory should frame the bucket's pace:\n%s", html)
	}
}

// TestTelemetryBucketTrajectoryIdle guards the degenerate per-bucket path: a bucket
// whose only spend predates the trailing window must render the Idle sentence, not
// a bogus rate/date.
func TestTelemetryBucketTrajectoryIdle(t *testing.T) {
	s, store := newSpendServer(t)
	s.allowance = 10_000
	s.forge.Agents = []ctxforge.Agent{{ID: "builder", Name: "Builder", Model: "gpt-5"}}
	// Only stale spend (20 days ago) — in month maybe, but well outside the 7-day window.
	if err := store.Append(telemetry.SpendRecord{At: recentDay(20), Model: "gpt-5", USD: 0.02, AgentID: "builder"}); err != nil {
		t.Fatal(err)
	}
	html := s.telemetryPartial()
	if !strings.Contains(html, "idle") {
		t.Errorf("a window-empty bucket should render the idle trajectory:\n%s", html)
	}
}

// TestTelemetryBucketTrajectoryNoBudget guards that with no allowance configured the
// per-bucket trajectory lines are absent (nothing to project toward) — the share
// bars still render, just no trajectory cell.
func TestTelemetryBucketTrajectoryNoBudget(t *testing.T) {
	s, store := newSpendServer(t)
	s.allowance = 0 // no budget
	s.forge.Agents = []ctxforge.Agent{{ID: "builder", Name: "Builder", Model: "gpt-5"}}
	if err := store.Append(telemetry.SpendRecord{At: recentDay(0), Model: "gpt-5", USD: 0.02, AgentID: "builder"}); err != nil {
		t.Fatal(err)
	}
	html := s.telemetryPartial()
	if strings.Contains(html, `class="trajectory`) {
		t.Errorf("no-budget page must not render a trajectory cell:\n%s", html)
	}
	// The share bar itself still renders.
	if !strings.Contains(html, "Cost by agent") {
		t.Errorf("share bars should still render with no budget:\n%s", html)
	}
}

// TestTelemetryBucketTrajectoryNoStoreNoPanic guards the all-absent case: with no
// spend store wired the page renders without a trajectory and without panicking.
func TestTelemetryBucketTrajectoryNoStoreNoPanic(t *testing.T) {
	s, _ := newTestServer() // no Spend wired (s.spend == nil)
	s.allowance = 10_000
	html := s.telemetryPartial()
	if strings.Contains(html, `class="trajectory`) {
		t.Errorf("no-store page must not render a trajectory cell:\n%s", html)
	}
}
