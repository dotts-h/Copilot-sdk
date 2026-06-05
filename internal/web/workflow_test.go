package web

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// twoStepRun builds a run with two lanes (Alpha → Beta) in the given mode, with no
// client involved — for testing the pure run state machine.
func twoStepRun(mode string) *workflowRun {
	wf := ctxforge.Workflow{ID: "w", Name: "W", Mode: mode}
	steps := []ctxforge.CompiledStep{
		{AgentID: "a", AgentName: "Alpha", Prompt: "first"},
		{AgentID: "b", AgentName: "Beta", Prompt: "second"},
	}
	specs := []copilot.SessionSpec{{Model: "gpt-5"}, {Model: "gpt-5"}}
	return newWorkflowRun(wf, steps, specs)
}

// --- pure run state machine ---

func TestRunSequentialHandoff(t *testing.T) {
	r := twoStepRun(ctxforge.WorkflowSequential)
	if got := r.start(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("start() = %v, want [0]", got)
	}
	if r.lanes[0].status != laneRunning {
		t.Fatal("lane 0 should be running after start")
	}
	if p := r.handoffPrompt(0); p != "first" {
		t.Errorf("first lane prompt = %q, want plain prompt", p)
	}
	r.lanes[0].appendText("alpha-output")

	next := r.finishLane(r.lanes[0], "done")
	if len(next) != 1 || next[0] != 1 {
		t.Fatalf("finishLane advanced to %v, want [1]", next)
	}
	if r.lanes[1].status != laneRunning {
		t.Fatal("lane 1 should be running after handoff")
	}
	// The second lane's prompt carries the first lane's output as a handoff.
	p := r.handoffPrompt(1)
	if !strings.Contains(p, "second") || !strings.Contains(p, "alpha-output") || !strings.Contains(p, "Handoff from Alpha") {
		t.Errorf("handoff prompt missing pieces: %q", p)
	}

	if next := r.finishLane(r.lanes[1], "done"); next != nil {
		t.Errorf("finishing the last lane should launch nothing, got %v", next)
	}
	if !r.done || r.failed {
		t.Errorf("run should be done and not failed: done=%v failed=%v", r.done, r.failed)
	}
}

func TestRunParallelStartsAllNoHandoff(t *testing.T) {
	r := twoStepRun(ctxforge.WorkflowParallel)
	got := r.start()
	if len(got) != 2 {
		t.Fatalf("parallel start() = %v, want both lanes", got)
	}
	for _, l := range r.lanes {
		if l.status != laneRunning {
			t.Fatal("all parallel lanes should be running after start")
		}
	}
	if p := r.handoffPrompt(1); p != "second" {
		t.Errorf("parallel lane gets no handoff, prompt = %q", p)
	}
	if next := r.finishLane(r.lanes[0], "done"); next != nil || r.done {
		t.Errorf("run not done until all lanes settle: next=%v done=%v", next, r.done)
	}
	if next := r.finishLane(r.lanes[1], "done"); next != nil || !r.done {
		t.Errorf("run should be done after the last lane: next=%v done=%v", next, r.done)
	}
}

func TestRunSequentialFailAborts(t *testing.T) {
	r := twoStepRun(ctxforge.WorkflowSequential)
	r.start()
	if done := r.failLane(r.lanes[0], "boom"); !done {
		t.Fatal("a failed step aborts a sequential run")
	}
	if !r.failed {
		t.Error("run should be marked failed")
	}
	if r.lanes[1].status != lanePending {
		t.Error("the next sequential lane should never start after a failure")
	}
}

func TestRunParallelFailLetsOthersFinish(t *testing.T) {
	r := twoStepRun(ctxforge.WorkflowParallel)
	r.start()
	if done := r.failLane(r.lanes[0], "boom"); done {
		t.Fatal("one failed lane should not end a parallel run while another runs")
	}
	r.finishLane(r.lanes[1], "done")
	if !r.done {
		t.Fatal("run ends once every lane settles")
	}
}

func TestLaneForRouting(t *testing.T) {
	// Parallel: both lanes running → must route by session id.
	r := twoStepRun(ctxforge.WorkflowParallel)
	r.start()
	r.lanes[0].sessionID, r.lanes[1].sessionID = "s0", "s1"
	if r.laneFor("s1") != r.lanes[1] {
		t.Error("laneFor should match by session id")
	}
	if r.laneFor("") != nil {
		t.Error("an empty session id with two running lanes is ambiguous → nil")
	}

	// Sequential: exactly one running lane → empty session id routes to it (mock).
	seq := twoStepRun(ctxforge.WorkflowSequential)
	seq.start()
	if seq.laneFor("") != seq.lanes[0] {
		t.Error("laneFor should fall back to the sole running lane")
	}
}

// --- reducer integration (sequential, via the mock seam) ---

// startSeqRun installs a two-step sequential run on s and marks lane 0 running,
// without launching any client work, so a test can drive the lane events directly.
func startSeqRun(s *Server) *workflowRun {
	run := twoStepRun(ctxforge.WorkflowSequential)
	s.mu.Lock()
	s.run = run
	s.busy = true
	run.start()
	s.mu.Unlock()
	return run
}

func TestWorkflowRunReducerSequential(t *testing.T) {
	s, _ := newTestServer()
	run := startSeqRun(s)

	// Lane 0 streams an answer; it lands in the lanes fragment, not the timeline.
	s.handleEvent(copilot.Event{Type: copilot.EvMessageDelta, Text: "alpha "})
	lanes := fragFor(s, copilot.Event{Type: copilot.EvMessage, Text: "alpha result"}, "lanes")
	if !strings.Contains(lanes, "alpha result") || !strings.Contains(lanes, "Alpha") {
		t.Fatalf("lane 0 output should render in the lanes panel: %q", lanes)
	}

	// Usage is metered (the multi-run cost accounting) and attributed to the lane.
	before := s.meter.Totals().Credits()
	s.handleEvent(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 1000, OutputTokens: 200,
	}})
	if s.meter.Totals().Credits() <= before {
		t.Error("workflow usage should be recorded in the account meter")
	}
	if run.lanes[0].credits <= 0 {
		t.Error("lane should accrue its metered cost")
	}

	// Idle finalizes lane 0 and hands off to lane 1.
	s.handleEvent(copilot.Event{Type: copilot.EvIdle})
	if run.lanes[0].status != laneDone || run.lanes[1].status != laneRunning {
		t.Fatalf("idle should finish lane 0 and start lane 1: %v %v",
			run.lanes[0].status, run.lanes[1].status)
	}

	// Lane 1 finishes → the run completes, busy clears, a timeline note is added.
	s.handleEvent(copilot.Event{Type: copilot.EvMessage, Text: "beta result"})
	frags := s.handleEvent(copilot.Event{Type: copilot.EvIdle})
	if !run.done {
		t.Fatal("run should be done after the last lane")
	}
	s.mu.Lock()
	busy := s.busy
	s.mu.Unlock()
	if busy {
		t.Error("busy should clear when the run finishes")
	}
	var sawTimeline bool
	for _, f := range frags {
		if f.Event == "timeline" && strings.Contains(f.HTML, "finished") {
			sawTimeline = true
		}
	}
	if !sawTimeline {
		t.Error("a completed run should add a 'finished' timeline note")
	}
}

func TestSendBlockedDuringWorkflowRun(t *testing.T) {
	hub, mock := newWorkflowHub()
	s := hub.newSession("t")
	startSeqRun(s)

	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp, err := http.PostForm(srv.URL+"/send", url.Values{"prompt": {"hi there"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "workflow is running") {
		t.Errorf("a send during a run should be refused with a note: %q", string(body))
	}
	if mock.SentCount() != 0 {
		t.Error("the prompt must not be dispatched while a run owns the turn")
	}
}

func TestWorkflowLanesEscapeModelText(t *testing.T) {
	s, _ := newTestServer()
	run := twoStepRun(ctxforge.WorkflowSequential)
	run.lanes[0].AgentName = "Al<pha>"
	s.mu.Lock()
	s.run = run
	s.busy = true
	run.start()
	s.mu.Unlock()

	lanes := fragFor(s, copilot.Event{Type: copilot.EvMessage, Text: "<script>x</script>"}, "lanes")
	if strings.Contains(lanes, "<script>") || strings.Contains(lanes, "Al<pha>") {
		t.Fatalf("lane text must be HTML-escaped: %q", lanes)
	}
	if !strings.Contains(lanes, "Al&lt;pha&gt;") {
		t.Errorf("escaped agent name missing: %q", lanes)
	}
}

// --- handlers & CRUD ---

// newWorkflowHub builds a hub whose forge has two agents and a runnable workflow.
func newWorkflowHub() (*Hub, *copilot.MockClient) {
	mock := copilot.NewMockClient()
	forge := &ctxforge.Forge{}
	_ = forge.AddAgent(ctxforge.Agent{ID: "builder", Name: "Builder", Model: "gpt-5"})
	_ = forge.AddAgent(ctxforge.Agent{ID: "sdet", Name: "SDET", Model: "gpt-5"})
	_ = forge.AddWorkflow(ctxforge.Workflow{
		ID: "ship", Name: "Ship", Mode: ctxforge.WorkflowSequential,
		Steps: []ctxforge.WorkflowStep{
			{AgentID: "builder", Prompt: "build"},
			{AgentID: "sdet", Prompt: "harden"},
		},
	})
	hub := New(Options{
		Client: mock, Forge: forge,
		Config: &config.Config{DefaultModel: "gpt-5"},
		Meter:  telemetry.NewMeter(telemetry.DefaultPriceBook()),
		Logger: log.New(io.Discard, "", 0),
	})
	return hub, mock
}

func TestWorkflowsPageLists(t *testing.T) {
	hub, _ := newWorkflowHub()
	s := hub.newSession("t")
	html := s.renderPage("workflows")
	if !strings.Contains(html, "Ship") || !strings.Contains(html, "builder") {
		t.Errorf("workflows page should list the workflow: %q", html)
	}
	if !strings.Contains(html, "/workflows/ship/run") {
		t.Errorf("workflows page should offer a run control: %q", html)
	}
}

func TestWorkflowRunHandlerStartsLanes(t *testing.T) {
	hub, mock := newWorkflowHub()
	s := hub.newSession("t")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/workflows/ship/run", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// The run lands the user on the chat page with the lanes panel showing.
	if !strings.Contains(string(body), "workflow-run") || !strings.Contains(string(body), "Builder") {
		t.Errorf("run response should render the lanes panel: %q", string(body))
	}
	// The first lane opens a backing session and sends its prompt (give the
	// launch goroutine a moment).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if mock.SentCount() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if mock.SentCount() < 1 {
		t.Fatal("the first lane should send its prompt to a backing session")
	}
	if got := mock.SentAt(0); got != "build" {
		t.Errorf("first lane prompt = %q, want %q", got, "build")
	}
	s.mu.Lock()
	running := s.run != nil && s.busy
	s.mu.Unlock()
	if !running {
		t.Error("a run should be active after starting the workflow")
	}
}

func TestWorkflowCreateValidationError(t *testing.T) {
	hub, _ := newWorkflowHub()
	s := hub.newSession("t")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	form := url.Values{
		"id": {"bad"}, "name": {"Bad"}, "mode": {"sequential"},
		"steps": {"ghost: do something"}, // references an unknown agent
	}
	resp, err := http.PostForm(srv.URL+"/workflows", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "unknown agent") {
		t.Errorf("invalid workflow should re-render the form with the error: %q", string(body))
	}
	s.hub.forgeMu.Lock()
	created := s.forge.Workflow("bad")
	s.hub.forgeMu.Unlock()
	if created != nil {
		t.Error("an invalid workflow must not be persisted")
	}
}

func TestWorkflowCreateAndParseSteps(t *testing.T) {
	hub, _ := newWorkflowHub()
	s := hub.newSession("t")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	form := url.Values{
		"id": {"two"}, "name": {"Two"}, "mode": {"parallel"},
		"steps": {"builder: do A\nsdet: do B"},
	}
	resp, err := http.PostForm(srv.URL+"/workflows", form)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	s.hub.forgeMu.Lock()
	wf := s.forge.Workflow("two")
	s.hub.forgeMu.Unlock()
	if wf == nil {
		t.Fatal("workflow should be created")
	}
	if wf.Mode != ctxforge.WorkflowParallel || len(wf.Steps) != 2 {
		t.Fatalf("parsed workflow wrong: %+v", wf)
	}
	if wf.Steps[0].AgentID != "builder" || wf.Steps[0].Prompt != "do A" {
		t.Errorf("step 0 parsed wrong: %+v", wf.Steps[0])
	}
}
