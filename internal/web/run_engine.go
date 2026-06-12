package web

import (
	"fmt"
	"strings"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
)

// This file is the pure run state machine behind the multi-agent run / handoff
// surface (item 2.1): the control surface that cashes the product's name. A forge
// Workflow (an ordered list of (agent, task) steps) is run as a set of LANES — each
// lane is a sub-run on the seam's session lifecycle (one CreateSession + Send per
// step), watched live in a dedicated panel. Sequential workflows hand each lane's
// output to the next; parallel workflows fan all lanes out at once. The run state
// machine (workflowRun) is pure and deterministic so it is unit-tested with no
// client; the Server adapter (run_adapter.go) maps each lane onto the seam and the
// renderer (run_render.go) draws it. See ADR-0013.

// laneStatus is one lane's lifecycle state.
type laneStatus int

const (
	lanePending laneStatus = iota
	laneRunning
	laneInputRequired // parked on a human-in-the-loop pause — non-terminal (S4 / ADR-0043)
	laneDone
	laneFailed
	laneSkipped // a gated step whose predicate was unsatisfied (B2 / ADR-0021)
)

// settled reports whether a lane has reached a terminal status (done, failed, or
// skipped) — the predicate evaluator and allSettled both key off this, so a skipped
// lane counts as settled and a branching run still terminates. input-required is
// deliberately NOT settled: a parked lane keeps the run live (A2A's non-terminal
// contract) so siblings stream and the human can still resolve it (S4).
func settled(st laneStatus) bool {
	return st == laneDone || st == laneFailed || st == laneSkipped
}

// lane is one step of a running workflow: its agent, task prompt, compiled spec,
// the backing copilot session, the accumulated output, and live status/cost.
type lane struct {
	Index     int
	AgentID   string
	AgentName string
	Prompt    string
	spec      copilot.SessionSpec
	status    laneStatus
	text      string                      // accumulated assistant output (deltas, replaced by the full message)
	detail    string                      // one-line summary on completion (cost) or the error message
	sessionID string                      // backing copilot session id, for SessionID-keyed event routing
	credits   float64                     // metered cost attributed to this lane
	tools     []*convo.ToolView           // this lane's own tool-execution timeline (B1)
	toolIdx   map[string]int              // tool-call id -> index into tools
	perms     []copilot.PermissionRequest // this lane's pending inline permission requests (B1)
	when      *ctxforge.StepCondition     // optional predicate gating this step (B2 / ADR-0021)
	// pauseID is the open human-in-the-loop pause this lane is parked on (S4); empty
	// when the lane is not input-required. cancelReason, when set, records that the
	// human cooperatively cancelled the pause: the lane keeps running so the sub-agent
	// wraps up, then settles failed(cancelled) at its next idle rather than done.
	pauseID      string
	cancelReason string
	// pauses counts how many times this lane parked on a human-in-the-loop pause
	// over the run, pausedDur accumulates the wall-clock it spent parked, and
	// pausedAt holds the start of the currently-open park (zero when not parked).
	// These feed the per-lane attention attribution recorded on the finished run
	// (RunLane.Pauses/PausedMs, S6) — "where humans were the bottleneck".
	pauses    int
	pausedDur time.Duration
	pausedAt  time.Time
}

// toolStart records a tool-execution start on the lane's own timeline. Mirrors
// convo.State.ToolStart but lives on the lane, since each parallel sub-run has its
// own interleaved tool activity (B1 / issue 0015).
func (l *lane) toolStart(id, name, args string) {
	if name == "" {
		return
	}
	if l.toolIdx == nil {
		l.toolIdx = map[string]int{}
	}
	l.tools = append(l.tools, &convo.ToolView{ID: id, Name: name, Args: args})
	if id != "" {
		l.toolIdx[id] = len(l.tools) - 1
	}
}

// toolProgress updates a running lane tool's latest progress message.
func (l *lane) toolProgress(id, msg string) {
	if tv := l.toolByID(id); tv != nil {
		tv.Progress = msg
	}
}

// toolEnd marks a lane tool finished, recording its result and success.
func (l *lane) toolEnd(id, result string, success bool) {
	if tv := l.toolByID(id); tv != nil {
		tv.Done = true
		tv.Failed = !success
		tv.Progress = ""
		if result != "" {
			tv.Result = result
		}
	}
}

func (l *lane) toolByID(id string) *convo.ToolView {
	if id == "" || l.toolIdx == nil {
		return nil
	}
	if i, ok := l.toolIdx[id]; ok && i < len(l.tools) {
		return l.tools[i]
	}
	return nil
}

// dropPerm removes a resolved permission from the lane, reporting whether it held
// one with that id.
func (l *lane) dropPerm(id string) bool {
	for i := range l.perms {
		if l.perms[i].ID == id {
			l.perms = append(l.perms[:i], l.perms[i+1:]...)
			return true
		}
	}
	return false
}

// appendText accumulates streamed output on a lane.
func (l *lane) appendText(s string) { l.text += s }

// workflowRun is a workflow in flight: the lanes and the sequential cursor. It is
// a pure state machine — every method mutates only the run and returns the lane
// indices the adapter should launch next, with no IO — so it is fully testable
// without a client. The Server guards it with s.mu.
type workflowRun struct {
	id       string
	name     string
	mode     string
	lanes    []*lane
	cur      int // index of the running lane in sequential mode
	done     bool
	failed   bool
	recorded bool // the terminal completion path (runFrags) has run — guards a second record
	// runID and started are stamped by the Server adapter (outside the pure engine)
	// for run history: runID is a unique id for this run instance (distinct from id,
	// which is the workflow definition's id), started is the launch time (ADR-0022).
	runID   string
	started time.Time
	// credits is the run's cumulative metered spend (sum of its lanes' credits),
	// accrued by the Server adapter as usage events land. It is the figure the
	// per-run budget cap checks before admitting the next lane (ADR-0053).
	credits float64
}

// capStopDetail is the lane detail set when the per-run budget cap refuses a lane.
const capStopDetail = "✗ over run budget cap"

// capStop settles a run stopped by its budget cap (ADR-0053). The lane the cap
// refused plus every not-yet-started (pending) lane are failed with the cap
// reason — so no further lane can be admitted — while a lane already running is
// left to finish, and the run is marked done+failed once every lane has settled.
// Unlike failLane it advances nothing: the cap is the single choke point, so a
// capped run can only wind down. Pure (no I/O); caller holds s.mu in the adapter.
func (r *workflowRun) capStop(refused *lane) {
	refused.status = laneFailed
	refused.detail = capStopDetail
	for _, l := range r.lanes {
		if l.status == lanePending {
			l.status = laneFailed
			l.detail = capStopDetail
		}
	}
	r.failed = true
	if r.allSettled() {
		r.done = true
	}
}

// newWorkflowRun builds a run with one pending lane per compiled step. specs are
// the per-step copilot SessionSpecs (translated from the forge's compiled specs
// with config fallbacks applied), parallel to steps.
func newWorkflowRun(wf ctxforge.Workflow, steps []ctxforge.CompiledStep, specs []copilot.SessionSpec) *workflowRun {
	lanes := make([]*lane, len(steps))
	for i, st := range steps {
		lanes[i] = &lane{
			Index: i, AgentID: st.AgentID, AgentName: st.AgentName,
			Prompt: st.Prompt, spec: specs[i], status: lanePending, when: st.When,
		}
	}
	return &workflowRun{id: wf.ID, name: wf.Name, mode: wf.EffectiveMode(), lanes: lanes}
}

// start marks the lanes to launch first and returns their indices: the first lane
// in sequential mode; in parallel mode every lane whose predicate is already
// runnable (ungated, or whose dependency has settled — none have at start, so the
// ungated lanes), leaving gated lanes pending until their dependency settles (B2).
func (r *workflowRun) start() []int {
	if len(r.lanes) == 0 {
		r.done = true
		return nil
	}
	if r.mode == ctxforge.WorkflowParallel {
		return r.evalPending()
	}
	r.cur = 0
	r.lanes[0].status = laneRunning
	return []int{0}
}

// evalWhen evaluates lane l's predicate against the run's settled lanes. satisfied
// reports whether the step should run; ready reports whether the predicate's
// dependency has settled yet (always true for a nil/always predicate). A lane with
// ready=false must wait — its dependency is still pending or running. Pure: it reads
// only prior lanes' settled status/output (ADR-0021). The referenced step is a
// strictly-prior, validated index, so dep is always in range.
func (r *workflowRun) evalWhen(l *lane) (satisfied, ready bool) {
	w := l.when
	if w == nil || w.Condition == ctxforge.CondAlways {
		return true, true
	}
	dep := r.lanes[w.Step-1] // 1-based, validated to reference a prior step
	if !settled(dep.status) {
		return false, false
	}
	switch w.Condition {
	case ctxforge.CondSucceeded:
		return dep.status == laneDone, true
	case ctxforge.CondFailed:
		return dep.status == laneFailed, true
	case ctxforge.CondOutputContains:
		return strings.Contains(strings.ToLower(dep.text), strings.ToLower(w.Value)), true
	}
	return true, true
}

// skipDetail is the one-line reason shown on a skipped lane.
func skipDetail(w *ctxforge.StepCondition) string {
	if w == nil {
		return "skipped"
	}
	switch w.Condition {
	case ctxforge.CondOutputContains:
		return fmt.Sprintf("skipped — step %d output did not contain %q", w.Step, w.Value)
	case ctxforge.CondSucceeded:
		return fmt.Sprintf("skipped — step %d did not succeed", w.Step)
	case ctxforge.CondFailed:
		return fmt.Sprintf("skipped — step %d did not fail", w.Step)
	}
	return "skipped"
}

// evalPending (parallel) launches every pending lane whose dependency has settled
// and whose predicate is satisfied, skipping the unsatisfied ones. It loops to a
// fixpoint so a skip — itself a settle — can unblock or cascade to further lanes.
// Returns the indices to launch.
func (r *workflowRun) evalPending() []int {
	var launch []int
	for {
		progressed := false
		for _, l := range r.lanes {
			if l.status != lanePending {
				continue
			}
			sat, ready := r.evalWhen(l)
			if !ready {
				continue
			}
			if sat {
				l.status = laneRunning
				launch = append(launch, l.Index)
			} else {
				l.status = laneSkipped
				l.detail = skipDetail(l.when)
			}
			progressed = true
		}
		if !progressed {
			break
		}
	}
	return launch
}

// handoffPrompt is the prompt to Send for lane idx. In a sequential run every step
// after the first receives the previous lane's output appended as a handoff so the
// chain composes; in parallel each lane starts from its own prompt alone.
func (r *workflowRun) handoffPrompt(idx int) string {
	l := r.lanes[idx]
	if r.mode == ctxforge.WorkflowParallel || idx == 0 {
		return l.Prompt
	}
	// Hand off from the nearest prior lane that actually ran and produced output,
	// stepping over skipped (or empty) lanes from a branch (B2).
	for j := idx - 1; j >= 0; j-- {
		prev := r.lanes[j]
		if prev.status == laneDone && strings.TrimSpace(prev.text) != "" {
			return l.Prompt + "\n\n--- Handoff from " + prev.AgentName + " ---\n" + strings.TrimSpace(prev.text)
		}
	}
	return l.Prompt
}

// laneFor resolves the lane an event belongs to: by its copilot session id when
// present (the only safe key when lanes run concurrently), else the sole running
// lane (covers the mock/demo, where events carry no session id and a sequential
// run has exactly one lane active). Returns nil when it can't be attributed.
func (r *workflowRun) laneFor(sessionID string) *lane {
	if sessionID != "" {
		for _, l := range r.lanes {
			if l.sessionID == sessionID {
				return l
			}
		}
	}
	var only *lane
	for _, l := range r.lanes {
		if l.status == laneRunning {
			if only != nil {
				return nil // ambiguous — needs a session id
			}
			only = l
		}
	}
	return only
}

// finishLane marks a lane done and advances the run, returning the lane indices to
// launch next. It sets done when the run is over.
func (r *workflowRun) finishLane(l *lane, detail string) []int {
	if l.status == laneRunning {
		l.status = laneDone
	}
	l.detail = detail
	return r.advance(l)
}

// failLane marks a lane failed and advances the run, returning the lane indices to
// launch next. A sequential run aborts (the chain is broken) and launches nothing; a
// parallel run lets the surviving lanes finish and may now unblock a `when failed`
// gated lane (B2). Returns the indices to launch (nil for the sequential abort).
func (r *workflowRun) failLane(l *lane, msg string) []int {
	l.status = laneFailed
	l.detail = msg
	if r.mode != ctxforge.WorkflowParallel {
		r.done = true
		r.failed = true
		return nil
	}
	return r.advance(l)
}

// abort settles a run the user stopped (ADR-0024). Every not-yet-settled lane (running
// or pending) is marked failed with an "aborted" detail and the run is flipped
// done+failed, so the shared runFrags completion path records it once and clears busy —
// a stopped run is a failed run, no new terminal status. It returns the backing session
// ids of the lanes that were still running, for the caller to abort over the seam.
func (r *workflowRun) abort() []string {
	var running []string
	for _, l := range r.lanes {
		// Abort every unsettled lane that has a live backing session — a lane parked
		// as input-required (S4) is non-terminal but still holds an open session, so
		// it must be aborted too, not just laneRunning ones, or its sub-agent leaks.
		if !settled(l.status) && l.sessionID != "" {
			running = append(running, l.sessionID)
		}
		if !settled(l.status) {
			l.status = laneFailed
			l.detail = "⏹ aborted"
		}
	}
	r.done = true
	r.failed = true
	return running
}

// advance runs the post-settle transition after lane l reached a terminal status.
// Sequential: walk forward to the next runnable step, skipping unsatisfied gated
// ones, and run the first satisfied one (or finish). Parallel: re-evaluate pending
// lanes (run-or-skip to a fixpoint) and finish once every lane has settled. Returns
// the lane indices to launch next.
func (r *workflowRun) advance(l *lane) []int {
	if r.mode == ctxforge.WorkflowParallel {
		launch := r.evalPending()
		if r.allSettled() {
			r.done = true
		}
		return launch
	}
	for next := l.Index + 1; next < len(r.lanes); next++ {
		nl := r.lanes[next]
		if sat, _ := r.evalWhen(nl); sat { // priors are settled in sequential, so ready
			r.cur = next
			nl.status = laneRunning
			return []int{next}
		}
		nl.status = laneSkipped
		nl.detail = skipDetail(nl.when)
	}
	r.done = true
	return nil
}

// allSettled reports whether every lane has reached a terminal status (done,
// failed, or skipped) — a skipped lane counts as settled so a branching run still
// terminates.
func (r *workflowRun) allSettled() bool {
	for _, l := range r.lanes {
		if !settled(l.status) {
			return false
		}
	}
	return true
}
