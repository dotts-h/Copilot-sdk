package web

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
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
	if next := r.failLane(r.lanes[0], "boom"); next != nil {
		t.Fatalf("a failed step aborts a sequential run, launching nothing: %v", next)
	}
	if !r.done || !r.failed {
		t.Errorf("run should be done and failed: done=%v failed=%v", r.done, r.failed)
	}
	if r.lanes[1].status != lanePending {
		t.Error("the next sequential lane should never start after a failure")
	}
}

func TestRunParallelFailLetsOthersFinish(t *testing.T) {
	r := twoStepRun(ctxforge.WorkflowParallel)
	r.start()
	if next := r.failLane(r.lanes[0], "boom"); next != nil {
		t.Fatalf("a plain failed lane launches nothing while another runs: %v", next)
	}
	if r.done {
		t.Fatal("one failed lane should not end a parallel run while another runs")
	}
	r.finishLane(r.lanes[1], "done")
	if !r.done {
		t.Fatal("run ends once every lane settles")
	}
}

// --- branching (B2): predicate gating, skip, and acyclic launch ordering ---

func TestRunSequentialSkipsUnsatisfied(t *testing.T) {
	wf := ctxforge.Workflow{ID: "w", Name: "W", Mode: ctxforge.WorkflowSequential}
	steps := []ctxforge.CompiledStep{
		{AgentID: "a", AgentName: "Alpha", Prompt: "review"},
		{AgentID: "b", AgentName: "Beta", Prompt: "fix",
			When: &ctxforge.StepCondition{Step: 1, Condition: ctxforge.CondOutputContains, Value: "issues"}},
		{AgentID: "c", AgentName: "Gamma", Prompt: "celebrate",
			When: &ctxforge.StepCondition{Step: 1, Condition: ctxforge.CondOutputContains, Value: "perfect"}},
	}
	r := newWorkflowRun(wf, steps, []copilot.SessionSpec{{}, {}, {}})

	if got := r.start(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("start = %v, want [0]", got)
	}
	r.lanes[0].appendText("found issues to address")

	// Alpha settles → Beta (contains "issues") runs; Gamma is left pending.
	next := r.finishLane(r.lanes[0], "done")
	if len(next) != 1 || next[0] != 1 {
		t.Fatalf("finishLane(0) = %v, want [1] (Beta runs)", next)
	}
	if r.lanes[1].status != laneRunning {
		t.Fatalf("Beta should run when its predicate is satisfied: %v", r.lanes[1].status)
	}
	// Beta settles → walk to Gamma → its predicate ("perfect") is unsatisfied → skip
	// → run is done (a skipped final lane still terminates).
	r.lanes[1].appendText("fixed")
	if next := r.finishLane(r.lanes[1], "done"); next != nil {
		t.Fatalf("finishLane(1) launched %v, want nil (Gamma skips)", next)
	}
	if r.lanes[2].status != laneSkipped {
		t.Fatalf("Gamma should be skipped, got %v", r.lanes[2].status)
	}
	if !r.done || r.failed {
		t.Errorf("a run ending in a skip should be done and not failed: done=%v failed=%v", r.done, r.failed)
	}
	if !r.allSettled() {
		t.Error("a skipped lane must count as settled so the run terminates")
	}
}

func TestRunParallelGatedRunsAndSkips(t *testing.T) {
	wf := ctxforge.Workflow{ID: "w", Name: "W", Mode: ctxforge.WorkflowParallel}
	steps := []ctxforge.CompiledStep{
		{AgentID: "a", AgentName: "Alpha", Prompt: "review"}, // ungated
		{AgentID: "b", AgentName: "Beta", Prompt: "ship",
			When: &ctxforge.StepCondition{Step: 1, Condition: ctxforge.CondSucceeded}},
		{AgentID: "c", AgentName: "Gamma", Prompt: "rollback",
			When: &ctxforge.StepCondition{Step: 1, Condition: ctxforge.CondFailed}},
	}
	r := newWorkflowRun(wf, steps, []copilot.SessionSpec{{}, {}, {}})

	// start launches only the ungated lane; gated lanes wait for their dependency.
	if got := r.start(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("parallel start = %v, want [0] only (gated lanes wait)", got)
	}
	if r.lanes[1].status != lanePending || r.lanes[2].status != lanePending {
		t.Fatal("gated lanes must stay pending until their dependency settles")
	}
	// Alpha succeeds → Beta (when succeeded) runs; Gamma (when failed) skips.
	next := r.finishLane(r.lanes[0], "done")
	if len(next) != 1 || next[0] != 1 {
		t.Fatalf("Alpha success should launch Beta only: %v", next)
	}
	if r.lanes[2].status != laneSkipped {
		t.Fatalf("Gamma (when failed) should skip when Alpha succeeded: %v", r.lanes[2].status)
	}
	if r.done {
		t.Fatal("run must not be done while Beta runs")
	}
	if next := r.finishLane(r.lanes[1], "done"); next != nil {
		t.Fatalf("finishing Beta launches nothing: %v", next)
	}
	if !r.done {
		t.Fatal("run is done once Beta settles and Gamma is skipped")
	}
}

func TestRunParallelFailUnblocksWhenFailed(t *testing.T) {
	wf := ctxforge.Workflow{ID: "w", Name: "W", Mode: ctxforge.WorkflowParallel}
	steps := []ctxforge.CompiledStep{
		{AgentID: "a", AgentName: "Alpha", Prompt: "build"},
		{AgentID: "b", AgentName: "Beta", Prompt: "rollback",
			When: &ctxforge.StepCondition{Step: 1, Condition: ctxforge.CondFailed}},
	}
	r := newWorkflowRun(wf, steps, []copilot.SessionSpec{{}, {}})
	if got := r.start(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("start = %v, want [0]", got)
	}
	// Alpha fails → Beta (when failed) is unblocked and launched (parallel).
	next := r.failLane(r.lanes[0], "boom")
	if len(next) != 1 || next[0] != 1 {
		t.Fatalf("a failed dependency should launch the when-failed lane: %v", next)
	}
	if r.lanes[1].status != laneRunning {
		t.Fatalf("Beta should run after Alpha fails: %v", r.lanes[1].status)
	}
	if r.done {
		t.Fatal("run must not be done while Beta runs")
	}
	r.finishLane(r.lanes[1], "done")
	if !r.done {
		t.Fatal("run is done once Beta settles")
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

// startParallelRun installs a two-step parallel run on s with both lanes running
// and distinct backing session ids, so a test can drive two concurrent lanes via
// SessionID-tagged events (the parallel path the offline mock can now exercise).
func startParallelRun(s *Server) *workflowRun {
	run := twoStepRun(ctxforge.WorkflowParallel)
	s.mu.Lock()
	s.run = run
	s.busy = true
	run.start()
	run.lanes[0].sessionID = "s0"
	run.lanes[1].sessionID = "s1"
	s.mu.Unlock()
	return run
}

func TestWorkflowRunReducerParallelRoutesBySessionID(t *testing.T) {
	s, _ := newTestServer()
	run := startParallelRun(s)

	// Two concurrent lanes commit distinct output, each tagged by its session id;
	// attribution must follow the SessionID, not arrival order or a running-lane
	// guess (which is ambiguous with two lanes active).
	s.handleEvent(copilot.Event{Type: copilot.EvMessage, SessionID: "s1", Text: "beta out"})
	s.handleEvent(copilot.Event{Type: copilot.EvMessage, SessionID: "s0", Text: "alpha out"})
	if run.lanes[0].text != "alpha out" || run.lanes[1].text != "beta out" {
		t.Fatalf("message must attribute to the lane named by SessionID: %q / %q",
			run.lanes[0].text, run.lanes[1].text)
	}

	// Usage attributes only to the lane that incurred it.
	s.handleEvent(copilot.Event{Type: copilot.EvUsage, SessionID: "s0",
		Usage: copilot.UsageData{Model: "gpt-5", InputTokens: 1000, OutputTokens: 200}})
	if run.lanes[0].credits <= 0 || run.lanes[1].credits != 0 {
		t.Fatalf("usage must attribute to lane 0 only: %v / %v",
			run.lanes[0].credits, run.lanes[1].credits)
	}

	// Each lane idles independently; the run completes only when both settle.
	s.handleEvent(copilot.Event{Type: copilot.EvIdle, SessionID: "s0"})
	if run.done {
		t.Fatal("run must not finish until every parallel lane settles")
	}
	s.handleEvent(copilot.Event{Type: copilot.EvIdle, SessionID: "s1"})
	if !run.done {
		t.Fatal("run should finish once both parallel lanes idle")
	}
	if run.lanes[0].status != laneDone || run.lanes[1].status != laneDone {
		t.Fatalf("both lanes should be done: %v / %v", run.lanes[0].status, run.lanes[1].status)
	}
}

func TestWorkflowLaneSurfacesToolTimeline(t *testing.T) {
	s, _ := newTestServer()
	run := startParallelRun(s)

	s.handleEvent(copilot.Event{Type: copilot.EvToolStart, SessionID: "s0", Tool: "bash",
		ToolCall: &copilot.ToolCall{ID: "t0", Name: "bash", Args: "go test ./..."}})
	lanes := fragFor(s, copilot.Event{Type: copilot.EvToolEnd, SessionID: "s0",
		ToolCall: &copilot.ToolCall{ID: "t0", Result: "ok", Success: true}}, "lanes")
	if !strings.Contains(lanes, "bash") || !strings.Contains(lanes, "go test") {
		t.Fatalf("a lane should surface its own tool card in the lanes panel: %q", lanes)
	}
	// The tool attributes to lane 0 by SessionID, never to the other lane.
	if len(run.lanes[0].tools) != 1 || len(run.lanes[1].tools) != 0 {
		t.Fatalf("tool must attribute to lane 0: %d / %d",
			len(run.lanes[0].tools), len(run.lanes[1].tools))
	}
	if !run.lanes[0].tools[0].Done {
		t.Error("the tool should be marked done after EvToolEnd")
	}
}

func TestWorkflowLaneInlinePermission(t *testing.T) {
	s, _ := newTestServer()
	run := startParallelRun(s)

	lanes := fragFor(s, copilot.Event{Type: copilot.EvPermission, SessionID: "s1",
		Permission: &copilot.PermissionRequest{ID: "p1", Kind: "write", Detail: "write file: x.go"}}, "lanes")
	if !strings.Contains(lanes, "/perm/p1") {
		t.Fatalf("a lane should surface an inline permission form: %q", lanes)
	}
	if len(run.lanes[1].perms) != 1 || len(run.lanes[0].perms) != 0 {
		t.Fatalf("permission must attribute to lane 1: %d / %d",
			len(run.lanes[1].perms), len(run.lanes[0].perms))
	}

	// Answering via /perm/{id} responds on the seam, drops the form from the lane,
	// and refreshes #lanes out-of-band.
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp, err := http.PostForm(srv.URL+"/perm/p1", url.Values{"approve": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `id="lanes"`) {
		t.Errorf("answering a lane permission should refresh #lanes OOB: %q", string(body))
	}
	s.mu.Lock()
	remaining := len(run.lanes[1].perms)
	s.mu.Unlock()
	if remaining != 0 {
		t.Errorf("the answered permission should be dropped from the lane: %d", remaining)
	}
}

func TestWorkflowLaneToolTextEscaped(t *testing.T) {
	s, _ := newTestServer()
	startParallelRun(s)
	lanes := fragFor(s, copilot.Event{Type: copilot.EvToolStart, SessionID: "s0", Tool: "bash",
		ToolCall: &copilot.ToolCall{ID: "t0", Name: "bash", Args: "<script>x</script>"}}, "lanes")
	if strings.Contains(lanes, "<script>") {
		t.Fatalf("lane tool args must be HTML-escaped (ADR-0001): %q", lanes)
	}
}

func TestStreamDemoLaneTagsSessionID(t *testing.T) {
	m := copilot.NewMockClient()
	streamDemoLane(m, "sess-x", "do the thing", nil)
	m.Close()
	var sawTagged, sawIdle, sawTool bool
	for e := range m.Events() {
		if e.SessionID != "sess-x" {
			t.Fatalf("every demo lane event must carry the lane's session id; got %q on %v", e.SessionID, e.Type)
		}
		sawTagged = true
		switch e.Type {
		case copilot.EvIdle:
			sawIdle = true
		case copilot.EvToolStart:
			sawTool = true
		}
	}
	if !sawTagged || !sawIdle || !sawTool {
		t.Fatalf("demo lane should emit tagged tool+idle events: tagged=%v tool=%v idle=%v", sawTagged, sawTool, sawIdle)
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
	html := s.renderPage("workflows", "")
	if !strings.Contains(html, "Ship") || !strings.Contains(html, "builder") {
		t.Errorf("workflows page should list the workflow: %q", html)
	}
	if !strings.Contains(html, "/workflows/ship/run") {
		t.Errorf("workflows page should offer a run control: %q", html)
	}
}

// withSpendStore attaches an ephemeral spend ledger so a workflow row can badge its
// per-workflow spend.
func withSpendStore(s *Server) *telemetry.SpendStore {
	ss, _ := telemetry.LoadSpendStore("")
	s.spend = ss
	return ss
}

func TestWorkflowsPageBadgesRunAndSpend(t *testing.T) {
	// When both stores carry the workflow, its row gains the last-run glyph + age, a
	// run-count badge, and a total-spend badge (V4). Assert STRUCTURE (the badge
	// classes) and that the glyph matches the LATEST run's outcome — never figures.
	hub, _ := newWorkflowHub()
	s := hub.newSession("t")
	rs := withRunStore(s)
	ss := withSpendStore(s)
	base := time.Now().Add(-2 * time.Hour)
	// Two runs of "ship"; the most recent FAILED, an earlier one finished — so the
	// last-run badge must show the failure glyph, not the finished one.
	_ = rs.Append(telemetry.RunRecord{
		ID: "r1", WorkflowID: "ship", Name: "Ship", Mode: "sequential",
		StartedAt: base, FinishedAt: base.Add(time.Minute), Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done", Credits: 2}},
	})
	_ = rs.Append(telemetry.RunRecord{
		ID: "r2", WorkflowID: "ship", Name: "Ship", Mode: "sequential",
		StartedAt: base.Add(time.Hour), FinishedAt: base.Add(time.Hour).Add(time.Minute), Outcome: "failed",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "failed", Credits: 1}},
	})
	_ = ss.Append(telemetry.SpendRecord{At: base, Model: "gpt-5", USD: 0.05, WorkflowID: "ship"})

	html := s.workflowsPartial()
	for _, sub := range []string{`class="wf-badges"`, "wf-lastrun", "wf-runs", "wf-spend", "2 runs"} {
		if !strings.Contains(html, sub) {
			t.Errorf("workflows badges missing %q\n%s", sub, html)
		}
	}
	// The glyph matches the LATEST run's outcome: failed (✗ / run-failed), not finished.
	if !strings.Contains(html, "wf-lastrun run-failed") || !strings.Contains(html, "✗") {
		t.Errorf("last-run badge should reflect the failed latest run:\n%s", html)
	}
	if strings.Contains(html, "wf-lastrun run-done") {
		t.Errorf("the earlier finished run must not win the last-run glyph:\n%s", html)
	}
}

func TestWorkflowsPageNoStoresNoBadges(t *testing.T) {
	// With NO run store and NO spend store wired, the row renders today's navigational
	// shape — no panic, no badges (the all-absent guard).
	hub, _ := newWorkflowHub()
	s := hub.newSession("t") // s.runs == nil, s.spend == nil
	html := s.workflowsPartial()
	if strings.Contains(html, "wf-badge") {
		t.Errorf("with no run/spend stores the row carries no badges:\n%s", html)
	}
	if !strings.Contains(html, "Ship") || !strings.Contains(html, "/workflows/ship/run") {
		t.Errorf("the row should still render its prior shape:\n%s", html)
	}
}

func TestWorkflowsPagePartialAndOrphanStores(t *testing.T) {
	// Run store only (no spend): run badges render, no spend badge.
	hub, _ := newWorkflowHub()
	s := hub.newSession("t")
	rs := withRunStore(s) // s.spend stays nil
	_ = rs.Append(telemetry.RunRecord{
		ID: "r", WorkflowID: "ship", Name: "Ship", Outcome: "finished", StartedAt: time.Now(),
		Lanes: []telemetry.RunLane{{Index: 0, Status: "done", Credits: 1}},
	})
	html := s.workflowsPartial()
	if !strings.Contains(html, "wf-runs") {
		t.Errorf("run badges should render with only a run store:\n%s", html)
	}
	if strings.Contains(html, "wf-spend") {
		t.Errorf("no spend store → no spend badge:\n%s", html)
	}

	// Spend store only (no runs): the spend badge renders, no run badges.
	hub2, _ := newWorkflowHub()
	s2 := hub2.newSession("t")
	ss := withSpendStore(s2) // s2.runs stays nil
	_ = ss.Append(telemetry.SpendRecord{At: time.Now(), Model: "gpt-5", USD: 0.03, WorkflowID: "ship"})
	html2 := s2.workflowsPartial()
	if !strings.Contains(html2, "wf-spend") {
		t.Errorf("spend badge should render with only a spend store:\n%s", html2)
	}
	if strings.Contains(html2, "wf-lastrun") || strings.Contains(html2, "wf-runs") {
		t.Errorf("no run store → no run badges:\n%s", html2)
	}

	// A since-deleted/renamed workflow id present only in the stores must not panic or
	// fabricate a row; the real workflow (no records) still renders, unbadged.
	hub3, _ := newWorkflowHub()
	s3 := hub3.newSession("t")
	rs3 := withRunStore(s3)
	ss3 := withSpendStore(s3)
	_ = rs3.Append(telemetry.RunRecord{
		ID: "g", WorkflowID: "ghost", Name: "Ghost", Outcome: "finished", StartedAt: time.Now(),
		Lanes: []telemetry.RunLane{{Index: 0, Status: "done", Credits: 1}},
	})
	_ = ss3.Append(telemetry.SpendRecord{At: time.Now(), Model: "gpt-5", USD: 0.02, WorkflowID: "ghost"})
	html3 := s3.workflowsPartial()
	if strings.Contains(html3, "wf-badge") {
		t.Errorf("an orphan store id should badge no row:\n%s", html3)
	}
	if !strings.Contains(html3, "Ship") {
		t.Errorf("the real workflow still renders:\n%s", html3)
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

// TestRunRerunHandlerStartsRecordedWorkflow proves the Runs-page rerun (ADR-0023)
// re-executes a recorded run's workflow through the SAME launchWorkflow trigger as the
// Workflows-page run: a POST to /runs/rerun/{workflow} compiles the workflow by id, opens
// a backing session, sends its first lane's prompt, and lands the user on the chat page.
// The new run carries the same workflow id (run.id == "ship"), so its spend rolls up under
// the same per-workflow totals — a rerun is a re-execution, not a separate workflow.
func TestRunRerunHandlerStartsRecordedWorkflow(t *testing.T) {
	hub, mock := newWorkflowHub()
	s := hub.newSession("t")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/runs/rerun/ship?window=30", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// On success the rerun lands the user on the chat page with the lanes panel showing.
	if !strings.Contains(string(body), "workflow-run") || !strings.Contains(string(body), "Builder") {
		t.Errorf("rerun response should render the lanes panel: %q", string(body))
	}
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
	defer s.mu.Unlock()
	if s.run == nil || !s.busy {
		t.Fatal("a run should be active after rerunning the workflow")
	}
	if s.run.id != "ship" {
		t.Errorf("rerun must launch under the recorded WorkflowID (rolls up together), got %q", s.run.id)
	}
}

// TestRunRerunUnknownWorkflowNoLaunch proves the rerun fails safe when the workflow no
// longer exists (an orphan run raced a delete, ADR-0023): no run starts, no state changes,
// and the Runs page is re-rendered unchanged.
func TestRunRerunUnknownWorkflowNoLaunch(t *testing.T) {
	hub, mock := newWorkflowHub()
	s := hub.newSession("t")
	withRunStore(s)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/runs/rerun/gone?window=14", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Workflow Runs") {
		t.Errorf("an unknown-workflow rerun should re-render the Runs page: %q", string(body))
	}
	if mock.SentCount() != 0 {
		t.Errorf("no lane should run for an unknown workflow, sent %d", mock.SentCount())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run != nil || s.busy {
		t.Error("no run should be active after rerunning an unknown workflow")
	}
}

// TestParallelDemoRunDrivesConcurrentLanes is the end-to-end proof B1 exists for:
// in demo mode the mock hands out distinct session ids and streamDemoLane tags its
// events with them, so a PARALLEL run drives two concurrent lanes through the real
// handler → pump → reducer path — each settling to done with its own tool timeline
// and a distinct backing session id. This is the offline coverage the sequential-
// only demo could not give (issue 0015 / TECH_DEBT #12).
func TestParallelDemoRunDrivesConcurrentLanes(t *testing.T) {
	mock := copilot.NewMockClient()
	forge := &ctxforge.Forge{}
	_ = forge.AddAgent(ctxforge.Agent{ID: "builder", Name: "Builder", Model: "gpt-5"})
	_ = forge.AddAgent(ctxforge.Agent{ID: "sdet", Name: "SDET", Model: "gpt-5"})
	_ = forge.AddWorkflow(ctxforge.Workflow{
		ID: "par", Name: "Par", Mode: ctxforge.WorkflowParallel,
		Steps: []ctxforge.WorkflowStep{
			{AgentID: "builder", Prompt: "review for correctness"},
			{AgentID: "sdet", Prompt: "review for coverage"},
		},
	})
	hub := New(Options{
		Client: mock, Forge: forge, Demo: true,
		Config: &config.Config{DefaultModel: "gpt-5"},
		Meter:  telemetry.NewMeter(telemetry.DefaultPriceBook()),
		Logger: log.New(io.Discard, "", 0),
	})
	s := hub.newSession("t")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/workflows/par/run", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		done := s.run != nil && s.run.done
		s.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run == nil || !s.run.done {
		t.Fatal("the parallel demo run should finish once both lanes settle")
	}
	for i, l := range s.run.lanes {
		if l.status != laneDone {
			t.Errorf("lane %d should be done, got %v", i, l.status)
		}
		if len(l.tools) == 0 {
			t.Errorf("lane %d should surface its own tool card", i)
		}
		if len(l.perms) == 0 {
			t.Errorf("lane %d should surface its inline permission", i)
		}
		if l.credits <= 0 {
			t.Errorf("lane %d should accrue its own metered cost", i)
		}
		if l.sessionID == "" {
			t.Errorf("lane %d should have a backing session id", i)
		}
	}
	if s.run.lanes[0].sessionID == s.run.lanes[1].sessionID {
		t.Fatalf("parallel lanes must have distinct session ids, both = %q", s.run.lanes[0].sessionID)
	}
	if s.busy {
		t.Error("busy should clear when the parallel run finishes")
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

// --- branching (B2): the demo run skips a lane, and the form round-trips a When ---

// TestBranchingDemoRunSkipsLane drives a seeded branching workflow through the real
// handler → pump → reducer path in demo mode: step 1's output gates step 2 (which
// RUNS, since the demo output contains "issues") and step 3 (which SKIPS, since the
// output lacks "perfect"), and the skipped lane lets the run terminate. This is the
// offline proof a branch evaluates without timing (issue 0020 / ADR-0021).
func TestBranchingDemoRunSkipsLane(t *testing.T) {
	mock := copilot.NewMockClient()
	forge := &ctxforge.Forge{}
	_ = forge.AddAgent(ctxforge.Agent{ID: "builder", Name: "Builder", Model: "gpt-5"})
	_ = forge.AddAgent(ctxforge.Agent{ID: "sdet", Name: "SDET", Model: "gpt-5"})
	_ = forge.AddWorkflow(ctxforge.Workflow{
		ID: "br", Name: "Br", Mode: ctxforge.WorkflowSequential,
		Steps: []ctxforge.WorkflowStep{
			{AgentID: "sdet", Prompt: "Review and flag any issues."},
			{AgentID: "builder", Prompt: "Apply fixes for the flagged issues.",
				When: &ctxforge.StepCondition{Step: 1, Condition: ctxforge.CondOutputContains, Value: "issues"}},
			{AgentID: "builder", Prompt: "Nothing to do.",
				When: &ctxforge.StepCondition{Step: 1, Condition: ctxforge.CondOutputContains, Value: "perfect"}},
		},
	})
	hub := New(Options{
		Client: mock, Forge: forge, Demo: true,
		Config: &config.Config{DefaultModel: "gpt-5"},
		Meter:  telemetry.NewMeter(telemetry.DefaultPriceBook()),
		Logger: log.New(io.Discard, "", 0),
	})
	s := hub.newSession("t")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/workflows/br/run", "application/x-www-form-urlencoded", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		done := s.run != nil && s.run.done
		s.mu.Unlock()
		if done {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run == nil || !s.run.done {
		t.Fatal("the branching demo run should finish")
	}
	if s.run.lanes[0].status != laneDone {
		t.Errorf("step 1 should run: %v", s.run.lanes[0].status)
	}
	if s.run.lanes[1].status != laneDone {
		t.Errorf("step 2 (output contains issues) should run: %v", s.run.lanes[1].status)
	}
	if s.run.lanes[2].status != laneSkipped {
		t.Errorf("step 3 (output contains perfect) should skip: %v", s.run.lanes[2].status)
	}
	html := renderLanes(s.run)
	if !strings.Contains(html, "lane-skipped") || !strings.Contains(html, "skipped") {
		t.Errorf("a skipped lane should render its skipped state: %q", html)
	}
	if s.busy {
		t.Error("busy should clear when the branching run finishes")
	}
}

// the steps textarea round-trips a predicate as a "[step N condition value]" prefix,
// so editing a branching workflow in the form doesn't lose its When.
func TestWorkflowStepConditionRoundTrip(t *testing.T) {
	steps := []ctxforge.WorkflowStep{
		{AgentID: "a", Prompt: "review"},
		{AgentID: "b", Prompt: "fix", When: &ctxforge.StepCondition{
			Step: 1, Condition: ctxforge.CondOutputContains, Value: "issues here"}},
		{AgentID: "c", Prompt: "done", When: &ctxforge.StepCondition{
			Step: 2, Condition: ctxforge.CondSucceeded}},
		// An output-contains value with a colon must survive: the separator colon is
		// the one after the predicate's "]", not the first colon on the line.
		{AgentID: "d", Prompt: "ship", When: &ctxforge.StepCondition{
			Step: 1, Condition: ctxforge.CondOutputContains, Value: "error: fatal"}},
	}
	got := stepsFromText(stepsToText(steps))
	if !reflect.DeepEqual(got, steps) {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", got, steps)
	}
}

// the create form parses a bracketed predicate into a WorkflowStep.When, and the
// edit form renders it back so a save doesn't drop it.
func TestWorkflowFormRoundTripsCondition(t *testing.T) {
	hub, _ := newWorkflowHub()
	s := hub.newSession("t")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	form := url.Values{
		"id": {"br"}, "name": {"Br"}, "mode": {"sequential"},
		"steps": {"builder: review\nsdet [step 1 output-contains issues]: harden"},
	}
	resp, err := http.PostForm(srv.URL+"/workflows", form)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	s.hub.forgeMu.Lock()
	wf := s.forge.Workflow("br")
	s.hub.forgeMu.Unlock()
	if wf == nil || len(wf.Steps) != 2 {
		t.Fatalf("workflow not created: %+v", wf)
	}
	w := wf.Steps[1].When
	if w == nil || w.Step != 1 || w.Condition != ctxforge.CondOutputContains || w.Value != "issues" {
		t.Fatalf("predicate not parsed from form: %+v", w)
	}
	edit := renderWorkflowForm(*wf, false, "")
	if !strings.Contains(edit, "[step 1 output-contains issues]") {
		t.Errorf("edit form should render the predicate prefix so it isn't lost: %q", edit)
	}
}

// --- worker pool / backpressure (issue 0084) ---

// blockingClient is a counting stub that controls when each CreateSession call
// returns. It lets the test measure peak concurrency against the worker cap.
type blockingClient struct {
	copilot.Client // embed for methods we don't need to override

	mu       sync.Mutex
	inflight int32         // atomic; tracks sessions currently in CreateSession
	peak     int32         // atomic; highest observed inflight
	unblock  chan struct{} // close to let ALL blocked CreateSession calls return
	sessions int
	sent     []string
}

func newBlockingClient() *blockingClient {
	return &blockingClient{
		Client:  copilot.NewMockClient(),
		unblock: make(chan struct{}),
	}
}

func (c *blockingClient) CreateSession(_ context.Context, _ copilot.SessionSpec) (string, error) {
	cur := atomic.AddInt32(&c.inflight, 1)
	for {
		p := atomic.LoadInt32(&c.peak)
		if cur <= p || atomic.CompareAndSwapInt32(&c.peak, p, cur) {
			break
		}
	}
	// Block until the test signals "go".
	<-c.unblock
	atomic.AddInt32(&c.inflight, -1)
	c.mu.Lock()
	c.sessions++
	id := fmt.Sprintf("blk-session-%d", c.sessions)
	c.mu.Unlock()
	return id, nil
}

func (c *blockingClient) Send(_ context.Context, _ string, prompt string, _ []string, _ string) error {
	c.mu.Lock()
	c.sent = append(c.sent, prompt)
	c.mu.Unlock()
	return nil
}

func (c *blockingClient) SentCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sent)
}

// Events/Close are forwarded to the embedded mock so the hub pump can drain events.
func (c *blockingClient) Events() <-chan copilot.Event { return c.Client.Events() }
func (c *blockingClient) Close() error                 { return c.Client.Close() }

// TestLaunchLanesWorkerPoolCapsConcurrency drives a parallel workflow whose lane
// count (laneWorkerCount+3) exceeds the fixed worker cap and asserts:
//
//  1. concurrent in-flight CreateSession calls never exceed laneWorkerCount, and
//  2. every lane still completes (all prompts are sent after unblocking).
//
// The pure run engine is not touched — this exercises only the launchLanes adapter.
func TestLaunchLanesWorkerPoolCapsConcurrency(t *testing.T) {
	const cap = laneWorkerCount
	const total = cap + 3 // intentionally more than the cap

	bc := newBlockingClient()

	forge := &ctxforge.Forge{}
	_ = forge.AddAgent(ctxforge.Agent{ID: "ag", Name: "Agent", Model: "gpt-5"})
	steps := make([]ctxforge.WorkflowStep, total)
	for i := range steps {
		steps[i] = ctxforge.WorkflowStep{AgentID: "ag", Prompt: fmt.Sprintf("lane-%d", i)}
	}
	_ = forge.AddWorkflow(ctxforge.Workflow{
		ID: "wide", Name: "Wide", Mode: ctxforge.WorkflowParallel, Steps: steps,
	})
	hub := New(Options{
		Client: bc, Forge: forge,
		Config: &config.Config{DefaultModel: "gpt-5"},
		Meter:  telemetry.NewMeter(telemetry.DefaultPriceBook()),
		Logger: log.New(io.Discard, "", 0),
	})
	s := hub.newSession("t")

	// Build and launch the run via launchLanes directly so we avoid the HTTP layer.
	wf, compiled, err := forge.CompileWorkflow("wide")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	specs := make([]copilot.SessionSpec, len(compiled))
	run := newWorkflowRun(wf, compiled, specs)
	idxs := run.start() // returns all lane indices (parallel mode)

	// Start launchLanes (it returns as soon as the worker goroutines are spinning).
	s.launchLanes(run, idxs)

	// Wait until cap workers are blocked inside CreateSession — this proves that
	// exactly cap goroutines entered the seam simultaneously and the rest are queued.
	// We require inflight to actually reach cap; a silent timeout would let the
	// assertion pass vacuously (peak==0 satisfies peak≤cap even with a broken pool).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&bc.inflight) >= int32(cap) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if atomic.LoadInt32(&bc.inflight) < int32(cap) {
		t.Fatalf("timed out waiting for %d workers to block in CreateSession (inflight=%d); pool may not be starting workers",
			cap, atomic.LoadInt32(&bc.inflight))
	}

	// Assert concurrency at the cap: no more than laneWorkerCount concurrent sessions.
	peak := atomic.LoadInt32(&bc.peak)
	if peak > int32(cap) {
		t.Errorf("peak concurrent in-flight sessions = %d, want ≤ %d (the worker cap)", peak, cap)
	}

	// Unblock all workers and wait for every lane to send its prompt.
	close(bc.unblock)
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if bc.SentCount() >= total {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// All lanes must complete.
	if got := bc.SentCount(); got != total {
		t.Errorf("want %d prompts sent (all lanes), got %d", total, got)
	}

	// Re-check peak after all workers have passed through.
	peak = atomic.LoadInt32(&bc.peak)
	if peak > int32(cap) {
		t.Errorf("final peak concurrent in-flight sessions = %d, want ≤ %d", peak, cap)
	}
}
