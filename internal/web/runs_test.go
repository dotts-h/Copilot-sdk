package web

import (
	"strings"
	"testing"

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

func TestRunsPartialEmpty(t *testing.T) {
	s, _ := newTestServer()
	withRunStore(s)
	html := s.runsPartial()
	if !strings.Contains(html, "no runs yet") {
		t.Errorf("empty runs partial should hint at no runs: %s", html)
	}
}
