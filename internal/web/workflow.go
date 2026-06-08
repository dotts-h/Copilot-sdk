package web

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file is the multi-agent run / handoff surface (item 2.1): the control
// surface that cashes the product's name. A forge Workflow (an ordered list of
// (agent, task) steps) is run as a set of LANES — each lane is a sub-run on the
// seam's session lifecycle (one CreateSession + Send per step), watched live in a
// dedicated panel. Sequential workflows hand each lane's output to the next;
// parallel workflows fan all lanes out at once. The run state machine
// (workflowRun) is pure and deterministic so it is unit-tested with no client; the
// Server is the thin adapter that maps each lane onto the seam and renders it.
// See ADR-0013.

// laneStatus is one lane's lifecycle state.
type laneStatus int

const (
	lanePending laneStatus = iota
	laneRunning
	laneDone
	laneFailed
	laneSkipped // a gated step whose predicate was unsatisfied (B2 / ADR-0021)
)

// settled reports whether a lane has reached a terminal status (done, failed, or
// skipped) — the predicate evaluator and allSettled both key off this, so a skipped
// lane counts as settled and a branching run still terminates.
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

// appendText accumulates streamed output on a lane.
func (l *lane) appendText(s string) { l.text += s }

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
		if l.status == laneRunning && l.sessionID != "" {
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

// --- Server adapter: map lanes onto the seam ---

// handleWorkflowRun starts a workflow run from the Workflows page. It compiles the
// workflow under forgeMu (each step → a SessionSpec), builds the run, and launches
// the first lane(s); the lanes stream into #lanes over SSE. A run is mutually
// exclusive with a normal turn (both gated by s.busy).
func (s *Server) handleWorkflowRun(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !s.launchWorkflow(r.PathValue("id")) {
		// A compile error or a busy server made no state change — stay on the
		// Workflows page (the run-trigger's source).
		s.writePartial(w, s.workflowsPartial())
		return
	}
	// Land the user on the Chat page, where the lanes panel streams each step;
	// subsequent lane updates arrive over SSE (the body-level #lanes listener).
	s.writePartial(w, s.chatPartial())
}

// handleRunRerun re-executes a recorded run's workflow from the Runs page — the first
// action on the orchestration history surface (ADR-0023). It looks the workflow up by the
// run's WorkflowID and launches its CURRENT definition via the same launchWorkflow trigger
// as the Workflows-page run, so the new run rolls up under the same per-workflow totals
// (a rerun is a re-execution, not a historical replay). On success the user lands on the
// Chat page where the lanes stream; when no run starts (the workflow was deleted since, or
// the server is busy) the Runs page is re-rendered unchanged at the request's window.
func (s *Server) handleRunRerun(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !s.launchWorkflow(r.PathValue("workflow")) {
		s.writePartial(w, s.runsPartial(clampWindow(r.URL.Query().Get("window"))))
		return
	}
	s.writePartial(w, s.chatPartial())
}

// handleRunAbort stops an in-flight workflow run from the Chat lanes panel — the second
// action on the orchestration surface and the dual of rerun (ADR-0024). It settles the run
// in place via abortRun (running lanes' sessions aborted over the existing client.Abort
// seam, unsettled lanes marked failed, the run recorded once and busy cleared) and lands the
// user back on the chat page where the settled lanes render. A no-op when no run is in flight,
// so a racing double-click can't double-settle a finished run.
func (s *Server) handleRunAbort(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	s.abortRun(r.Context())
	s.writePartial(w, s.chatPartial())
}

// abortRun settles the active run as aborted and aborts its still-running lanes' backing
// sessions. Under s.mu it marks the run done+failed (run.abort), then runs the shared
// runFrags completion path (records the run once, clears busy) and broadcasts the terminal
// fragments to every connected view; the per-lane client.Abort calls happen OUTSIDE the lock
// (like handleAbort), since the seam call may block. A no-op when no run is active or it
// already finished — events stop routing to a done run (session.go), so this can't race the
// reducer into a double-record.
func (s *Server) abortRun(ctx context.Context) {
	s.mu.Lock()
	run := s.run
	if run == nil || run.done {
		s.mu.Unlock()
		return
	}
	running := run.abort()
	frags := s.runFrags(run, true)
	s.mu.Unlock()

	s.broadcast(frags)
	for _, sid := range running {
		if err := s.client.Abort(ctx, sid); err != nil {
			s.logger.Printf("abort run lane %q: %v", sid, err)
		}
	}
}

// launchWorkflow compiles a workflow by id and, if no run or turn is in flight, installs a
// fresh run and starts its lanes — the shared run-trigger behind both the Workflows-page
// "Run" (handleWorkflowRun) and the Runs-page "Rerun" (handleRunRerun) entry points
// (ADR-0023). It returns true when a run was launched; false — with NO state change — when
// the workflow can't be compiled (renamed/deleted since) or the server is busy, so each
// caller re-renders its own page. The workflow is looked up by id and run as its CURRENT
// definition, so a rerun re-executes the live workflow (not a historical replay) under the
// same WorkflowID. Holds forgeMu then s.mu (never inverted), like the original trigger.
func (s *Server) launchWorkflow(id string) bool {
	s.hub.forgeMu.Lock()
	wf, steps, err := s.forge.CompileWorkflow(id)
	var specs []copilot.SessionSpec
	if err == nil {
		specs = make([]copilot.SessionSpec, len(steps))
		for i, st := range steps {
			specs[i] = s.workflowLaneSpec(st.Spec)
		}
	}
	s.hub.forgeMu.Unlock()
	if err != nil {
		s.logger.Printf("compile workflow %q: %v", id, err)
		return false
	}

	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		// A turn or another run is in flight — make no change and let the caller stay put.
		return false
	}
	run := newWorkflowRun(wf, steps, specs)
	run.runID = newID()
	run.started = time.Now()
	s.run = run
	s.busy = true
	s.turnStartMs = nowMs()
	launch := run.start()
	s.state.AddSystem("▶ workflow " + wf.Name + " started (" + run.mode + ")")
	s.mu.Unlock()

	go s.launchLanes(run, launch)
	return true
}

// workflowLaneSpec translates a forge-compiled step spec into the seam's
// SessionSpec via the shared SeamSpec translation. Caller holds forgeMu (it reads
// s.config).
func (s *Server) workflowLaneSpec(cs ctxforge.SessionSpec) copilot.SessionSpec {
	var defModel, defEffort string
	if s.config != nil {
		defModel, defEffort = s.config.DefaultModel, s.config.ReasoningEffort
	}
	return SeamSpec(cs, defModel, defEffort, s.lookupEnv)
}

// launchLanes starts each given lane's sub-run concurrently.
func (s *Server) launchLanes(run *workflowRun, idxs []int) {
	for _, i := range idxs {
		go s.startLane(run, i)
	}
}

// startLane opens a backing session for a lane and sends its (handoff) prompt.
// Errors mark the lane failed and broadcast the update so the run never hangs.
func (s *Server) startLane(run *workflowRun, idx int) {
	ctx := context.Background()
	s.mu.Lock()
	l := run.lanes[idx]
	spec := l.spec
	prompt := run.handoffPrompt(idx)
	s.mu.Unlock()

	cid, err := s.client.CreateSession(ctx, spec)
	if err != nil {
		s.laneError(run, l, err)
		return
	}
	s.mu.Lock()
	l.sessionID = cid
	s.mu.Unlock()
	s.hub.bind(cid, s)

	if err := s.client.Send(ctx, cid, prompt, nil, ""); err != nil {
		s.laneError(run, l, err)
		return
	}
	if s.demo {
		if mock, ok := s.client.(*copilot.MockClient); ok {
			go streamDemoLane(mock, cid, prompt)
		}
	}
}

// laneError fails a lane and broadcasts the resulting lane/status fragments.
func (s *Server) laneError(run *workflowRun, l *lane, err error) {
	s.mu.Lock()
	next := run.failLane(l, "✗ "+err.Error())
	if len(next) > 0 {
		go s.launchLanes(run, next)
	}
	frags := s.runFrags(run, run.done)
	s.mu.Unlock()
	s.broadcast(frags)
}

// handleRunEvent reduces an event that belongs to an active workflow run, routing
// it to the right lane and advancing the run. Caller holds s.mu (it is called from
// handleEvent). It returns the SSE fragments to emit.
func (s *Server) handleRunEvent(run *workflowRun, e copilot.Event) []fragment {
	l := run.laneFor(e.SessionID)

	switch e.Type {
	case copilot.EvMessageDelta:
		if l != nil {
			l.appendText(e.Text)
		}
		return []fragment{s.lanesFrag()}

	case copilot.EvMessage:
		if l != nil {
			l.text = e.Text // the committed full message replaces the streamed deltas
		}
		return []fragment{s.lanesFrag()}

	case copilot.EvUsage:
		// A workflow run owns the turn: attribute the spend to the run and, when the
		// event routed to a lane, that lane's agent + index (ADR-0018).
		tag := spendTag{workflowID: run.id}
		if l != nil {
			tag.agentID = l.AgentID
			tag.laneIndex = l.Index
		}
		cost := s.recordUsage(e.Usage, tag)
		if l != nil {
			l.credits += cost.Credits()
		}
		return []fragment{
			{Event: "cost", HTML: renderCostFooter(s.monthToDate().Credits(), s.budget())},
			s.lanesFrag(), s.statFrag(),
		}

	case copilot.EvIdle:
		if l == nil {
			return nil
		}
		next := run.finishLane(l, l.costDetail())
		if len(next) > 0 {
			go s.launchLanes(run, next)
		}
		return s.runFrags(run, run.done)

	case copilot.EvToolStart:
		if l == nil {
			return nil
		}
		args := ""
		if e.ToolCall != nil {
			args = e.ToolCall.Args
		}
		l.toolStart(toolID(e), e.Tool, args)
		return []fragment{s.lanesFrag()}

	case copilot.EvToolProgress:
		if l == nil || e.ToolCall == nil {
			return nil
		}
		l.toolProgress(e.ToolCall.ID, e.ToolCall.Progress)
		return []fragment{s.lanesFrag()}

	case copilot.EvToolEnd:
		if l == nil || e.ToolCall == nil {
			return nil
		}
		l.toolEnd(e.ToolCall.ID, e.ToolCall.Result, e.ToolCall.Success)
		return []fragment{s.lanesFrag()}

	case copilot.EvPermission:
		if l == nil || e.Permission == nil {
			return nil
		}
		l.perms = append(l.perms, *e.Permission)
		return []fragment{s.lanesFrag()}

	case copilot.EvError:
		if l == nil {
			return []fragment{s.lanesFrag()}
		}
		msg := "lane error"
		if e.Err != nil {
			msg = e.Err.Error()
		}
		next := run.failLane(l, "✗ "+msg)
		if len(next) > 0 {
			go s.launchLanes(run, next)
		}
		return s.runFrags(run, run.done)

	default:
		// Reasoning and context events from a sub-run are not surfaced per-lane
		// (the lane shows output, its tool timeline, inline permissions, and cost);
		// ignore them.
		return nil
	}
}

// runFrags builds the lane-strip fragment plus, when the run has finished, the
// terminal status. On completion it clears the busy flag and notes the outcome in
// the transcript. Caller holds s.mu.
func (s *Server) runFrags(run *workflowRun, done bool) []fragment {
	if !done {
		return []fragment{s.lanesFrag()}
	}
	if run.recorded {
		// The terminal path already ran (e.g. an abort settled the run, then a still
		// in-flight lane goroutine erred and re-entered here, ADR-0024). Idempotent:
		// don't re-clear busy, re-record, or re-note the outcome — just re-render the
		// (already-settled) lanes.
		return []fragment{s.lanesFrag()}
	}
	run.recorded = true
	s.busy = false
	s.turnStartMs = 0
	// Persist the finished run to history. This is the one completion point — guarded by
	// run.recorded so it runs exactly once even if abort and a late lane error both reach
	// it — so the run is recorded once, including any skipped (branched) lanes (ADR-0022).
	s.recordRun(run)
	outcome := "✓ workflow " + run.name + " finished"
	if run.failed {
		outcome = "✗ workflow " + run.name + " stopped on a failed step"
	}
	s.state.AddSystem(outcome)
	return []fragment{
		{Event: "timeline", HTML: renderTimelineInner(&s.state)},
		s.lanesFrag(),
		s.statusFrag("", false),
	}
}

// recordRun appends the finished run to the persisted run history (ADR-0022). It is
// best-effort: a disk error is logged, never surfaced, so the run's terminal
// fragments still render. A no-op when no run store is wired. Caller holds s.mu.
func (s *Server) recordRun(run *workflowRun) {
	if s.runs == nil {
		return
	}
	if err := s.runs.Append(runRecord(run)); err != nil {
		s.logger.Printf("persist run history: %v", err)
	}
}

// runRecord maps a finished workflowRun onto a persisted RunRecord — a pure
// translation of the in-flight run into the immutable history shape (ADR-0022). Each
// lane carries its agent, settled status, and metered credits (zero for a skipped or
// free lane).
func runRecord(run *workflowRun) telemetry.RunRecord {
	lanes := make([]telemetry.RunLane, len(run.lanes))
	for i, l := range run.lanes {
		lanes[i] = telemetry.RunLane{
			Index: l.Index, AgentID: l.AgentID,
			Status: laneStatusName(l.status), Credits: l.credits,
		}
	}
	outcome := "finished"
	if run.failed {
		outcome = "failed"
	}
	return telemetry.RunRecord{
		ID: run.runID, WorkflowID: run.id, Name: run.name, Mode: run.mode,
		StartedAt: run.started, Outcome: outcome, Lanes: lanes,
	}
}

// laneStatusName is the persisted/string form of a settled lane status. It is total
// (an unsettled lane — which a finished run never has — reads as "pending").
func laneStatusName(st laneStatus) string {
	switch st {
	case laneRunning:
		return "running"
	case laneDone:
		return "done"
	case laneFailed:
		return "failed"
	case laneSkipped:
		return "skipped"
	default:
		return "pending"
	}
}

// costDetail formats a finished lane's metered cost for its summary line.
func (l *lane) costDetail() string {
	if l.credits <= 0 {
		return "done"
	}
	return telemetry.FormatCredits(l.credits)
}

// lanesFrag builds the workflow-lanes SSE fragment. Caller holds s.mu.
func (s *Server) lanesFrag() fragment {
	return fragment{Event: "lanes", HTML: renderLanes(s.run)}
}

// renderLanes renders the workflow run panel: a header plus one card per lane
// (status glyph, step/agent, collapsible output, cost/detail). Empty when no run
// is active, so the region is ambient like the sub-agent strip.
func renderLanes(run *workflowRun) string {
	if run == nil {
		return ""
	}
	lanes := make([]map[string]any, len(run.lanes))
	for i, l := range run.lanes {
		glyph, state := laneGlyph(l.status)
		lanes[i] = map[string]any{
			"Step": l.Index + 1, "Agent": l.AgentName, "Glyph": glyph, "State": state,
			"Output": l.text, "HasOutput": strings.TrimSpace(l.text) != "",
			"Detail": l.detail, "HasDetail": l.detail != "",
			"Tools": laneToolsHTML(l.tools), "HasTools": len(l.tools) > 0,
			"Perms": lanePermsHTML(l.perms), "HasPerms": len(l.perms) > 0,
		}
	}
	return frag("workflowLanes", map[string]any{
		"Name": run.name, "Mode": run.mode, "Running": !run.done, "Lanes": lanes,
	})
}

// laneToolsHTML renders a lane's own tool-execution timeline by reusing the chat
// tool card (so a sub-run's tools look identical to a chat turn's). The result is
// a composed-from-escaped-fragments HTML string — its args/results pass through
// the same richtext escaping as the chat timeline (ADR-0001).
func laneToolsHTML(tools []*convo.ToolView) template.HTML {
	var b strings.Builder
	for _, tv := range tools {
		b.WriteString(renderToolCard(tv))
	}
	return trusted(b.String())
}

// lanePermsHTML renders a lane's pending inline permission requests by reusing the
// chat permission form (the compact form or the diff review lane), so a lane's
// permissions are answerable in place via the same /perm/{id} flow (ADR-0012).
func lanePermsHTML(perms []copilot.PermissionRequest) template.HTML {
	var b strings.Builder
	for _, p := range perms {
		b.WriteString(renderPermForm(p))
	}
	return trusted(b.String())
}

// laneGlyph maps a live lane status to its glyph and CSS state class. It routes
// through the status-string vocabulary (laneStatusName → glyphFor) so a live lane and
// a persisted run-history lane (which only has the string) can never drift apart.
func laneGlyph(st laneStatus) (glyph, state string) {
	return glyphFor(laneStatusName(st))
}

// glyphFor is the single source of truth mapping a lane status string to its glyph and
// CSS state class — shared by live lanes (laneGlyph) and the persisted Runs history
// (runs.go), so the two render identically.
func glyphFor(status string) (glyph, state string) {
	switch status {
	case "running":
		return "◐", "running"
	case "done":
		return "✓", "done"
	case "failed":
		return "✗", "failed"
	case "skipped":
		return "⊘", "skipped"
	default:
		return "○", "pending"
	}
}

// streamDemoLane emits a scripted sub-run for one workflow lane so the lanes
// surface is exercised offline. Every event is tagged with the lane's backing
// session id (sid) so a PARALLEL run drives concurrent lanes that the reducer
// disambiguates by SessionID (workflow.go laneFor) — the offline mock can now
// cover the parallel path, not just sequential (B1 / issue 0015). It mirrors a
// real lane turn: a tool execution, an inline file-write permission, a short
// streamed answer, usage, and idle — so each lane shows its own tool timeline and
// inline permission, not just output + cost. Permissions don't block in demo mode.
func streamDemoLane(m *copilot.MockClient, sid, prompt string) {
	emit := func(e copilot.Event) {
		e.SessionID = sid
		m.Emit(e)
	}

	// A tool execution as a first-class per-lane timeline entry.
	emit(copilot.Event{Type: copilot.EvToolStart, Tool: "bash",
		ToolCall: &copilot.ToolCall{ID: sid + "-tool", Name: "bash", Args: "go test ./..."}})
	emit(copilot.Event{Type: copilot.EvToolEnd,
		ToolCall: &copilot.ToolCall{ID: sid + "-tool", Result: "ok\tgithub.com/dotts-h/copilot-sdk", Success: true}})

	// An inline file-write permission, rendered as a diff review lane inside this
	// lane's card. Nothing blocks on the decision in demo mode; submitting it
	// exercises the /perm route and refreshes #lanes.
	emit(copilot.Event{Type: copilot.EvPermission, Permission: &copilot.PermissionRequest{
		ID: sid + "-perm", Kind: "write", Detail: "write file: internal/lane.go",
		FileName: "internal/lane.go", Intention: "apply the change for this lane",
		Diff: "--- a/internal/lane.go\n" +
			"+++ b/internal/lane.go\n" +
			"@@ -1,3 +1,4 @@\n" +
			" package internal\n" +
			" \n" +
			"-func todo() {}\n" +
			"+// done by the lane\n" +
			"+func done() {}\n",
	}})

	// The streamed answer.
	reply := "Handled: " + firstLine(prompt)
	for _, tok := range tokenize(reply) {
		emit(copilot.Event{Type: copilot.EvMessageDelta, Text: tok})
	}
	emit(copilot.Event{Type: copilot.EvMessage, Text: reply})
	emit(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 600, OutputTokens: 120,
	}})
	emit(copilot.Event{Type: copilot.EvIdle})
}

// firstLine returns the first non-empty line of s, trimmed and bounded, for a
// compact demo lane summary.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return truncate(t, 60)
		}
	}
	return "the task"
}

// --- Workflows CRUD page (mirrors the agents page) ---

func (s *Server) workflowsPartial() string {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	// Join the two pure readers keyed by workflow id, each guarded for an absent store:
	// RunAggregates carries the last-run signal + run count (RunStore), WorkflowShares
	// the authoritative per-workflow spend (SpendStore). The row already knows its own
	// id/name, so nothing new is resolved — the join only badges each row (V4). With no
	// store wired (or no record for a workflow), the maps stay empty and the row renders
	// its prior shape, no badges. Read under forgeMu like runsPartial; the stores' own
	// mutex is a leaf lock, so no lock-order risk.
	runAggs := map[string]telemetry.RunAggregate{}
	if s.runs != nil {
		for _, a := range telemetry.RunAggregates(s.runs.Records()) {
			runAggs[a.WorkflowID] = a
		}
	}
	spendShares := map[string]telemetry.WorkflowShare{}
	if s.spend != nil {
		for _, sh := range telemetry.WorkflowShares(s.spend.Records()) {
			spendShares[sh.WorkflowID] = sh
		}
	}
	rows := make([]map[string]any, 0, len(s.forge.Workflows))
	for _, w := range s.forge.Workflows {
		agents := make([]string, len(w.Steps))
		for i, st := range w.Steps {
			agents[i] = st.AgentID
		}
		desc := w.EffectiveMode() + " · " + strings.Join(agents, " → ")
		if w.EffectiveMode() == ctxforge.WorkflowParallel {
			desc = w.EffectiveMode() + " · " + strings.Join(agents, " ‖ ")
		}
		row := map[string]any{"ID": w.ID, "Name": w.Name, "Desc": truncate(desc, 90)}
		// Last-run + run-count badges (RunStore). Guard on Runs>0 so a workflow with no
		// run history shows no run badge, just navigation.
		if a, ok := runAggs[w.ID]; ok && a.Runs > 0 {
			glyph, state := runOutcomeGlyph(a.LastOutcome)
			row["HasRuns"] = true
			row["Runs"] = a.Runs
			row["LastGlyph"] = glyph
			row["LastState"] = state
			row["LastOutcome"] = a.LastOutcome
			row["LastWhen"] = humanWhen(a.LastStartedAt)
		}
		// Spend badge (SpendStore): the workflow's total metered credits.
		if sh, ok := spendShares[w.ID]; ok {
			row["HasSpend"] = true
			row["Spend"] = telemetry.FormatCredits(sh.Credits)
		}
		rows = append(rows, row)
	}
	return frag("workflowsPage", map[string]any{"Add": addData("workflows", "workflow"), "Rows": rows})
}

var workflowModeOpts = []string{ctxforge.WorkflowSequential, ctxforge.WorkflowParallel}

func renderWorkflowForm(w ctxforge.Workflow, isNew bool, errMsg string) string {
	title, action := "Edit workflow", "/workflows/"+w.ID
	if isNew {
		title, action = "New workflow", "/workflows"
	}
	mode := w.Mode
	if mode == "" {
		mode = ctxforge.WorkflowSequential
	}
	return formShell(title, action, "workflows", errMsg,
		idField(w.ID, isNew),
		textField("Name", "name", w.Name, true),
		textField("Description", "description", w.Description, false),
		selectField("Mode", "mode", mode, workflowModeOpts),
		textArea("Steps (one per line: agentID [step N condition value]: prompt)", "steps", stepsToText(w.Steps), true),
	)
}

// stepsToText renders a workflow's steps as the textarea's "agentID: prompt" lines
// (with an optional "[step N condition value]" predicate prefix on a gated step),
// the inverse of stepsFromText.
func stepsToText(steps []ctxforge.WorkflowStep) string {
	var b strings.Builder
	for _, st := range steps {
		b.WriteString(st.AgentID)
		if cond := formatStepCondition(st.When); cond != "" {
			b.WriteString(" [")
			b.WriteString(cond)
			b.WriteString("]")
		}
		b.WriteString(": ")
		b.WriteString(st.Prompt)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// stepsFromText parses the steps textarea: one "agentID [predicate]: prompt" per
// non-blank line, where the bracketed predicate is optional. A line with no colon is
// treated as an agent id with an empty prompt, so Validate surfaces a clear "prompt
// is required" rather than silently dropping it.
func stepsFromText(raw string) []ctxforge.WorkflowStep {
	var steps []ctxforge.WorkflowStep
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		head, prompt := splitStepLine(line)
		agentPart := strings.TrimSpace(head)
		var when *ctxforge.StepCondition
		if i := strings.IndexByte(agentPart, '['); i >= 0 {
			if j := strings.IndexByte(agentPart, ']'); j > i {
				when = parseStepCondition(agentPart[i+1 : j])
			}
			agentPart = strings.TrimSpace(agentPart[:i])
		}
		steps = append(steps, ctxforge.WorkflowStep{
			AgentID: agentPart, Prompt: strings.TrimSpace(prompt), When: when,
		})
	}
	return steps
}

// splitStepLine splits a step line into its "agentID [predicate]" head and prompt
// at the separator colon. The colon is the first one AFTER any closing "]", so an
// output-contains value inside the bracket may itself contain a colon without
// derailing the split; with no bracket it is the first colon (as before). A line
// with no qualifying colon is all head (empty prompt), so Validate surfaces a clear
// "prompt is required".
func splitStepLine(line string) (head, prompt string) {
	sep := strings.IndexByte(line, ':')
	if b := strings.IndexByte(line, '['); b >= 0 {
		if c := strings.IndexByte(line, ']'); c > b {
			if rel := strings.IndexByte(line[c:], ':'); rel >= 0 {
				sep = c + rel
			} else {
				sep = -1
			}
		}
	}
	if sep < 0 {
		return line, ""
	}
	return line[:sep], line[sep+1:]
}

// formatStepCondition renders a predicate as the bracket body for stepsToText: ""
// for a nil predicate (ungated), "always", or "step N condition [value]".
func formatStepCondition(c *ctxforge.StepCondition) string {
	if c == nil {
		return ""
	}
	if c.Condition == ctxforge.CondAlways {
		return ctxforge.CondAlways
	}
	s := fmt.Sprintf("step %d %s", c.Step, c.Condition)
	if c.Condition == ctxforge.CondOutputContains && c.Value != "" {
		s += " " + c.Value
	}
	return s
}

// parseStepCondition parses the bracket body from a step line back into a predicate
// ("always", or "step N condition [value...]"). A malformed body is kept as a raw
// condition so the whole-forge Validate surfaces the error rather than silently
// dropping the predicate. An empty body yields nil (ungated).
func parseStepCondition(spec string) *ctxforge.StepCondition {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	if spec == ctxforge.CondAlways {
		return &ctxforge.StepCondition{Condition: ctxforge.CondAlways}
	}
	fields := strings.Fields(spec)
	if len(fields) >= 3 && fields[0] == "step" {
		n, _ := strconv.Atoi(fields[1])
		return &ctxforge.StepCondition{
			Step:      n,
			Condition: fields[2],
			Value:     strings.TrimSpace(strings.Join(fields[3:], " ")),
		}
	}
	return &ctxforge.StepCondition{Condition: spec}
}

func workflowFromForm(r *http.Request, id string) ctxforge.Workflow {
	return ctxforge.Workflow{
		ID:          id,
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Mode:        strings.TrimSpace(r.FormValue("mode")),
		Steps:       stepsFromText(r.FormValue("steps")),
	}
}

func (s *Server) handleWorkflowNew(w http.ResponseWriter, r *http.Request) {
	s.writePartial(w, renderWorkflowForm(ctxforge.Workflow{Mode: ctxforge.WorkflowSequential}, true, ""))
}

func (s *Server) handleWorkflowEdit(w http.ResponseWriter, r *http.Request) {
	s.hub.forgeMu.Lock()
	wf := s.forge.Workflow(r.PathValue("id"))
	var form string
	if wf != nil {
		form = renderWorkflowForm(*wf, false, "")
	}
	s.hub.forgeMu.Unlock()
	if form == "" {
		s.writePartial(w, s.workflowsPartial())
		return
	}
	s.writePartial(w, form)
}

func (s *Server) handleWorkflowCreate(w http.ResponseWriter, r *http.Request) {
	wf := workflowFromForm(r, strings.TrimSpace(r.FormValue("id")))
	if err := s.editForge(func() error { return s.forge.AddWorkflow(wf) }); err != nil {
		s.writePartial(w, renderWorkflowForm(wf, true, err.Error()))
		return
	}
	s.writePartial(w, s.workflowsPartial())
}

func (s *Server) handleWorkflowUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wf := workflowFromForm(r, id)
	if err := s.editForge(func() error { return s.forge.UpdateWorkflow(id, wf) }); err != nil {
		s.writePartial(w, renderWorkflowForm(wf, false, err.Error()))
		return
	}
	s.writePartial(w, s.workflowsPartial())
}

func (s *Server) handleWorkflowDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.editForge(func() error { return s.forge.RemoveWorkflow(r.PathValue("id")) }); err != nil {
		s.logger.Printf("remove workflow: %v", err)
	}
	s.writePartial(w, s.workflowsPartial())
}
