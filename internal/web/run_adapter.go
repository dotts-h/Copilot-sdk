package web

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/pause"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file is the Server adapter for the run engine (run_engine.go): it maps each
// pure workflowRun lane onto the seam's session lifecycle, reduces the resulting
// events back into the run, and persists the finished run to history. The engine is
// pure; this is the thin, IO-bearing shell around it. See ADR-0013.

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
	// Close any open pause span BEFORE recording the run: abort is the one completion
	// path that records while a lane is still parked, and the escalate goroutine that
	// would otherwise fold the span in (closeLanePause) can't re-acquire s.mu until we
	// release it below — after runFrags has already persisted the run. Fold it here so
	// an abort-while-parked attributes the full wait; the goroutine's later call finds
	// the mark cleared and no-ops (S6).
	for _, l := range run.lanes {
		s.closeLanePauseLane(l)
	}
	// Force-resolve any pending pause so a blocked escalate goroutine unblocks and
	// its lane settles (ADR-0024 + S4): an abort that left a pause open would leak
	// the tool-handler goroutine and the run would never clear busy.
	s.pauses.CancelAll("run aborted")
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
	var autoApprove bool
	if s.config != nil {
		defModel, defEffort = s.config.DefaultModel, s.config.ReasoningEffort
		autoApprove = s.config.AutoApproveTools
	}
	return SeamSpec(cs, defModel, defEffort, s.lookupEnv, s.hub.baseSpec.Workspace, autoApprove)
}

// laneWorkerCount is the fixed size of the worker pool that bounds concurrent
// in-flight lane sessions. Excess lanes wait in the queue until a worker is free
// (backpressure). The value is large enough for typical parallel workflows (a few
// agents) while preventing unbounded fan-out on wide workflows (issue 0084).
const laneWorkerCount = 8

// launchLanes starts each given lane via a bounded worker pool so that at most
// laneWorkerCount lanes are in-flight concurrently. Lanes beyond the cap queue and
// start as workers free up — no lane is dropped. Result ordering is still driven by
// finishLane/evalPending, not by the order lanes are dispatched here.
//
// Lock order: workers call startLane, which may acquire s.mu and s.hub.forgeMu
// internally (forgeMu → s.mu, never inverted). launchLanes itself holds no lock.
func (s *Server) launchLanes(run *workflowRun, idxs []int) {
	if len(idxs) == 0 {
		return
	}

	queue := make(chan int, len(idxs))
	for _, i := range idxs {
		queue <- i
	}
	close(queue)

	for range min(laneWorkerCount, len(idxs)) {
		go func() {
			for i := range queue {
				s.startLane(run, i)
			}
		}()
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
			go streamDemoLane(mock, cid, prompt, s.escalate)
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
			{Event: "cost", HTML: renderActualCostFooter(s.monthToDateActual(), s.budget())},
			s.lanesFrag(), s.statFrag(),
		}

	case copilot.EvIdle:
		if l == nil {
			return nil
		}
		// A cooperatively-cancelled lane wrapped up its turn: settle it failed
		// (cancelled) rather than done, recording why (S4 / ADR-0043).
		var next []int
		if l.cancelReason != "" {
			next = run.failLane(l, "✗ cancelled: "+l.cancelReason)
		} else {
			next = run.finishLane(l, l.costDetail())
		}
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
			Pauses: l.pauses, PausedMs: l.pausedDur.Milliseconds(),
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

// demoEscalateMarker is the phrase in the seeded "Escalation demo" workflow prompt
// that triggers streamDemoLane's scripted escalation. It is the full clause (not a
// bare "escalate") so it is specific to that demo and survives the firstLine handoff
// truncation without tripping unrelated sequential lanes. — S4 / ADR-0043.
const demoEscalateMarker = "escalate if the requirements are ambiguous"

// streamDemoLane emits a scripted sub-run for one workflow lane so the lanes
// surface is exercised offline. Every event is tagged with the lane's backing
// session id (sid) so a PARALLEL run drives concurrent lanes that the reducer
// disambiguates by SessionID (run_engine.go laneFor) — the offline mock can now
// cover the parallel path, not just sequential (B1 / issue 0015). It mirrors a
// real lane turn: a tool execution, an inline file-write permission, a short
// streamed answer, usage, and idle — so each lane shows its own tool timeline and
// inline permission, not just output + cost. Permissions don't block in demo mode.
func streamDemoLane(m *copilot.MockClient, sid, prompt string, escalate func(escalateReq) string) {
	emit := func(e copilot.Event) {
		e.SessionID = sid
		m.Emit(e)
	}

	// A tool execution as a first-class per-lane timeline entry.
	emit(copilot.Event{Type: copilot.EvToolStart, Tool: "bash",
		ToolCall: &copilot.ToolCall{ID: sid + "-tool", Name: "bash", Args: "go test ./..."}})
	emit(copilot.Event{Type: copilot.EvToolEnd,
		ToolCall: &copilot.ToolCall{ID: sid + "-tool", Result: "ok\tgithub.com/dotts-h/copilot-sdk", Success: true}})

	// A scripted human-in-the-loop escalation (S4): when the lane's task asks for it,
	// the sub-agent parks as input-required via the orchestrator's escalate
	// back-channel and BLOCKS until the human resolves the pause — driving the pause
	// surface offline (the e2e clicks continue/cancel). The human's answer (or the
	// cancel directive) is folded into the lane's reply so the round-trip is visible.
	// The trigger is the seeded "Escalation demo" prompt's full marker phrase, not a
	// bare "escalate" substring: a sequential handoff echoes only firstLine(prompt)
	// (truncated before "ambiguous"), so an upstream lane's output can't accidentally
	// trip a downstream lane, and an ordinary user prompt mentioning "escalate" won't.
	steer := ""
	if escalate != nil && strings.Contains(strings.ToLower(prompt), demoEscalateMarker) {
		steer = escalate(escalateReq{
			laneSession: sid, agentID: sid, kind: pause.KindIssue,
			message: "The task is ambiguous — continue with a hint, or cancel?",
			caps:    []pause.Cap{pause.CapContinue, pause.CapCancel},
		})
	}

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

	// The streamed answer, folding in the human's escalation answer when one was given.
	reply := "Handled: " + firstLine(prompt)
	if steer != "" {
		reply += " — " + steer
	}
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
