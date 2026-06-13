// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// seedCompareRun appends a run to the store and (when msg != "") writes a two-event
// log for it — a user prompt and a final assistant message — so the compare view can
// reconstruct the run's final output.
func seedCompareRun(t *testing.T, s *Server, dir, id, workflow, outcome, msg string, lanes ...telemetry.RunLane) telemetry.RunRecord {
	t.Helper()
	base := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	rec := telemetry.RunRecord{
		ID: id, WorkflowID: workflow, Name: workflow, Mode: "sequential",
		StartedAt: base, FinishedAt: base.Add(time.Minute), Outcome: outcome, Lanes: lanes,
	}
	_ = s.runs.Append(rec)
	if msg != "" {
		log, _ := telemetry.LoadRunEventLog(dir, id)
		_ = log.Append(telemetry.RunEvent{Type: "EvUserMessage", LaneIndex: 0, Text: "go"})
		_ = log.Append(telemetry.RunEvent{Type: "EvMessage", LaneIndex: 0, Text: msg})
		waitForEventLog(t, dir, id, 2)
	}
	return rec
}

// TestRunCompare_SideBySideDeltas proves two runs of one workflow render side-by-side
// with per-field deltas (total credits, duration, per-lane) — the keyed comparison.
func TestRunCompare_SideBySideDeltas(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	a := seedCompareRun(t, s, dir, "run-a", "build", "failed", "",
		telemetry.RunLane{Index: 0, AgentID: "builder", Status: "failed", Credits: 1.0})
	b := seedCompareRun(t, s, dir, "run-b", "build", "finished", "",
		telemetry.RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 2.5})

	html := s.runComparePartial(a, b, defaultSpendWindow)
	// Both run ids appear (the two sides).
	if !strings.Contains(html, "run-a") || !strings.Contains(html, "run-b") {
		t.Fatalf("both run ids should appear in the comparison:\n%s", html)
	}
	// The credits delta (B−A = +1.50) renders.
	if !strings.Contains(html, "1.50") {
		t.Fatalf("expected the +1.50 credits delta:\n%s", html)
	}
	// Per-lane statuses from both sides render.
	if !strings.Contains(html, "failed") || !strings.Contains(html, "done") {
		t.Fatalf("per-lane statuses from both runs should render:\n%s", html)
	}
}

// TestRunCompare_RefusesCrossWorkflow proves comparing two runs of DIFFERENT workflows
// is refused cleanly — a note, not a meaningless lane diff.
func TestRunCompare_RefusesCrossWorkflow(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	a := seedCompareRun(t, s, dir, "run-a", "build", "finished", "",
		telemetry.RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 1.0})
	b := seedCompareRun(t, s, dir, "run-c", "ship", "finished", "",
		telemetry.RunLane{Index: 0, AgentID: "shipper", Status: "done", Credits: 1.0})

	html := s.runComparePartial(a, b, defaultSpendWindow)
	if !strings.Contains(html, "different workflow") {
		t.Fatalf("cross-workflow comparison should be refused with a note:\n%s", html)
	}
	// No per-lane delta table when refused.
	if strings.Contains(html, "compare-lanes") {
		t.Fatalf("a refused comparison must not render the per-lane table:\n%s", html)
	}
}

// TestRunCompare_FinalOutputsNeedBothLogs proves the final outputs render only when
// BOTH event logs exist; a missing log degrades to the summary-only comparison.
func TestRunCompare_FinalOutputsNeedBothLogs(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	// Both logs present → outputs render.
	a := seedCompareRun(t, s, dir, "run-a", "build", "finished", "ALPHA output",
		telemetry.RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 1.0})
	b := seedCompareRun(t, s, dir, "run-b", "build", "finished", "BETA output",
		telemetry.RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 2.0})

	html := s.runComparePartial(a, b, defaultSpendWindow)
	if !strings.Contains(html, "ALPHA output") || !strings.Contains(html, "BETA output") {
		t.Fatalf("both final outputs should render when both logs exist:\n%s", html)
	}

	// One log missing → outputs degrade away, summary still renders.
	c := seedCompareRun(t, s, dir, "run-c", "build", "finished", "",
		telemetry.RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 3.0})
	html2 := s.runComparePartial(a, c, defaultSpendWindow)
	if strings.Contains(html2, "ALPHA output") {
		t.Fatalf("final outputs must not render when one log is missing:\n%s", html2)
	}
	if !strings.Contains(html2, "run-c") {
		t.Fatalf("the summary comparison should still render with a missing log:\n%s", html2)
	}
}

// TestRunCompare_EscapesHostileOutput proves a hostile final output cannot inject
// markup — the rendered page escapes it (ADR-0001).
func TestRunCompare_EscapesHostileOutput(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	a := seedCompareRun(t, s, dir, "run-a", "build", "finished", "<script>alert(1)</script>",
		telemetry.RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 1.0})
	b := seedCompareRun(t, s, dir, "run-b", "build", "finished", "plain",
		telemetry.RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 1.0})
	html := s.runComparePartial(a, b, defaultSpendWindow)
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatalf("hostile final output was not escaped:\n%s", html)
	}
}

// TestHandleRunCompare_ThroughMux drives the real mux: GET /runs/compare?a=&b=
// renders the comparison; an unknown id 404s.
func TestHandleRunCompare_ThroughMux(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	_ = seedCompareRun(t, s, dir, "run-a", "build", "finished", "",
		telemetry.RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 1.0})
	_ = seedCompareRun(t, s, dir, "run-b", "build", "finished", "",
		telemetry.RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 2.0})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/runs/compare?a=run-a&b=run-b", nil)
	s.hub.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "run-b") {
		t.Fatalf("comparison did not render through the mux:\n%s", rr.Body.String())
	}

	// Unknown id 404s.
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/runs/compare?a=run-a&b=nope", nil)
	s.hub.Handler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("unknown id should 404, got %d", rr2.Code)
	}
}

// TestSignedDeltas_EpsilonAndSign pins the compare view's delta rendering: an explicit
// sign for a real change, and a clean "equal" rendering (no signed "0", no "up" colour)
// for a delta that is zero or sub-threshold float residue — so the displayed magnitude
// and the up flag can never disagree.
func TestSignedDeltas_EpsilonAndSign(t *testing.T) {
	// Credits: positive rises, negative falls, sub-cent residue reads equal.
	if s, up := signedCredits(1.5); s != "+1.50 cr" || !up {
		t.Errorf("signedCredits(1.5) = %q,%v; want +1.50 cr,true", s, up)
	}
	if s, up := signedCredits(-0.30); s != "-0.30 cr" || up {
		t.Errorf("signedCredits(-0.30) = %q,%v; want -0.30 cr,false", s, up)
	}
	if s, up := signedCredits(1e-9); s != "0.00 cr" || up {
		t.Errorf("signedCredits(residue) = %q,%v; want 0.00 cr,false", s, up)
	}
	if s, up := signedCredits(-1e-9); s != "0.00 cr" || up {
		t.Errorf("signedCredits(-residue) = %q,%v; want 0.00 cr,false (no -0.00)", s, up)
	}
	// Duration: a span that rounds to zero collapses to "0s", not a signed "0s".
	if s, up := signedDuration(90 * time.Second); s != "+1m 30s" || !up {
		t.Errorf("signedDuration(90s) = %q,%v; want +1m 30s,true", s, up)
	}
	if s, up := signedDuration(-25 * time.Second); s != "-25s" || up {
		t.Errorf("signedDuration(-25s) = %q,%v; want -25s,false", s, up)
	}
	if s, up := signedDuration(300 * time.Millisecond); s != "0s" || up {
		t.Errorf("signedDuration(300ms) = %q,%v; want 0s,false", s, up)
	}
	// Pauses: exact ints.
	if s, up := signedInt(2); s != "+2" || !up {
		t.Errorf("signedInt(2) = %q,%v; want +2,true", s, up)
	}
	if s, up := signedInt(-1); s != "-1" || up {
		t.Errorf("signedInt(-1) = %q,%v; want -1,false", s, up)
	}
}

// TestHandleRunCompare_SelfCompare404 proves comparing a run with itself 404s (the
// all-zero self-diff the picker never offers).
func TestHandleRunCompare_SelfCompare404(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	_ = seedCompareRun(t, s, dir, "run-a", "build", "finished", "",
		telemetry.RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 1.0})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/runs/compare?a=run-a&b=run-a", nil)
	s.hub.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("self-compare should 404, got %d", rr.Code)
	}
}

// TestRunDetail_CompareWithPicker proves the run-detail page offers a "compare with"
// control listing the OTHER runs of the same workflow (and not the run itself, nor
// runs of a different workflow).
func TestRunDetail_CompareWithPicker(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	rec := seedCompareRun(t, s, dir, "run-a", "build", "finished", "",
		telemetry.RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 1.0})
	_ = seedCompareRun(t, s, dir, "run-b", "build", "finished", "",
		telemetry.RunLane{Index: 0, AgentID: "builder", Status: "done", Credits: 2.0})
	_ = seedCompareRun(t, s, dir, "run-z", "ship", "finished", "",
		telemetry.RunLane{Index: 0, AgentID: "shipper", Status: "done", Credits: 1.0})

	html := s.runDetailPartial(rec, defaultSpendWindow, viewTimeline)
	// A compare link to the other same-workflow run, anchored on this run as A.
	if !strings.Contains(html, "/runs/compare?a=run-a&amp;b=run-b") {
		t.Fatalf("expected a compare-with link to run-b:\n%s", html)
	}
	// The cross-workflow run is not offered.
	if strings.Contains(html, "b=run-z") {
		t.Fatalf("a cross-workflow run must not be a compare candidate:\n%s", html)
	}
}
