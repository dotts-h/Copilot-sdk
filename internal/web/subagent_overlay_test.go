// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/pause"
)

// The per-sub-agent chat overlay (epic 0069 S5, issue 0074): opening a sub-agent
// from the S2 list into a <dialog> with its own live transcript, an input-required
// pause form, and — for a lane-backed sub-agent — a steer composer.

// A tagged stream now also feeds the sub-agent's drill-down transcript, addressed
// to its own named SSE event so an open overlay streams live.
func TestSubagentStreamFoldsTranscript(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-1", "Explore"))

	html := fragFor(s, tagSub("sub-1", copilot.Event{Type: copilot.EvMessageDelta, Text: "scanning internal/…"}), "subagent-tc-1")
	if !strings.Contains(html, "scanning internal/…") {
		t.Fatalf("a tagged delta should fold into the sub-agent transcript fragment: %q", html)
	}
	html = fragFor(s, tagSub("sub-1", copilot.Event{Type: copilot.EvToolStart, Tool: "grep",
		ToolCall: &copilot.ToolCall{ID: "t1", Name: "grep", Args: "func main"}}), "subagent-tc-1")
	if !strings.Contains(html, "grep") || !strings.Contains(html, "func main") {
		t.Fatalf("a tagged tool start should render as a collapsed tool line: %q", html)
	}
}

// GET /subagent/{id} renders the drill-down dialog with the live transcript region
// wired to its named SSE listener.
func TestSubagentOverlayGET(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.handleEvent(subStart("tc-1", "Explore"))
	s.handleEvent(tagSub("sub-1", copilot.Event{Type: copilot.EvMessageDelta, Text: "looking around"}))

	body := getBody(t, srv.URL+"/subagent/tc-1")
	if !strings.Contains(body, "<dialog") || !strings.Contains(body, "Explore") {
		t.Fatalf("overlay should render a dialog for the sub-agent: %q", body)
	}
	if !strings.Contains(body, `sse-swap="subagent-tc-1"`) {
		t.Errorf("transcript region must carry its own named SSE listener: %q", body)
	}
	if !strings.Contains(body, "looking around") {
		t.Errorf("overlay should render the transcript so far: %q", body)
	}
}

// An unknown / cleared sub-agent id is a 404 so htmx leaves the page untouched.
func TestSubagentOverlayUnknownIs404(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/subagent/ghost")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown sub-agent should 404, got %d", resp.StatusCode)
	}
}

// Model/SDK-originated transcript text is HTML-escaped (ADR-0001).
func TestSubagentOverlayEscapesContent(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.handleEvent(subStart("tc-1", "Explore"))
	s.handleEvent(tagSub("sub-1", copilot.Event{Type: copilot.EvMessageDelta, Text: "<img onerror=x> & more"}))

	body := getBody(t, srv.URL+"/subagent/tc-1")
	if strings.Contains(body, "<img onerror=x>") {
		t.Fatalf("transcript text must be escaped, not raw: %q", body)
	}
	if !strings.Contains(body, "&lt;img onerror=x&gt;") || !strings.Contains(body, "&amp;") {
		t.Errorf("transcript not escaped as expected: %q", body)
	}
}

// Closing and reopening renders the same bounded transcript — no duplicate turns
// (idempotent re-render is the SSE-replay foundation).
func TestSubagentOverlayIdempotentReopen(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.handleEvent(subStart("tc-1", "Explore"))
	s.handleEvent(tagSub("sub-1", copilot.Event{Type: copilot.EvMessageDelta, Text: "alpha "}))
	s.handleEvent(tagSub("sub-1", copilot.Event{Type: copilot.EvToolStart, Tool: "grep",
		ToolCall: &copilot.ToolCall{ID: "t1", Name: "grep"}}))

	a := getBody(t, srv.URL+"/subagent/tc-1")
	b := getBody(t, srv.URL+"/subagent/tc-1")
	if a != b {
		t.Fatalf("reopen must render identical HTML:\n%q\n%q", a, b)
	}
}

// An input-required pause addressed to the sub-agent renders its pause form inside
// the overlay, and the registry row flips to the attention state — list and overlay
// agree on state.
func TestSubagentOverlayRendersPauseForm(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.handleEvent(subStart("tc-1", "Explore"))
	s.handleEvent(tagSub("sub-1", copilot.Event{Type: copilot.EvToolStart, Tool: "grep",
		ToolCall: &copilot.ToolCall{ID: "t1", Name: "grep"}})) // join sub-1

	// A pause addressed to the instance + the registry mark (what escalate does).
	s.mu.Lock()
	p := s.pauses.Register(pause.Pause{AgentID: "sub-1", Kind: pause.KindIssue, Message: "need a decision",
		Caps: []pause.Cap{pause.CapContinue, pause.CapCancel}})
	s.subreg.MarkInputRequired("sub-1")
	listHTML := s.subagentsFrag().HTML
	s.mu.Unlock()

	if !strings.Contains(listHTML, "input required") {
		t.Errorf("the list row should show the input-required state: %q", listHTML)
	}
	body := getBody(t, srv.URL+"/subagent/tc-1")
	if !strings.Contains(body, "need a decision") || !strings.Contains(body, "/pause/"+p.ID) {
		t.Fatalf("overlay should render the pause form addressed to this sub-agent: %q", body)
	}
}

// A lane-backed sub-agent shows the steer composer; an SDK-native one does not
// (it has no Send target — read+pause-only).
func TestSubagentOverlaySteerComposerGating(t *testing.T) {
	s, _ := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.handleEvent(subStart("tc-1", "Explore")) // SDK-native
	s.handleEvent(subStart("tc-2", "Builder"))
	s.mu.Lock()
	s.subreg.SetLaneSession("tc-2", "mock-session-7") // lane-backed
	s.mu.Unlock()

	native := getBody(t, srv.URL+"/subagent/tc-1")
	if strings.Contains(native, `class="sa-steer"`) {
		t.Errorf("an SDK-native sub-agent must not show a steer composer: %q", native)
	}
	lane := getBody(t, srv.URL+"/subagent/tc-2")
	if !strings.Contains(lane, `class="sa-steer"`) || !strings.Contains(lane, "/subagent/tc-2/steer") {
		t.Errorf("a lane-backed sub-agent should show the steer composer: %q", lane)
	}
}

// POST /subagent/{id}/steer delivers the human's message to the lane's backing
// session via Send (mock-recorded) and annotates it in the overlay transcript.
func TestSubagentSteerDeliversToSession(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.handleEvent(subStart("tc-2", "Builder"))
	s.mu.Lock()
	s.subreg.SetLaneSession("tc-2", "mock-session-7")
	s.mu.Unlock()

	resp, err := http.PostForm(srv.URL+"/subagent/tc-2/steer", url.Values{"prompt": {"focus on the parser"}})
	if err != nil {
		t.Fatal(err)
	}
	body := readBody(t, resp)

	if len(mock.Sent) != 1 || mock.Sent[0] != "focus on the parser" {
		t.Fatalf("steer should Send the message to the runtime: %+v", mock.Sent)
	}
	if len(mock.SentSessions) != 1 || mock.SentSessions[0] != "mock-session-7" {
		t.Fatalf("steer should target the lane's backing session: %+v", mock.SentSessions)
	}
	if !strings.Contains(body, "focus on the parser") || !strings.Contains(body, "steer") {
		t.Errorf("steer should be annotated in the returned transcript: %q", body)
	}
}

// Steering an SDK-native (no LaneSession) sub-agent must NOT Send — it has no
// target. The transcript is returned unchanged.
func TestSubagentSteerNonLaneNoSend(t *testing.T) {
	s, mock := newTestServer()
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.handleEvent(subStart("tc-1", "Explore")) // no lane session

	if _, err := http.PostForm(srv.URL+"/subagent/tc-1/steer", url.Values{"prompt": {"do something"}}); err != nil {
		t.Fatal(err)
	}
	if len(mock.Sent) != 0 {
		t.Fatalf("an SDK-native sub-agent has no Send target: %+v", mock.Sent)
	}
}

// A failed Send must NOT annotate the transcript as delivered — the human's steer
// is only recorded once it actually reaches the runtime.
func TestSubagentSteerNotAnnotatedOnSendFailure(t *testing.T) {
	s, mock := newTestServer()
	mock.SendErr = errors.New("runtime down")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	s.handleEvent(subStart("tc-2", "Builder"))
	s.mu.Lock()
	s.subreg.SetLaneSession("tc-2", "mock-session-7")
	s.mu.Unlock()

	resp, err := http.PostForm(srv.URL+"/subagent/tc-2/steer", url.Values{"prompt": {"lost message"}})
	if err != nil {
		t.Fatal(err)
	}
	if body := readBody(t, resp); strings.Contains(body, "lost message") {
		t.Errorf("a failed steer must not show as delivered in the transcript: %q", body)
	}
	if v, _ := s.subreg.ByID("tc-2"); len(v.Transcript) != 0 {
		t.Errorf("a failed steer must not be recorded: %+v", v.Transcript)
	}
}

// The row carries the spawn id and the open affordances (button + dblclick) so the
// overlay can be opened by mouse and keyboard.
func TestSubagentRowCarriesOpenAffordance(t *testing.T) {
	s, _ := newTestServer()
	html := fragFor(s, subStart("tc-1", "Explore"), "subagents")
	if !strings.Contains(html, `data-sa-id="tc-1"`) {
		t.Errorf("row should carry its spawn id for the overlay: %q", html)
	}
	if !strings.Contains(html, "openSubagent") || !strings.Contains(html, "sa-open") {
		t.Errorf("row should carry a visible open button (not dblclick-only): %q", html)
	}
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	return readBody(t, resp)
}
