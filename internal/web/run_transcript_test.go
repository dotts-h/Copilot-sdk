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

// TestClampRunView pins the view clamp: only the explicit "transcript" opts into
// the chat-order view; an empty, garbage, or "timeline" value defaults to the
// timeline — the same discipline as clampWindow.
func TestClampRunView(t *testing.T) {
	cases := map[string]string{
		"transcript": viewTranscript,
		"timeline":   viewTimeline,
		"":           viewTimeline,
		"TRANSCRIPT": viewTimeline, // case-sensitive, like the query contract
		"bogus":      viewTimeline,
	}
	for raw, want := range cases {
		if got := clampRunView(raw); got != want {
			t.Errorf("clampRunView(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestBuildRunTranscript_ChatOrderWithLaneMarkers is the core reconstruction test:
// events flatten in chronological (chat) order — NOT grouped by lane — with a
// lane-transition marker at every lane change, and tool start+end join into one card.
func TestBuildRunTranscript_ChatOrderWithLaneMarkers(t *testing.T) {
	events := []telemetry.RunEvent{
		ev("EvUserMessage", 0),
		ev("EvReasoning", 0),    // coalesced away (not a committed turn)
		ev("EvMessageDelta", 0), // coalesced away
		ev("EvMessage", 0),      // assistant turn A
		{Type: "EvToolStart", LaneIndex: 1, Tool: "bash", Args: "ls", At: time.Now()},
		{Type: "EvToolEnd", LaneIndex: 1, Tool: "bash", Result: "ok", Success: true, At: time.Now()},
		ev("EvMessage", 0), // back to lane 0 — a new marker, chat order interleaves lanes
	}
	items := buildRunTranscript(events)

	wantKinds := []string{"lane", "user", "assistant", "lane", "tool", "lane", "assistant"}
	if len(items) != len(wantKinds) {
		t.Fatalf("want %d items, got %d: %+v", len(wantKinds), len(items), items)
	}
	for i, want := range wantKinds {
		if items[i].Kind != want {
			t.Fatalf("item %d kind = %q, want %q (full: %+v)", i, items[i].Kind, want, items)
		}
	}
	// Reasoning / deltas never surface as turns.
	for _, it := range items {
		if it.Kind == "reasoning" || strings.Contains(it.Kind, "delta") {
			t.Fatalf("a non-committed event leaked into the transcript: %+v", it)
		}
	}
	// The tool start+end joined into ONE done card carrying both args and result.
	tool := items[4]
	if tool.Label != "bash" || tool.Args != "ls" || tool.Result != "ok" || tool.State != "done" {
		t.Fatalf("tool card did not join start+end: %+v", tool)
	}
	// Lane markers carry their lane index for label resolution.
	if items[0].LaneIndex != 0 || items[3].LaneIndex != 1 || items[5].LaneIndex != 0 {
		t.Fatalf("lane markers carry the wrong lane index: %+v", items)
	}
}

// TestBuildRunTranscript_PerAssistantCredits proves O2's priced usage attaches to
// the assistant turn it belongs to: usage reported after a turn lands on that turn;
// usage seen before any assistant turn in a lane seeds the next one.
func TestBuildRunTranscript_PerAssistantCredits(t *testing.T) {
	events := []telemetry.RunEvent{
		ev("EvMessage", 0), // turn A
		{Type: "EvUsage", LaneIndex: 0, Credits: 0.30, At: time.Now()}, // → A
		ev("EvMessage", 0), // turn B
		{Type: "EvUsage", LaneIndex: 0, Credits: 0.20, At: time.Now()}, // → B
		// Lane 1: usage LEADS the turn (reported before the committed message).
		{Type: "EvUsage", LaneIndex: 1, Credits: 0.05, At: time.Now()},
		ev("EvMessage", 1), // turn C, seeded by the pending 0.05
	}
	items := buildRunTranscript(events)

	var assistants []transcriptItem
	for _, it := range items {
		if it.Kind == "assistant" {
			assistants = append(assistants, it)
		}
	}
	if len(assistants) != 3 {
		t.Fatalf("want 3 assistant turns, got %d: %+v", len(assistants), assistants)
	}
	if assistants[0].Credits != 0.30 {
		t.Errorf("turn A credits = %v, want 0.30", assistants[0].Credits)
	}
	if assistants[1].Credits != 0.20 {
		t.Errorf("turn B credits = %v, want 0.20", assistants[1].Credits)
	}
	if assistants[2].Credits != 0.05 {
		t.Errorf("turn C (lead-usage) credits = %v, want 0.05", assistants[2].Credits)
	}
}

// TestRunDetailPartial_TranscriptRendersMarkdown proves an assistant message body
// goes through the block-AST designed-output pipeline (renderMarkdown) — markdown
// renders as designed HTML, not raw asterisks. (Acceptance: message bodies designed.)
func TestRunDetailPartial_TranscriptRendersMarkdown(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	rec := telemetry.RunRecord{ID: "run-md", Name: "MD Run", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done"}}}
	_ = s.runs.Append(rec)
	log, _ := telemetry.LoadRunEventLog(dir, "run-md")
	_ = log.Append(telemetry.RunEvent{Type: "EvUserMessage", LaneIndex: 0, Text: "ship it"})
	_ = log.Append(telemetry.RunEvent{Type: "EvMessage", LaneIndex: 0, Text: "done — **bold** result"})
	waitForEventLog(t, dir, "run-md", 2)

	html := s.runDetailPartial(rec, defaultSpendWindow, viewTranscript)
	if !strings.Contains(html, "<strong>bold</strong>") {
		t.Fatalf("assistant body should render markdown through the block-AST pipeline:\n%s", html)
	}
	if strings.Contains(html, "**bold**") {
		t.Fatalf("raw markdown leaked — the body was not rendered:\n%s", html)
	}
	// The timeline view of the same run renders the message as a plain step (no markdown).
	timeline := s.runDetailPartial(rec, defaultSpendWindow, viewTimeline)
	if strings.Contains(timeline, "<strong>bold</strong>") {
		t.Fatalf("the timeline view must not run message bodies through markdown:\n%s", timeline)
	}
}

// TestRunDetailPartial_TranscriptEscapesHostile proves a hostile message body
// cannot inject markup through the transcript renderer (ADR-0001).
func TestRunDetailPartial_TranscriptEscapesHostile(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	rec := telemetry.RunRecord{ID: "run-xss", Name: "XSS", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done"}}}
	_ = s.runs.Append(rec)
	log, _ := telemetry.LoadRunEventLog(dir, "run-xss")
	_ = log.Append(telemetry.RunEvent{Type: "EvMessage", LaneIndex: 0, Text: "<script>alert(1)</script>"})
	waitForEventLog(t, dir, "run-xss", 1)

	html := s.runDetailPartial(rec, defaultSpendWindow, viewTranscript)
	if strings.Contains(html, "<script>alert(1)</script>") {
		t.Fatal("hostile message body was not escaped in the transcript")
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Fatalf("expected the escaped script entity:\n%s", html)
	}
}

// TestRunDetailPartial_GarbageViewRendersTimeline proves the view toggle clamps: an
// empty or unrecognized ?view= renders the timeline (lane-grouped steps), never a
// half-built page.
func TestRunDetailPartial_GarbageViewRendersTimeline(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	rec := telemetry.RunRecord{ID: "run-clamp", Name: "Clamp", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done"}}}
	_ = s.runs.Append(rec)
	log, _ := telemetry.LoadRunEventLog(dir, "run-clamp")
	_ = log.Append(telemetry.RunEvent{Type: "EvMessage", LaneIndex: 0, Text: "hello"})
	waitForEventLog(t, dir, "run-clamp", 1)

	for _, view := range []string{"", "bogus", "Transcript"} {
		html := s.runDetailPartial(rec, defaultSpendWindow, view)
		if !strings.Contains(html, "timeline-steps") {
			t.Fatalf("view %q should clamp to the timeline:\n%s", view, html)
		}
		if strings.Contains(html, `class="transcript"`) {
			t.Fatalf("view %q must not render the transcript:\n%s", view, html)
		}
	}
}

// TestHandleRunDetail_TranscriptThroughMux drives the real mux end-to-end: a seeded
// run rendered with ?view=transcript surfaces the chat-order transcript, and the
// view toggle offers both views.
func TestHandleRunDetail_TranscriptThroughMux(t *testing.T) {
	dir := t.TempDir()
	s := newTestServerWithRunsAndLog(t, dir)
	rec := telemetry.RunRecord{ID: "run-tx", Name: "TX Run", Mode: "sequential", Outcome: "finished",
		Lanes: []telemetry.RunLane{{Index: 0, AgentID: "builder", Status: "done"}}}
	_ = s.runs.Append(rec)
	log, _ := telemetry.LoadRunEventLog(dir, "run-tx")
	_ = log.Append(telemetry.RunEvent{Type: "EvUserMessage", LaneIndex: 0, Text: "go"})
	_ = log.Append(telemetry.RunEvent{Type: "EvMessage", LaneIndex: 0, Text: "going"})
	waitForEventLog(t, dir, "run-tx", 2)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/page/runs/run-tx?view=transcript", nil)
	s.hub.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `class="transcript"`) {
		t.Fatalf("the transcript container did not render:\n%s", body)
	}
	if !strings.Contains(body, "view=transcript") || !strings.Contains(body, "view=timeline") {
		t.Fatalf("the view toggle should offer both views:\n%s", body)
	}
	if !strings.Contains(body, "going") {
		t.Fatalf("the assistant turn did not render:\n%s", body)
	}
}
