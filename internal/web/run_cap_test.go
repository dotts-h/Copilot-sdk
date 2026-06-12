package web

import (
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// --- pure engine: capStop (ADR-0053) ---

// capStop settles the lane the cap refused plus every not-yet-started lane,
// leaves a still-running lane to finish, and marks the run done once all settle.
func TestRunCapStopSettlesRefusedAndPending(t *testing.T) {
	r := twoStepRun(ctxforge.WorkflowSequential)
	// Real sequential shape at cap time: lane 0 already finished, lane 1 was about
	// to be admitted (running) when the cap refused it.
	r.start()
	r.finishLane(r.lanes[0], "done")
	if r.lanes[1].status != laneRunning {
		t.Fatalf("precondition: lane 1 should be running, got %v", r.lanes[1].status)
	}

	r.capStop(r.lanes[1])

	if r.lanes[0].status != laneDone {
		t.Errorf("a lane that finished before the cap stays done, got %v", r.lanes[0].status)
	}
	if r.lanes[1].status != laneFailed || !strings.Contains(r.lanes[1].detail, "over run budget cap") {
		t.Errorf("the refused lane fails with the cap reason, got %v %q", r.lanes[1].status, r.lanes[1].detail)
	}
	if !r.done || !r.failed {
		t.Errorf("the run settles done+failed once every lane is settled: done=%v failed=%v", r.done, r.failed)
	}
}

// capStop fails every pending lane (no further admission) and leaves a concurrently
// running lane to finish — the run is not done until that lane settles.
func TestRunCapStopLeavesRunningLaneToFinish(t *testing.T) {
	r := twoStepRun(ctxforge.WorkflowParallel)
	r.start() // both lanes running
	// Model a wide run where lane 1 is queued (pending) behind the worker pool when
	// the cap refuses it, while lane 0 is still streaming.
	r.lanes[1].status = lanePending

	r.capStop(r.lanes[1])

	if r.lanes[1].status != laneFailed {
		t.Errorf("the refused (pending) lane fails, got %v", r.lanes[1].status)
	}
	if r.done {
		t.Error("the run is not done while lane 0 is still running")
	}
	// Lane 0 finishing settles the run with no new admission.
	if next := r.finishLane(r.lanes[0], "done"); next != nil {
		t.Errorf("a capped run admits no next lane, got %v", next)
	}
	if !r.done {
		t.Error("the run is done once the last running lane settles")
	}
}

// --- server decision: runOverCap reuses telemetry.Leash semantics ---

func TestRunOverCapDecision(t *testing.T) {
	s, _ := newTestServer()
	r := twoStepRun(ctxforge.WorkflowSequential)

	s.runCap = 10
	r.credits = 9.99
	if s.runOverCap(r) {
		t.Error("under the cap is admitted")
	}
	r.credits = 10 // inclusive >= : the cap is the last allowed value, the crossing turn gates
	if !s.runOverCap(r) {
		t.Error("reaching the cap stops admission")
	}
}

func TestRunCapZeroNeverStops(t *testing.T) {
	s, _ := newTestServer()
	r := twoStepRun(ctxforge.WorkflowSequential)
	s.runCap = 0 // the default: no per-run cap
	r.credits = 1e9
	if s.runOverCap(r) {
		t.Error("a zero cap never gates — byte-identical to the no-cap path")
	}
}

// --- wiring: startLane refuses an over-cap lane without opening a session ---

func TestStartLaneRefusedOverCap(t *testing.T) {
	s, _ := newTestServer()
	r := twoStepRun(ctxforge.WorkflowSequential)
	s.mu.Lock()
	s.run = r
	s.busy = true
	r.start()
	r.finishLane(r.lanes[0], "done") // lane 0 done, lane 1 about to be admitted
	r.credits = 5                    // the run has already spent past the cap
	s.runCap = 1
	s.mu.Unlock()

	s.startLane(r, 1)

	if r.lanes[1].sessionID != "" {
		t.Errorf("an over-cap lane must not open a backing session, got %q", r.lanes[1].sessionID)
	}
	if r.lanes[1].status != laneFailed || !strings.Contains(r.lanes[1].detail, "over run budget cap") {
		t.Errorf("the refused lane fails with the cap reason, got %v %q", r.lanes[1].status, r.lanes[1].detail)
	}
	if !r.done {
		t.Error("the run settles once the cap refuses its last lane")
	}
	if !hasSystemNote(s, "over budget cap") {
		t.Error("a system note should surface the cap-hit")
	}
}

// A run under its cap is admitted normally — the streaming/record path is
// unchanged when the cap does not bite.
func TestStartLaneAdmittedUnderCap(t *testing.T) {
	s, _ := newTestServer()
	r := twoStepRun(ctxforge.WorkflowSequential)
	s.mu.Lock()
	s.run = r
	s.busy = true
	r.start()
	r.credits = 0
	s.runCap = 100
	s.mu.Unlock()

	s.startLane(r, 0)

	if r.lanes[0].sessionID == "" {
		t.Error("a lane under the cap should open its backing session (CreateSession ran)")
	}
}

// hasSystemNote reports whether the transcript carries a system note containing sub.
func hasSystemNote(s *Server, sub string) bool {
	for _, turn := range s.state.Committed() {
		if turn.Role == convo.RoleSystem && strings.Contains(turn.Text, sub) {
			return true
		}
	}
	return false
}
