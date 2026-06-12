package web

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// newTestServerWithRunsAndLog builds a session wired with both a persisted run
// history and the per-run event log enabled (EventLogDir = dir) — the inspector's
// two read dependencies.
func newTestServerWithRunsAndLog(t *testing.T, dir string) *Server {
	t.Helper()
	runs, _ := telemetry.LoadRunStore("")
	hub := New(Options{
		Client:      copilot.NewMockClient(),
		Forge:       &ctxforge.Forge{},
		Config:      &config.Config{DefaultModel: "gpt-5"},
		Meter:       telemetry.NewMeter(telemetry.DefaultPriceBook()),
		Logger:      log.New(io.Discard, "", 0),
		Runs:        runs,
		EventLogDir: dir,
	})
	return hub.newSession("test")
}

// ev is a terse RunEvent constructor for the timeline-reconstruction tests.
func ev(typ string, lane int) telemetry.RunEvent {
	return telemetry.RunEvent{Type: typ, LaneIndex: lane, At: time.Now()}
}

// TestBuildRunTimeline_JoinsToolsCoalescesDeltasGroupsByLane is the core
// reconstruction test: tool start+end join into one step, streaming deltas and
// EvToolProgress are coalesced away, and steps group by lane. — ADR-0052.
func TestBuildRunTimeline_JoinsToolsCoalescesDeltasGroupsByLane(t *testing.T) {
	events := []telemetry.RunEvent{
		ev("EvUserMessage", 0),
		ev("EvReasoningDelta", 0), // coalesced away
		ev("EvReasoning", 0),
		ev("EvMessageDelta", 0), // coalesced away
		{Type: "EvToolStart", LaneIndex: 0, Tool: "bash", Args: "echo ok", At: time.Now()},
		ev("EvToolProgress", 0), // coalesced away
		{Type: "EvToolEnd", LaneIndex: 0, Tool: "bash", Result: "ok", Success: true, At: time.Now()},
		ev("EvMessage", 0),
		// A second lane.
		{Type: "EvToolStart", LaneIndex: 1, Tool: "read", Args: "file.go", At: time.Now()},
		{Type: "EvToolEnd", LaneIndex: 1, Tool: "read", Result: "boom", Success: false, At: time.Now()},
	}

	lanes := buildRunTimeline(events)
	if len(lanes) != 2 {
		t.Fatalf("want 2 lane groups, got %d: %+v", len(lanes), lanes)
	}

	// Lane 0: user, reasoning, tool(bash joined), message — 4 steps, no deltas.
	l0 := lanes[0]
	if l0.LaneIndex != 0 {
		t.Fatalf("first group should be lane 0, got %d", l0.LaneIndex)
	}
	if len(l0.Steps) != 4 {
		t.Fatalf("lane 0 want 4 steps, got %d: %+v", len(l0.Steps), l0.Steps)
	}
	for _, s := range l0.Steps {
		if strings.Contains(s.Type, "delta") || s.Type == "progress" {
			t.Fatalf("a delta/progress step leaked into the timeline: %+v", s)
		}
	}
	tool := l0.Steps[2]
	if tool.Type != "tool" || tool.Label != "bash" {
		t.Fatalf("step 2 should be the joined bash tool, got %+v", tool)
	}
	if tool.Args != "echo ok" || tool.Result != "ok" || tool.State != "done" {
		t.Fatalf("joined tool missing args/result/success: %+v", tool)
	}

	// Lane 1: a single failed tool step.
	l1 := lanes[1]
	if l1.LaneIndex != 1 || len(l1.Steps) != 1 {
		t.Fatalf("lane 1 want 1 step, got %+v", l1)
	}
	if l1.Steps[0].State != "failed" || l1.Steps[0].Result != "boom" {
		t.Fatalf("lane 1 tool should be failed with result, got %+v", l1.Steps[0])
	}
}

// TestBuildRunTimeline_UnmatchedToolStartRenders proves a tool that never
// returned (crashed/in-flight run) still renders as a tool step with no result,
// and an unattributed (-1) event groups as run-level, ordered last.
func TestBuildRunTimeline_UnmatchedToolStartRenders(t *testing.T) {
	events := []telemetry.RunEvent{
		{Type: "EvToolStart", LaneIndex: 0, Tool: "bash", Args: "sleep 99", At: time.Now()},
		ev("EvError", -1), // unattributed → run-level group, ordered last
	}
	lanes := buildRunTimeline(events)
	if len(lanes) != 2 {
		t.Fatalf("want 2 groups (lane 0 + run-level), got %+v", lanes)
	}
	if lanes[0].LaneIndex != 0 || lanes[1].LaneIndex != -1 {
		t.Fatalf("run-level (-1) group must sort last, got %d then %d", lanes[0].LaneIndex, lanes[1].LaneIndex)
	}
	open := lanes[0].Steps[0]
	if open.Type != "tool" || open.State != "running" || open.Result != "" {
		t.Fatalf("unmatched tool start should render running with no result, got %+v", open)
	}
}

// TestBuildRunTimeline_MismatchedEndRendersStandalone proves an EvToolEnd whose
// tool name matches no open start in its lane renders on its own rather than
// hijacking an unrelated open tool's result/success (ADR-0052 join is name-scoped).
func TestBuildRunTimeline_MismatchedEndRendersStandalone(t *testing.T) {
	events := []telemetry.RunEvent{
		{Type: "EvToolStart", LaneIndex: 0, Tool: "bash", Args: "sleep 99", At: time.Now()},
		{Type: "EvToolEnd", LaneIndex: 0, Tool: "read", Result: "file.go", Success: true, At: time.Now()},
	}
	lanes := buildRunTimeline(events)
	if len(lanes) != 1 || len(lanes[0].Steps) != 2 {
		t.Fatalf("want 1 lane with 2 steps (open bash + standalone read end), got %+v", lanes)
	}
	bash := lanes[0].Steps[0]
	if bash.Label != "bash" || bash.State != "running" || bash.Result != "" {
		t.Fatalf("bash must stay open/running with no result, got %+v", bash)
	}
	read := lanes[0].Steps[1]
	if read.Label != "read" || read.Result != "file.go" || read.State != "done" {
		t.Fatalf("read end must render standalone with its own result, got %+v", read)
	}
}

// TestBuildRunTimeline_RollsUpLaneCreditsFromUsage proves EvUsage events stay
// coalesced away (no visible step) yet their credits roll into the owning lane's
// subtotal — the O2 per-step pricing grain (issue 0092).
func TestBuildRunTimeline_RollsUpLaneCreditsFromUsage(t *testing.T) {
	events := []telemetry.RunEvent{
		{Type: "EvUserMessage", LaneIndex: 0, At: time.Now()},
		{Type: "EvUsage", LaneIndex: 0, TokensIn: 1000, TokensOut: 200, Credits: 0.30, At: time.Now()},
		{Type: "EvUsage", LaneIndex: 0, TokensIn: 500, TokensOut: 100, Credits: 0.20, At: time.Now()},
		{Type: "EvUsage", LaneIndex: 1, TokensIn: 800, TokensOut: 150, Credits: 0.50, At: time.Now()},
	}
	lanes := buildRunTimeline(events)
	if len(lanes) != 2 {
		t.Fatalf("want 2 lane groups, got %d: %+v", len(lanes), lanes)
	}
	// Lane 0: one visible step (the user message) — the two EvUsage events are
	// coalesced away — and a 0.50 cr subtotal.
	if len(lanes[0].Steps) != 1 {
		t.Fatalf("lane 0 want 1 visible step (usage coalesced away), got %+v", lanes[0].Steps)
	}
	if lanes[0].Credits != 0.50 {
		t.Fatalf("lane 0 subtotal = %v, want 0.50", lanes[0].Credits)
	}
	// Lane 1: usage-only — no visible steps, a 0.50 cr subtotal still rolls up.
	if len(lanes[1].Steps) != 0 {
		t.Fatalf("lane 1 should have no visible steps, got %+v", lanes[1].Steps)
	}
	if lanes[1].Credits != 0.50 {
		t.Fatalf("lane 1 subtotal = %v, want 0.50", lanes[1].Credits)
	}
}

// TestRunDetailPartial_CrossChecksLogCreditsAgainstRecord proves the detail header
// shows the summed event-log credits beside RunRecord.Credits and ambers only on a
// non-trivial mismatch — the per-run mini-reconciliation (issue 0092).
func TestRunDetailPartial_CrossChecksLogCreditsAgainstRecord(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	// RunRecord credits = 0.50 (lane 0). The event log will sum to a mismatching 0.80.
	rec := telemetry.RunRecord{ID: "run-xcheck", Name: "Xcheck", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done", Credits: 0.50}}}
	_ = s.runs.Append(rec)
	log, _ := telemetry.LoadRunEventLog(dir, "run-xcheck")
	_ = log.Append(telemetry.RunEvent{Type: "EvUserMessage", LaneIndex: 0})
	_ = log.Append(telemetry.RunEvent{Type: "EvUsage", LaneIndex: 0, TokensIn: 1000, TokensOut: 200, Credits: 0.80})
	waitForEventLog(t, dir, "run-xcheck", 2)

	html := s.runDetailPartial(rec, defaultSpendWindow, viewTimeline)
	if !strings.Contains(html, "0.80 cr") {
		t.Fatalf("header should show the summed event-log credits (0.80 cr):\n%s", html)
	}
	if !strings.Contains(html, "amber") {
		t.Fatalf("a non-trivial log-vs-record mismatch should amber:\n%s", html)
	}

	// A matching pair: log sums to 0.50, equal to the record — no amber.
	recMatch := telemetry.RunRecord{ID: "run-match", Name: "Match", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done", Credits: 0.50}}}
	_ = s.runs.Append(recMatch)
	logM, _ := telemetry.LoadRunEventLog(dir, "run-match")
	_ = logM.Append(telemetry.RunEvent{Type: "EvUsage", LaneIndex: 0, TokensIn: 1000, TokensOut: 200, Credits: 0.50})
	waitForEventLog(t, dir, "run-match", 1)

	htmlM := s.runDetailPartial(recMatch, defaultSpendWindow, viewTimeline)
	if strings.Contains(htmlM, "amber") {
		t.Fatalf("a matching log-vs-record pair must not amber:\n%s", htmlM)
	}
}

// TestRunDetailPartial_PreO2LogRendersUnpriced proves a pre-O2 log (no priced
// usage events) renders the timeline without cost-column noise — the header shows
// the record's own credits but no zero-valued event-log figure (issue 0092).
func TestRunDetailPartial_PreO2LogRendersUnpriced(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	rec := telemetry.RunRecord{ID: "run-preo2", Name: "PreO2", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done", Credits: 0.50}}}
	_ = s.runs.Append(rec)
	log, _ := telemetry.LoadRunEventLog(dir, "run-preo2")
	_ = log.Append(telemetry.RunEvent{Type: "EvMessage", LaneIndex: 0, Text: "hello"})
	waitForEventLog(t, dir, "run-preo2", 1)

	html := s.runDetailPartial(rec, defaultSpendWindow, viewTimeline)
	if !strings.Contains(html, "hello") {
		t.Fatalf("the timeline should render the message step:\n%s", html)
	}
	// No event-log cost figure and no amber when the log carries no pricing.
	if strings.Contains(html, "log:") || strings.Contains(html, "amber") {
		t.Fatalf("a pre-O2 log must render unpriced (no log-credit figure / amber):\n%s", html)
	}
}

// TestRunRow_NoLinkWhenIDEmpty proves a legacy run record with an empty ID renders
// its name as plain text, not a link that would 404 on click (ADR-0052).
func TestRunRow_NoLinkWhenIDEmpty(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	_ = s.runs.Append(telemetry.RunRecord{ID: "", Name: "Legacy", Mode: "sequential", Outcome: "finished"})

	html := s.runsPartial(defaultSpendWindow)
	if strings.Contains(html, "/page/runs/?") {
		t.Fatalf("empty-id run must not render a detail link:\n%s", html)
	}
	if !strings.Contains(html, "Legacy") {
		t.Fatal("the legacy run row should still render its name")
	}
}

// TestRunDetailPartial_CorruptLogNote proves a present-but-unreadable event log
// surfaces a distinct "could not be read" note, not the misleading "ran before
// logging" note (ADR-0052 graceful degradation).
func TestRunDetailPartial_CorruptLogNote(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	rec := telemetry.RunRecord{ID: "run-corrupt", Name: "Corrupt", Mode: "sequential", Outcome: "finished"}
	_ = s.runs.Append(rec)
	// Write a malformed log file at the canonical path so LoadRunEventLog errors.
	path := telemetry.RunEventLogPath(dir, "run-corrupt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	html := s.runDetailPartial(rec, defaultSpendWindow, viewTimeline)
	if !strings.Contains(html, "could not be read") {
		t.Fatalf("corrupt log should surface a distinct note:\n%s", html)
	}
	if strings.Contains(html, "ran before the per-run event log was enabled") {
		t.Fatal("corrupt log must not show the 'no log' note")
	}
}

// TestRunDetailPartial_EscapesHostileText proves a hostile tool result cannot
// inject markup — the rendered page escapes it (ADR-0001).
func TestRunDetailPartial_EscapesHostileText(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	rec := telemetry.RunRecord{ID: "run-x", Name: "Hostile", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done"}}}
	_ = s.runs.Append(rec)
	log, _ := telemetry.LoadRunEventLog(dir, "run-x")
	_ = log.Append(telemetry.RunEvent{Type: "EvToolEnd", LaneIndex: 0, Tool: "bash",
		Result: "<script>alert(1)</script>", Success: true})
	waitForEventLog(t, dir, "run-x", 1)

	html := s.runDetailPartial(rec, defaultSpendWindow, viewTimeline)
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatal("hostile tool result was not escaped")
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Fatalf("expected escaped script entity in output:\n%s", html)
	}
}

// TestRunDetailPartial_MissingLogRendersNote proves a run with no event log
// renders the summary + a note, never an error (ADR-0052 graceful degradation).
func TestRunDetailPartial_MissingLogRendersNote(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	rec := telemetry.RunRecord{ID: "run-nolog", Name: "No Log", Mode: "sequential", Outcome: "finished"}
	_ = s.runs.Append(rec)

	html := s.runDetailPartial(rec, defaultSpendWindow, viewTimeline)
	if !strings.Contains(html, "No Log") {
		t.Fatal("summary card (run name) missing")
	}
	if !strings.Contains(html, "no event log") {
		t.Fatalf("expected the no-event-log note:\n%s", html)
	}
}

// TestHandleRunDetail_UnknownID404 proves an unknown run id 404s cleanly.
func TestHandleRunDetail_UnknownID404(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	req := httptest.NewRequest(http.MethodGet, "/page/runs/nope", nil)
	req.SetPathValue("id", "nope")
	rr := httptest.NewRecorder()
	s.handleRunDetail(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("unknown id should 404, got %d", rr.Code)
	}
}

// TestHandleRunDetail_RendersTimelineThroughMux drives the real mux end-to-end:
// a seeded run with a log renders its joined tool step.
func TestHandleRunDetail_RendersTimelineThroughMux(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	rec := telemetry.RunRecord{ID: "run-mux", Name: "Mux Run", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done"}}}
	_ = s.runs.Append(rec)
	log, _ := telemetry.LoadRunEventLog(dir, "run-mux")
	_ = log.Append(telemetry.RunEvent{Type: "EvToolStart", LaneIndex: 0, Tool: "bash", Args: "ls"})
	_ = log.Append(telemetry.RunEvent{Type: "EvToolEnd", LaneIndex: 0, Tool: "bash", Result: "a\nb", Success: true})
	waitForEventLog(t, dir, "run-mux", 2)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/page/runs/run-mux", nil)
	s.hub.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Mux Run") || !strings.Contains(body, "bash") {
		t.Fatalf("timeline did not render the run/tool:\n%s", body)
	}
}
