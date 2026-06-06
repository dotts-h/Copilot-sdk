package web

import (
	"context"
	"net/http"
	"strings"

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
)

// lane is one step of a running workflow: its agent, task prompt, compiled spec,
// the backing copilot session, the accumulated output, and live status/cost.
type lane struct {
	Index     int
	AgentID   string
	AgentName string
	Prompt    string
	spec      copilot.SessionSpec
	status    laneStatus
	text      string  // accumulated assistant output (deltas, replaced by the full message)
	detail    string  // one-line summary on completion (cost) or the error message
	sessionID string  // backing copilot session id, for SessionID-keyed event routing
	credits   float64 // metered cost attributed to this lane
}

// workflowRun is a workflow in flight: the lanes and the sequential cursor. It is
// a pure state machine — every method mutates only the run and returns the lane
// indices the adapter should launch next, with no IO — so it is fully testable
// without a client. The Server guards it with s.mu.
type workflowRun struct {
	id     string
	name   string
	mode   string
	lanes  []*lane
	cur    int // index of the running lane in sequential mode
	done   bool
	failed bool
}

// newWorkflowRun builds a run with one pending lane per compiled step. specs are
// the per-step copilot SessionSpecs (translated from the forge's compiled specs
// with config fallbacks applied), parallel to steps.
func newWorkflowRun(wf ctxforge.Workflow, steps []ctxforge.CompiledStep, specs []copilot.SessionSpec) *workflowRun {
	lanes := make([]*lane, len(steps))
	for i, st := range steps {
		lanes[i] = &lane{
			Index: i, AgentID: st.AgentID, AgentName: st.AgentName,
			Prompt: st.Prompt, spec: specs[i], status: lanePending,
		}
	}
	return &workflowRun{id: wf.ID, name: wf.Name, mode: wf.EffectiveMode(), lanes: lanes}
}

// start marks the lanes to launch first and returns their indices: the first lane
// in sequential mode, every lane in parallel mode.
func (r *workflowRun) start() []int {
	if len(r.lanes) == 0 {
		r.done = true
		return nil
	}
	if r.mode == ctxforge.WorkflowParallel {
		idxs := make([]int, len(r.lanes))
		for i, l := range r.lanes {
			l.status = laneRunning
			idxs[i] = i
		}
		return idxs
	}
	r.cur = 0
	r.lanes[0].status = laneRunning
	return []int{0}
}

// handoffPrompt is the prompt to Send for lane idx. In a sequential run every step
// after the first receives the previous lane's output appended as a handoff so the
// chain composes; in parallel each lane starts from its own prompt alone.
func (r *workflowRun) handoffPrompt(idx int) string {
	l := r.lanes[idx]
	if r.mode == ctxforge.WorkflowParallel || idx == 0 {
		return l.Prompt
	}
	prev := r.lanes[idx-1]
	if strings.TrimSpace(prev.text) == "" {
		return l.Prompt
	}
	return l.Prompt + "\n\n--- Handoff from " + prev.AgentName + " ---\n" + strings.TrimSpace(prev.text)
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
// launch next (the next step in sequential mode once the current one completes;
// none in parallel until every lane is finished). It sets done when the run is over.
func (r *workflowRun) finishLane(l *lane, detail string) []int {
	if l.status == laneRunning {
		l.status = laneDone
	}
	l.detail = detail
	if r.mode == ctxforge.WorkflowParallel {
		if r.allSettled() {
			r.done = true
		}
		return nil
	}
	// Sequential: hand off to the next pending lane.
	next := l.Index + 1
	if next >= len(r.lanes) {
		r.done = true
		return nil
	}
	r.cur = next
	r.lanes[next].status = laneRunning
	return []int{next}
}

// failLane marks a lane failed. A sequential run aborts (the chain is broken); a
// parallel run lets the surviving lanes finish. Returns done.
func (r *workflowRun) failLane(l *lane, msg string) bool {
	l.status = laneFailed
	l.detail = msg
	if r.mode == ctxforge.WorkflowParallel {
		if r.allSettled() {
			r.done = true
		}
	} else {
		r.done = true
		r.failed = true
	}
	return r.done
}

// allSettled reports whether every lane has finished (done or failed).
func (r *workflowRun) allSettled() bool {
	for _, l := range r.lanes {
		if l.status == lanePending || l.status == laneRunning {
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
	id := r.PathValue("id")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

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
		s.writePartial(w, s.workflowsPartial())
		return
	}

	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		// A turn or another run is in flight — surface a note and stay put.
		s.writePartial(w, s.workflowsPartial())
		return
	}
	run := newWorkflowRun(wf, steps, specs)
	s.run = run
	s.busy = true
	s.turnStartMs = nowMs()
	launch := run.start()
	s.state.AddSystem("▶ workflow " + wf.Name + " started (" + run.mode + ")")
	s.mu.Unlock()

	go s.launchLanes(run, launch)
	// Land the user on the Chat page, where the lanes panel streams each step;
	// subsequent lane updates arrive over SSE (the body-level #lanes listener).
	s.writePartial(w, s.chatPartial())
}

// workflowLaneSpec translates a forge-compiled step spec into the seam's
// SessionSpec via the shared SeamSpec translation. Caller holds forgeMu (it reads
// s.config).
func (s *Server) workflowLaneSpec(cs ctxforge.SessionSpec) copilot.SessionSpec {
	var defModel, defEffort string
	if s.config != nil {
		defModel, defEffort = s.config.DefaultModel, s.config.ReasoningEffort
	}
	return SeamSpec(cs, defModel, defEffort)
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
			go streamDemoLane(mock, prompt)
		}
	}
}

// laneError fails a lane and broadcasts the resulting lane/status fragments.
func (s *Server) laneError(run *workflowRun, l *lane, err error) {
	s.mu.Lock()
	done := run.failLane(l, "✗ "+err.Error())
	frags := s.runFrags(run, done)
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
		usage := telemetry.Usage{
			Model:            e.Usage.Model,
			InputTokens:      e.Usage.InputTokens,
			CachedTokens:     e.Usage.CachedTokens,
			CacheWriteTokens: e.Usage.CacheWriteTokens,
			OutputTokens:     e.Usage.OutputTokens,
			ReasoningTokens:  e.Usage.ReasoningTokens,
		}
		cost := s.meter.Record(usage)
		s.sessionMeter.Record(usage)
		aiu := e.Usage.NanoAIU * 1e-9
		s.meter.RecordReportedAIU(aiu)
		if l != nil {
			l.credits += cost.Credits()
		}
		if s.spend != nil {
			rec := telemetry.SpendRecord{
				SessionID: s.sessionID, Model: e.Usage.Model,
				InputTokens: e.Usage.InputTokens, CachedTokens: e.Usage.CachedTokens,
				OutputTokens: e.Usage.OutputTokens, USD: cost.USD(), AIU: aiu,
			}
			if err := s.spend.Append(rec); err != nil {
				s.logger.Printf("persist spend: %v", err)
			}
		}
		return []fragment{
			{Event: "cost", HTML: renderCostFooter(s.meter, s.budget())},
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

	case copilot.EvError:
		if l == nil {
			return []fragment{s.lanesFrag()}
		}
		msg := "lane error"
		if e.Err != nil {
			msg = e.Err.Error()
		}
		done := run.failLane(l, "✗ "+msg)
		return s.runFrags(run, done)

	default:
		// Reasoning, tool, permission, and context events from a sub-run are not
		// surfaced per-lane (the lane shows its output + cost); ignore them.
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
	s.busy = false
	s.turnStartMs = 0
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
		}
	}
	return frag("workflowLanes", map[string]any{
		"Name": run.name, "Mode": run.mode, "Running": !run.done, "Lanes": lanes,
	})
}

// laneGlyph maps a lane status to its glyph and CSS state class.
func laneGlyph(st laneStatus) (glyph, state string) {
	switch st {
	case laneRunning:
		return "◐", "running"
	case laneDone:
		return "✓", "done"
	case laneFailed:
		return "✗", "failed"
	default:
		return "○", "pending"
	}
}

// streamDemoLane emits a scripted sub-run for one workflow lane so the lanes
// surface is exercised offline (sequential mode, one lane active at a time). It
// mirrors a real lane turn: a short streamed answer, usage, and idle.
func streamDemoLane(m *copilot.MockClient, prompt string) {
	reply := "Handled: " + firstLine(prompt)
	for _, tok := range tokenize(reply) {
		m.Emit(copilot.Event{Type: copilot.EvMessageDelta, Text: tok})
	}
	m.Emit(copilot.Event{Type: copilot.EvMessage, Text: reply})
	m.Emit(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 600, OutputTokens: 120,
	}})
	m.Emit(copilot.Event{Type: copilot.EvIdle})
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
		rows = append(rows, map[string]any{
			"ID": w.ID, "Name": w.Name, "Desc": truncate(desc, 90),
		})
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
		textArea("Steps (one per line: agentID: prompt)", "steps", stepsToText(w.Steps), true),
	)
}

// stepsToText renders a workflow's steps as the textarea's "agentID: prompt"
// lines, the inverse of stepsFromText.
func stepsToText(steps []ctxforge.WorkflowStep) string {
	var b strings.Builder
	for _, st := range steps {
		b.WriteString(st.AgentID)
		b.WriteString(": ")
		b.WriteString(st.Prompt)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// stepsFromText parses the steps textarea: one "agentID: prompt" per non-blank
// line. A line with no colon is treated as an agent id with an empty prompt, so
// Validate surfaces a clear "prompt is required" rather than silently dropping it.
func stepsFromText(raw string) []ctxforge.WorkflowStep {
	var steps []ctxforge.WorkflowStep
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		agent, prompt, _ := strings.Cut(line, ":")
		steps = append(steps, ctxforge.WorkflowStep{
			AgentID: strings.TrimSpace(agent), Prompt: strings.TrimSpace(prompt),
		})
	}
	return steps
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
