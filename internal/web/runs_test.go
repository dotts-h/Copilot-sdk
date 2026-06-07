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
	html := s.runsPartial()
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
	html := s.runsPartial()
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
	html := s.runsPartial()
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
	html := s.runsPartial()
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
	html := s.runsPartial()
	if !strings.Contains(html, `href="/runs/export.csv"`) {
		t.Fatalf("Runs page should carry the export link:\n%s", html)
	}
}
