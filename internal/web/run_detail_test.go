package web

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
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

	html := s.runDetailPartial(rec, defaultSpendWindow)
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

	html := s.runDetailPartial(rec, defaultSpendWindow)
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
