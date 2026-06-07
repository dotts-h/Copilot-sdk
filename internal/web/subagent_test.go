package web

import (
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

func fragFor(s *Server, e copilot.Event, name string) string {
	for _, f := range s.handleEvent(e) {
		if f.Event == name {
			return f.HTML
		}
	}
	return ""
}

func subStart(tc, name string) copilot.Event {
	return copilot.Event{Type: copilot.EvSubagentStart, Subagent: &copilot.SubagentInfo{
		ToolCallID: tc, Name: "explorer", DisplayName: name, Model: "sonnet",
	}}
}

func subStartDesc(tc, name, desc string) copilot.Event {
	return copilot.Event{Type: copilot.EvSubagentStart, Subagent: &copilot.SubagentInfo{
		ToolCallID: tc, Name: "explorer", DisplayName: name, Description: desc, Model: "sonnet",
	}}
}

func subEnd(tc, name string, ok bool, detail string) copilot.Event {
	return copilot.Event{Type: copilot.EvSubagentEnd, Subagent: &copilot.SubagentInfo{
		ToolCallID: tc, Name: "explorer", DisplayName: name, Success: ok, Detail: detail,
	}}
}

func TestSubagentStartShowsIndicator(t *testing.T) {
	s, _ := newTestServer()
	html := fragFor(s, subStart("tc-1", "Explore"), "subagents")
	if html == "" || !strings.Contains(html, "Explore") {
		t.Fatalf("subagent start should render an activity indicator: %q", html)
	}
	if len(s.subagents) != 1 {
		t.Fatalf("active subagent not tracked: %d", len(s.subagents))
	}
}

func TestSubagentEndClearsIndicator(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-1", "Explore"))
	html := fragFor(s, subEnd("tc-1", "Explore", true, "1.2s · 3.4k tok"), "subagents")
	if len(s.subagents) != 0 {
		t.Fatalf("completed subagent should be removed: %d remain", len(s.subagents))
	}
	// With no active subagents the strip is empty (ambient — appears only while running).
	if strings.Contains(html, "Explore") {
		t.Errorf("strip should be empty once the subagent finished: %q", html)
	}
}

func TestSubagentEndAddsTimelineNote(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-1", "Explore"))
	note := fragFor(s, subEnd("tc-1", "Explore", true, "1.2s · 3.4k tok"), "timeline")
	if !strings.Contains(note, "Explore") || !strings.Contains(note, "1.2s") {
		t.Errorf("subagent completion should add a timeline note with the summary: %q", note)
	}
}

func TestSubagentFailureNoted(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-2", "Build"))
	note := fragFor(s, subEnd("tc-2", "Build", false, "boom"), "timeline")
	if !strings.Contains(note, "boom") {
		t.Errorf("subagent failure should surface the error: %q", note)
	}
}

func TestClearResetsSubagents(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-1", "Explore"))
	out := s.runCommand("/clear")
	if len(s.subagents) != 0 {
		t.Errorf("clear should drop active subagents: %d", len(s.subagents))
	}
	if !strings.Contains(out, `id="subagents"`) {
		t.Errorf("clear should OOB-clear the #subagents strip: %s", out)
	}
}

func TestSubagentChipSurfacesDescription(t *testing.T) {
	s, _ := newTestServer()
	html := fragFor(s, subStartDesc("tc-1", "Explore", "search the repo"), "subagents")
	if !strings.Contains(html, `title="search the repo"`) {
		t.Errorf("chip should surface the description as a title tooltip: %q", html)
	}
}

func TestSubagentChipEmptyDescriptionKeepsPriorShape(t *testing.T) {
	s, _ := newTestServer()
	// subStart leaves Description empty — the chip must render no title attribute,
	// preserving the prior shape (not every sub-agent carries a description).
	html := fragFor(s, subStart("tc-1", "Explore"), "subagents")
	if strings.Contains(html, "title=") {
		t.Errorf("an empty description must not render a title attribute: %q", html)
	}
}

func TestSubagentChipEscapesDescription(t *testing.T) {
	s, _ := newTestServer()
	// Description is model/SDK-originated text → it must be HTML-escaped (ADR-0001),
	// mirroring TestWorkflowLanesEscapeModelText.
	html := fragFor(s, subStartDesc("tc-1", "Explore", "<b>grep</b> & scan"), "subagents")
	if strings.Contains(html, "<b>grep</b>") {
		t.Fatalf("description must be HTML-escaped, not raw: %q", html)
	}
	if !strings.Contains(html, "&lt;b&gt;grep&lt;/b&gt;") || !strings.Contains(html, "&amp;") {
		t.Errorf("description not escaped as expected: %q", html)
	}
}

func TestChatPartialRendersActiveSubagent(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-1", "Explore"))
	if html := s.chatPartial(); !strings.Contains(html, "Explore") {
		t.Errorf("chat page should render active subagents: %s", html)
	}
}
