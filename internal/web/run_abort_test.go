package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// TestRunAbortStopsInFlightRun proves the dual of rerun (ADR-0024): a POST to /run/abort
// settles an in-flight run in place — every still-running lane's backing session is aborted
// over the existing client.Abort seam, every unsettled lane is marked failed (a stopped run
// is a failed run — no new terminal status), the run flips done+failed so the shared runFrags
// completion path records it once and clears busy, and the user lands back on the chat page.
func TestRunAbortStopsInFlightRun(t *testing.T) {
	hub, mock := newWorkflowHub()
	s := hub.newSession("t")
	rs := withRunStore(s)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	// Install a parallel run with two lanes running on distinct backing sessions (s0/s1).
	run := startParallelRun(s)

	resp, err := http.Post(srv.URL+"/run/abort", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// The abort lands the user back on the chat page with the settled lanes panel.
	if !strings.Contains(string(body), "workflow-run") {
		t.Errorf("abort should re-render the chat page with the settled lanes: %q", string(body))
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		t.Error("busy should clear when the run is aborted")
	}
	if !run.done || !run.failed {
		t.Errorf("an aborted run should be done+failed, got done=%v failed=%v", run.done, run.failed)
	}
	for i, l := range run.lanes {
		if l.status != laneFailed {
			t.Errorf("lane %d should settle failed on abort, got %v", i, l.status)
		}
	}
	// Each running lane's backing session is aborted over the seam.
	if len(mock.Aborted) != 2 {
		t.Errorf("both running lanes' sessions should be aborted, got %v", mock.Aborted)
	}
	// The aborted run is recorded once, as a failed run (reuses the run-record path).
	recs := rs.Records()
	if len(recs) != 1 {
		t.Fatalf("an aborted run should be recorded once, got %d", len(recs))
	}
	if recs[0].Outcome != "failed" {
		t.Errorf("an aborted run records as a failed outcome, got %q", recs[0].Outcome)
	}
}

// TestRunAbortThenLateLaneErrorRecordsOnce guards the completion path's idempotency
// (ADR-0024): aborting a run forces it done while lane goroutines may still be in flight,
// and a lane whose Send then errors re-enters runFrags via laneError (it bypasses the
// session.go done-guard). The run must still be recorded exactly once.
func TestRunAbortThenLateLaneErrorRecordsOnce(t *testing.T) {
	s, _ := newTestServer()
	rs := withRunStore(s)
	run := startParallelRun(s) // two lanes running on s0/s1

	s.abortRun(context.Background()) // settles + records the run once

	// A still-in-flight lane goroutine errors after the abort already settled the run.
	s.laneError(run, run.lanes[0], errLate)

	if recs := rs.Records(); len(recs) != 1 {
		t.Fatalf("an aborted run must be recorded once even after a late lane error, got %d", len(recs))
	}
}

var errLate = errLateErr("send aborted")

type errLateErr string

func (e errLateErr) Error() string { return string(e) }

// TestRunAbortNoRunIsNoop proves a stop with no run in flight is fail-safe: it aborts no
// session, changes no state, and re-renders the chat page (a racing double-click can't
// double-settle a finished run).
func TestRunAbortNoRunIsNoop(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/run/abort", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		t.Error("no run was in flight — busy must stay clear")
	}
	if len(mock.Aborted) != 0 {
		t.Errorf("no session should be aborted when no run is in flight, got %v", mock.Aborted)
	}
}

// TestLanesPanelShowsStopForRunningRunHidesWhenDone proves the lanes panel offers a stop
// control (a DISJOINT .stop-run class, ≠ the chat .abort / Workflows button.run) only while
// the run is in flight — the affordance that fronts /run/abort.
func TestLanesPanelShowsStopForRunningRunHidesWhenDone(t *testing.T) {
	run := twoStepRun(ctxforge.WorkflowSequential)
	run.start() // lane 0 running → the run is in flight

	html := renderLanes(run)
	if !strings.Contains(html, `class="stop-run"`) || !strings.Contains(html, `hx-post="/run/abort"`) {
		t.Errorf("a running run should offer a stop-run control: %q", html)
	}

	run.done = true
	if got := renderLanes(run); strings.Contains(got, "stop-run") || strings.Contains(got, "/run/abort") {
		t.Errorf("a finished run should not offer a stop control: %q", got)
	}
}
