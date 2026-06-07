package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// withRunStore attaches an ephemeral run store to a test server so a completed run is
// recorded somewhere observable.
func withRunStore(s *Server) *telemetry.RunStore {
	rs, _ := telemetry.LoadRunStore("")
	s.runs = rs
	return rs
}

func TestWorkflowRunRecordedOnceSequential(t *testing.T) {
	s, _ := newTestServer()
	rs := withRunStore(s)
	run := startSeqRun(s)

	// Drive both lanes to completion via the reducer (the sole-running-lane fallback
	// attributes the cost-free idle events with no session id).
	s.handleEvent(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 1000, OutputTokens: 200}})
	s.handleEvent(copilot.Event{Type: copilot.EvMessage, Text: "alpha"})
	s.handleEvent(copilot.Event{Type: copilot.EvIdle}) // lane 0 done → lane 1 runs
	s.handleEvent(copilot.Event{Type: copilot.EvMessage, Text: "beta"})
	s.handleEvent(copilot.Event{Type: copilot.EvIdle}) // lane 1 done → run done

	if !run.done {
		t.Fatal("run should be done after the last lane")
	}
	if rs.Count() != 1 {
		t.Fatalf("a completed run should be recorded exactly once, got %d", rs.Count())
	}
	rec := rs.Records()[0]
	if rec.WorkflowID != "w" || rec.Mode != ctxforge.WorkflowSequential || rec.Outcome != "finished" {
		t.Fatalf("run record fields wrong: %+v", rec)
	}
	if len(rec.Lanes) != 2 || rec.Lanes[0].Status != "done" || rec.Lanes[1].Status != "done" {
		t.Fatalf("run record lanes wrong: %+v", rec.Lanes)
	}
	if rec.Lanes[0].AgentID != "a" || rec.Lanes[0].Credits <= 0 {
		t.Fatalf("lane 0 should carry its agent + metered cost: %+v", rec.Lanes[0])
	}
}

func TestWorkflowRunRecordedOnceParallel(t *testing.T) {
	s, _ := newTestServer()
	rs := withRunStore(s)
	run := startParallelRun(s) // two lanes, sessionIDs s0/s1

	s.handleEvent(copilot.Event{Type: copilot.EvMessage, SessionID: "s0", Text: "alpha"})
	s.handleEvent(copilot.Event{Type: copilot.EvIdle, SessionID: "s0"})
	s.handleEvent(copilot.Event{Type: copilot.EvMessage, SessionID: "s1", Text: "beta"})
	s.handleEvent(copilot.Event{Type: copilot.EvIdle, SessionID: "s1"})

	if !run.done {
		t.Fatal("parallel run should be done once both lanes settle")
	}
	if rs.Count() != 1 {
		t.Fatalf("a completed parallel run should be recorded exactly once, got %d", rs.Count())
	}
	if rs.Records()[0].Mode != ctxforge.WorkflowParallel {
		t.Fatalf("recorded mode = %q, want parallel", rs.Records()[0].Mode)
	}
}

func TestWorkflowRunRecordsSkippedLane(t *testing.T) {
	// A branched sequential run whose last lane skips must persist that skip — the
	// reason a sibling run store exists (a skip leaves no spend record). ADR-0022.
	wf := ctxforge.Workflow{ID: "w", Name: "Branchy", Mode: ctxforge.WorkflowSequential}
	steps := []ctxforge.CompiledStep{
		{AgentID: "a", AgentName: "Alpha", Prompt: "review"},
		{AgentID: "b", AgentName: "Beta", Prompt: "fix",
			When: &ctxforge.StepCondition{Step: 1, Condition: ctxforge.CondFailed}},
	}
	run := newWorkflowRun(wf, steps, []copilot.SessionSpec{{}, {}})
	s, _ := newTestServer()
	rs := withRunStore(s)
	s.mu.Lock()
	s.run = run
	s.busy = true
	run.start()
	s.mu.Unlock()

	// Lane 0 succeeds → Beta's "when step 1 failed" predicate is unsatisfied → skip →
	// the run terminates.
	s.handleEvent(copilot.Event{Type: copilot.EvMessage, Text: "all good"})
	s.handleEvent(copilot.Event{Type: copilot.EvIdle})

	if !run.done {
		t.Fatal("run should terminate after the gated lane skips")
	}
	if rs.Count() != 1 {
		t.Fatalf("want one recorded run, got %d", rs.Count())
	}
	lanes := rs.Records()[0].Lanes
	if len(lanes) != 2 || lanes[1].Status != "skipped" {
		t.Fatalf("the gated lane should persist as skipped: %+v", lanes)
	}
	if lanes[1].Credits != 0 {
		t.Fatalf("a skipped lane incurs no cost, got %v", lanes[1].Credits)
	}
}

func TestWorkflowRunRecordingNoStoreNoPanic(t *testing.T) {
	s, _ := newTestServer() // no run store wired
	run := startSeqRun(s)
	s.handleEvent(copilot.Event{Type: copilot.EvIdle})
	s.handleEvent(copilot.Event{Type: copilot.EvIdle})
	if !run.done {
		t.Fatal("run should still complete with no run store")
	}
}

func TestRunsPartialRendersStructure(t *testing.T) {
	s, _ := newTestServer()
	rs := withRunStore(s)
	_ = rs.Append(telemetry.RunRecord{
		ID: "run-1", WorkflowID: "build-and-harden", Name: "Build &amp; harden",
		Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{
			{Index: 0, AgentID: "builder", Status: "done", Credits: 2.6},
			{Index: 1, AgentID: "fixer", Status: "skipped"},
		},
	})
	html := s.runsPartial(defaultSpendWindow)
	for _, sub := range []string{"Runs", "Build &amp;amp; harden", "sequential", "skipped"} {
		if !strings.Contains(html, sub) {
			t.Errorf("runs partial missing %q\n%s", sub, html)
		}
	}
}

func TestRunsPartialRendersDurationAndSummary(t *testing.T) {
	s, _ := newTestServer()
	rs := withRunStore(s)
	base := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	// Two runs of one workflow (one failed) → a summary row with a failure count, and
	// per-run duration cells in the history below.
	_ = rs.Append(telemetry.RunRecord{
		ID: "r1", WorkflowID: "build-and-harden", Name: "Build & harden", Mode: "sequential",
		StartedAt: base, FinishedAt: base.Add(2 * time.Minute), Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done", Credits: 2.6}},
	})
	_ = rs.Append(telemetry.RunRecord{
		ID: "r2", WorkflowID: "build-and-harden", Name: "Build & harden", Mode: "sequential",
		StartedAt: base, FinishedAt: base.Add(4 * time.Minute), Outcome: "failed",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "failed", Credits: 1.0}},
	})
	html := s.runsPartial(defaultSpendWindow)
	// Structure: a summary table with a per-workflow row, and a duration cell.
	for _, sub := range []string{
		`class="run-summary"`, `class="run-summary-row"`,
		`run-summary-name`, `run-summary-failures`, `run-summary-avgdur`,
		`class="run-duration dim"`,
	} {
		if !strings.Contains(html, sub) {
			t.Errorf("runs partial missing %q\n%s", sub, html)
		}
	}
	// The failed run contributes a non-zero failure count in the summary.
	if !strings.Contains(html, "has-failures") {
		t.Errorf("a failed run should flag the failures cell:\n%s", html)
	}
}

func TestRunsSummaryShowsTotalAndAvgCredits(t *testing.T) {
	// The per-workflow summary surfaces a workflow's CUMULATIVE orchestrated spend
	// (TotalCredits) beside its average (V13). Two runs of one workflow make total ≠
	// avg, so a row that showed only the average would be indistinguishable from one
	// that showed only the total.
	s, _ := newTestServer()
	rs := withRunStore(s)
	base := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	_ = rs.Append(telemetry.RunRecord{
		ID: "r1", WorkflowID: "build-and-harden", Name: "Build & harden", Mode: "sequential",
		StartedAt: base, FinishedAt: base.Add(time.Minute), Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done", Credits: 2.6}},
	})
	_ = rs.Append(telemetry.RunRecord{
		ID: "r2", WorkflowID: "build-and-harden", Name: "Build & harden", Mode: "sequential",
		StartedAt: base, FinishedAt: base.Add(time.Minute), Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done", Credits: 1.0}},
	})
	// total = 3.60 cr, avg = 1.80 cr — distinct strings.
	html := s.runsPartial(defaultSpendWindow)
	for _, sub := range []string{
		`class="run-summary-totalcost"`, // the new column cell
		"Total&nbsp;cost",               // the new column header
		"3.60 cr",                       // cumulative spend across the two runs
		"1.80 cr",                       // the average, still rendered beside it
	} {
		if !strings.Contains(html, sub) {
			t.Errorf("runs summary missing %q\n%s", sub, html)
		}
	}
}

func TestRunsPartialRendersLaneShares(t *testing.T) {
	// The "Cost by lane" roll-up (V14) is the finest orchestration-attribution grain:
	// which lane in a workflow costs / fails most. It renders below the per-workflow
	// summary, resolving the lane's agent id to a display name under forgeMu (like the
	// run rows). A skipped lane contributes zero cost.
	s, _ := newTestServer()
	_ = s.forge.AddAgent(ctxforge.Agent{ID: "builder", Name: "Builder", Model: "gpt-5"})
	rs := withRunStore(s)
	_ = rs.Append(telemetry.RunRecord{
		ID: "r1", WorkflowID: "build-and-harden", Name: "Build & harden", Mode: "sequential",
		StartedAt: time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC), Outcome: "finished",
		Lanes: []telemetry.RunLane{
			{Index: 0, AgentID: "builder", Status: "done", Credits: 2.6},
			{Index: 1, AgentID: "fixer", Status: "skipped"},
		},
	})
	html := s.runsPartial(defaultSpendWindow)
	for _, sub := range []string{
		"Cost by lane",              // the section heading
		`class="trend lane-shares"`, // the list
		`class="trend-row lane-share-row"`,
		"Builder", // the lane's agent id resolved to its display name
	} {
		if !strings.Contains(html, sub) {
			t.Errorf("runs partial missing lane-share %q\n%s", sub, html)
		}
	}
}

func TestRunsPartialSlicesToWindow(t *testing.T) {
	// The Runs page mirrors the Telemetry trend's 14/30/90-day window selector (V12):
	// a clamped ?window= slices the run history (anchored to the most recent run) so a
	// long history stays scannable. A run outside the window is dropped from BOTH the
	// history list AND the per-workflow summary roll-up.
	s, _ := newTestServer()
	rs := withRunStore(s)
	latest := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	// A fresh run 2 days before the latest (inside every window) and a stale run 40 days
	// before it (outside 14/30, inside 90). The anchor fixes "latest".
	_ = rs.Append(telemetry.RunRecord{
		ID: "old", WorkflowID: "stale", Name: "StaleFlow", Mode: "sequential",
		StartedAt: latest.AddDate(0, 0, -40), FinishedAt: latest.AddDate(0, 0, -40).Add(time.Minute),
		Outcome: "finished", Lanes: []telemetry.RunLane{{Index: 0, AgentID: "a", Status: "done", Credits: 9.0}},
	})
	_ = rs.Append(telemetry.RunRecord{
		ID: "recent", WorkflowID: "fresh", Name: "FreshFlow", Mode: "sequential",
		StartedAt: latest.AddDate(0, 0, -2), FinishedAt: latest.AddDate(0, 0, -2).Add(time.Minute),
		Outcome: "finished", Lanes: []telemetry.RunLane{{Index: 0, AgentID: "a", Status: "done", Credits: 1.0}},
	})
	_ = rs.Append(telemetry.RunRecord{
		ID: "anchor", WorkflowID: "fresh", Name: "FreshFlow", Mode: "sequential",
		StartedAt: latest, FinishedAt: latest.Add(time.Minute),
		Outcome: "finished", Lanes: []telemetry.RunLane{{Index: 0, AgentID: "a", Status: "done", Credits: 1.0}},
	})

	// 14-day window: the 40-day-old run is excluded from history + summary; the fresh
	// workflow stays.
	narrow := s.runsPartial(defaultSpendWindow)
	if strings.Contains(narrow, "StaleFlow") {
		t.Errorf("a run 40 days before the latest must fall outside the 14-day window:\n%s", narrow)
	}
	if !strings.Contains(narrow, "FreshFlow") {
		t.Errorf("recent runs must stay inside the 14-day window:\n%s", narrow)
	}

	// 90-day window: the stale run reappears in both the history and the summary.
	wide := s.runsPartial(90)
	if !strings.Contains(wide, "StaleFlow") {
		t.Errorf("the 90-day window must include the 40-day-old run:\n%s", wide)
	}

	// The selector renders the three windows, the active one (14) marked, each
	// re-fetching GET /page/runs?window=N — mirroring the Telemetry trend selector.
	for _, sub := range []string{
		`class="window-row"`,
		`class="window on" hx-get="/page/runs?window=14"`,
		`hx-get="/page/runs?window=30"`,
		`hx-get="/page/runs?window=90"`,
	} {
		if !strings.Contains(narrow, sub) {
			t.Errorf("runs window selector missing %q\n%s", sub, narrow)
		}
	}
}

func TestRunsWindowClampsGarbage(t *testing.T) {
	// renderPage threads ?window= through clampWindow (reused from the Telemetry trend):
	// garbage or out-of-range falls back to the default 14-day window.
	s, _ := newTestServer()
	rs := withRunStore(s)
	_ = rs.Append(telemetry.RunRecord{
		ID: "r", WorkflowID: "w", Name: "W", Mode: "sequential",
		StartedAt: time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC), Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "a", Status: "done", Credits: 1.0}},
	})
	for _, raw := range []string{"bogus", "999", "", "-1"} {
		html := s.renderPage("runs", raw)
		if !strings.Contains(html, `class="window on" hx-get="/page/runs?window=14"`) {
			t.Errorf("?window=%q should clamp back to the default (14):\n%s", raw, html)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{0, ""},
		{-time.Minute, ""}, // a negative span renders empty, never a "-" string
		{30 * time.Second, "30s"},
		{2 * time.Minute, "2m"},
		{2*time.Minute + 30*time.Second, "2m 30s"},
		{time.Hour, "1h"},
		{time.Hour + 5*time.Minute, "1h 5m"},
		// Sub-second rounding must CARRY into the next unit, never print "60s"/"60m":
		{59*time.Second + 600*time.Millisecond, "1m"},                  // 59.6s → 1m, not 60s
		{time.Minute + 59*time.Second + 600*time.Millisecond, "2m"},    // 1m59.6s → 2m, not 1m 60s
		{59*time.Minute + 59*time.Second + 600*time.Millisecond, "1h"}, // 59m59.6s → 1h, not 60m
	} {
		if got := humanDuration(tc.d); got != tc.want {
			t.Errorf("humanDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestRunsPartialEmpty(t *testing.T) {
	s, _ := newTestServer()
	withRunStore(s)
	html := s.runsPartial(defaultSpendWindow)
	if !strings.Contains(html, "no runs yet") {
		t.Errorf("empty runs partial should hint at no runs: %s", html)
	}
}

func TestRunsExportReturnsCSV(t *testing.T) {
	// The Runs page's export mirrors the spend ledger export: a CSV attachment over the
	// persisted run history, so orchestration runs can be analysed outside the app.
	s, _ := newTestServer()
	rs := withRunStore(s)
	if err := rs.Append(telemetry.RunRecord{
		ID: "run-1", WorkflowID: "build-and-harden", Name: "Build & harden", Mode: "sequential",
		StartedAt: time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC),
		Outcome:   "finished",
		Lanes:     []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done", Credits: 2.6}},
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/runs/export.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("Content-Type = %q, want text/csv", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("Content-Disposition = %q, want attachment", cd)
	}
	body, _ := io.ReadAll(resp.Body)
	out := string(body)
	if !strings.Contains(out, "run,workflow,name") || !strings.Contains(out, "build-and-harden") {
		t.Fatalf("CSV body missing header or data:\n%s", out)
	}
}

func TestRunsExportHeaderOnlyWithoutStore(t *testing.T) {
	// No run store wired (e.g. ephemeral chat-only build): the export is the header
	// alone, never a 500 — mirrors the spend export's empty case.
	s, _ := newTestServer() // s.runs == nil
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/runs/export.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(string(body), "run,workflow,name") {
		t.Fatalf("want header-only CSV, got:\n%s", body)
	}
}

func TestRunsPageHasExportLink(t *testing.T) {
	// The Runs page surfaces the export affordance only when history exists (mirroring
	// the Telemetry "Spend history" export link).
	s, _ := newTestServer()
	rs := withRunStore(s)
	if err := rs.Append(telemetry.RunRecord{
		ID: "run-1", WorkflowID: "w", Name: "W", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, Status: "done"}},
	}); err != nil {
		t.Fatal(err)
	}
	html := s.runsPartial(defaultSpendWindow)
	if !strings.Contains(html, `href="/runs/export.csv"`) {
		t.Fatalf("Runs page should carry the export link:\n%s", html)
	}
}
