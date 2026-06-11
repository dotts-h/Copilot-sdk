package web

import (
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
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

func subEnd(tc, name string, ok bool, detail string, tokens int64) copilot.Event {
	return copilot.Event{Type: copilot.EvSubagentEnd, Subagent: &copilot.SubagentInfo{
		ToolCallID: tc, Name: "explorer", DisplayName: name, Success: ok, Detail: detail,
		TotalTokens: tokens,
	}}
}

// The list header carries an attention badge (S6): a count of the sub-agents
// parked on a human-in-the-loop pause. It appears only when something needs the
// human, carries the amber warn class, and — never color-only (a11y) — exposes an
// accessible name describing what the count means.
func TestSubagentListHeaderAttentionBadge(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-1", "Explore"))
	s.handleEvent(subStart("tc-2", "Audit"))

	// Two working entries, none parked → a list, but no attention badge.
	if html := renderSubagents(s.subreg.Entries()); strings.Contains(html, "sa-badge") {
		t.Fatalf("no parked sub-agent should mean no attention badge: %q", html)
	}

	// One parks → the header badge appears with the count and an accessible name.
	s.subreg.MarkInputRequired("tc-1")
	html := renderSubagents(s.subreg.Entries())
	if !strings.Contains(html, "sa-badge") || !strings.Contains(html, "warn") {
		t.Fatalf("a parked sub-agent should raise an amber attention badge: %q", html)
	}
	if !strings.Contains(html, `aria-label="1 sub-agent needs your input"`) {
		t.Errorf("the badge needs an accessible name (not color-only): %q", html)
	}

	// A second parks → the count pluralizes.
	s.subreg.MarkInputRequired("tc-2")
	html = renderSubagents(s.subreg.Entries())
	if !strings.Contains(html, `aria-label="2 sub-agents need your input"`) {
		t.Errorf("the badge count should reflect both parked sub-agents: %q", html)
	}
}

func TestSubagentStartShowsWorkingRow(t *testing.T) {
	s, _ := newTestServer()
	html := fragFor(s, subStart("tc-1", "Explore"), "subagents")
	if html == "" || !strings.Contains(html, "Explore") {
		t.Fatalf("subagent start should render a list row: %q", html)
	}
	if !strings.Contains(html, "working") {
		t.Errorf("a fresh row must carry the textual status label: %q", html)
	}
	if got := len(s.subreg.Entries()); got != 1 {
		t.Fatalf("subagent not registered: %d", got)
	}
}

// A finished sub-agent STAYS listed with its terminal status — the list is the
// session's roster (issue 0071), not the transient busy strip it replaces
// (issue 0031).
func TestSubagentEndKeepsRowAsDone(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-1", "Explore"))
	html := fragFor(s, subEnd("tc-1", "Explore", true, "1.2s · 3.4k tok", 3400), "subagents")
	if got := len(s.subreg.Entries()); got != 1 {
		t.Fatalf("finished subagent must stay listed: %d entries", got)
	}
	if !strings.Contains(html, "Explore") || !strings.Contains(html, "done") {
		t.Errorf("finished row should render with the done label: %q", html)
	}
	if strings.Contains(html, "unverified") {
		t.Errorf("a token-corroborated completion is verified: %q", html)
	}
}

func TestSubagentFailureRowShowsFailed(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-1", "Build"))
	html := fragFor(s, subEnd("tc-1", "Build", false, "boom", 0), "subagents")
	if !strings.Contains(html, "failed") || !strings.Contains(html, "boom") {
		t.Errorf("failed row should carry the failed label and the error: %q", html)
	}
}

// Don't trust completed blindly (claude-code#47936): a successful end that
// reported zero tokens, from an instance whose stream we never observed,
// renders done (unverified).
func TestZeroTokenCompletionRendersUnverified(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-1", "Explore"))
	html := fragFor(s, subEnd("tc-1", "Explore", true, "", 0), "subagents")
	if !strings.Contains(html, "done (unverified)") {
		t.Errorf("zero-token silent completion must render done (unverified): %q", html)
	}
}

func TestSubagentEndAddsTimelineNote(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-1", "Explore"))
	note := fragFor(s, subEnd("tc-1", "Explore", true, "1.2s · 3.4k tok", 3400), "timeline")
	if !strings.Contains(note, "Explore") || !strings.Contains(note, "1.2s") {
		t.Errorf("subagent completion should add a timeline note with the summary: %q", note)
	}
}

func TestSubagentFailureNoted(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-2", "Build"))
	note := fragFor(s, subEnd("tc-2", "Build", false, "boom", 0), "timeline")
	if !strings.Contains(note, "boom") {
		t.Errorf("subagent failure should surface the error: %q", note)
	}
}

func TestClearResetsSubagents(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-1", "Explore"))
	out := s.runCommand("/clear")
	if !s.subreg.Empty() {
		t.Errorf("clear should reset the registry: %d entries", len(s.subreg.Entries()))
	}
	if !strings.Contains(out, `id="subagents"`) {
		t.Errorf("clear should OOB-clear the #subagents list: %s", out)
	}
}

func TestSubagentRowSurfacesDescription(t *testing.T) {
	s, _ := newTestServer()
	html := fragFor(s, subStartDesc("tc-1", "Explore", "search the repo"), "subagents")
	if !strings.Contains(html, `title="search the repo"`) {
		t.Errorf("row should surface the description as a title tooltip: %q", html)
	}
}

func TestSubagentRowEmptyDescriptionKeepsPriorShape(t *testing.T) {
	s, _ := newTestServer()
	// subStart leaves Description empty — the row must render no title attribute,
	// preserving the prior shape (not every sub-agent carries a description).
	html := fragFor(s, subStart("tc-1", "Explore"), "subagents")
	if strings.Contains(html, "title=") {
		t.Errorf("an empty description must not render a title attribute: %q", html)
	}
}

func TestSubagentRowEscapesModelText(t *testing.T) {
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
	// The activity column is tool-originated text on the same surface.
	act := fragFor(s, tagSub("sub-1", copilot.Event{Type: copilot.EvToolStart, Tool: "<img onerror=x>",
		ToolCall: &copilot.ToolCall{ID: "t1", Name: "<img onerror=x>"}}), "subagents")
	if strings.Contains(act, "<img") {
		t.Fatalf("activity must be HTML-escaped, not raw: %q", act)
	}
}

func tagSub(id string, e copilot.Event) copilot.Event { e.AgentID = id; return e }

// The tagged stream now FEEDS the registry (S2): a tool start becomes the row's
// current activity; a tool end / delta returns it to "thinking…". It still
// never touches the root transcript or meter (the S1 invariant, pinned below).
func TestTaggedToolStartUpdatesRowActivity(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-1", "Explore"))
	html := fragFor(s, tagSub("sub-1", copilot.Event{Type: copilot.EvToolStart, Tool: "grep",
		ToolCall: &copilot.ToolCall{ID: "t1", Name: "grep", Args: "func main"}}), "subagents")
	if !strings.Contains(html, "grep") {
		t.Fatalf("tagged tool start should surface as the row's activity: %q", html)
	}
	html = fragFor(s, tagSub("sub-1", copilot.Event{Type: copilot.EvToolEnd,
		ToolCall: &copilot.ToolCall{ID: "t1", Success: true}}), "subagents")
	if !strings.Contains(html, "thinking…") {
		t.Fatalf("tagged tool end should return the activity to thinking: %q", html)
	}
}

// An observed stream verifies the completion even when the lifecycle event
// reports zero tokens — the registry watched the instance do work.
func TestObservedStreamVerifiesCompletion(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-1", "Explore"))
	s.handleEvent(tagSub("sub-1", copilot.Event{Type: copilot.EvToolStart, Tool: "grep",
		ToolCall: &copilot.ToolCall{ID: "t1", Name: "grep"}}))
	html := fragFor(s, subEnd("tc-1", "Explore", true, "", 0), "subagents")
	if strings.Contains(html, "unverified") {
		t.Errorf("an observed stream corroborates the completion: %q", html)
	}
}

// A tagged event with no started sub-agent to join is ignored gracefully: no
// registry entry, no fragment, no transcript mutation.
func TestUnknownInstanceTagIgnored(t *testing.T) {
	s, _ := newTestServer()
	frags := s.handleEvent(tagSub("ghost", copilot.Event{Type: copilot.EvToolStart, Tool: "grep",
		ToolCall: &copilot.ToolCall{ID: "t1", Name: "grep"}}))
	if len(frags) != 0 {
		t.Fatalf("unjoinable tag should emit nothing: %v", frags)
	}
	if !s.subreg.Empty() {
		t.Fatalf("unjoinable tag should not invent an entry")
	}
}

// Replaying the same registry state produces byte-identical HTML: the list is
// an idempotent full-fragment re-render (no append-leak), so an SSE reconnect
// or duplicate render is safe.
func TestSubagentListRenderIdempotent(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStartDesc("tc-1", "Explore", "search the repo"))
	s.handleEvent(tagSub("sub-1", copilot.Event{Type: copilot.EvToolStart, Tool: "grep",
		ToolCall: &copilot.ToolCall{ID: "t1", Name: "grep"}}))
	s.mu.Lock()
	a := s.subagentsFrag().HTML
	b := s.subagentsFrag().HTML
	s.mu.Unlock()
	if a != b {
		t.Fatalf("re-render of the same state must be identical:\n%q\n%q", a, b)
	}
	// A repeated identical activity event changes nothing — no fragment at all
	// (the delta-storm guard).
	if frags := s.handleEvent(tagSub("sub-1", copilot.Event{Type: copilot.EvToolStart, Tool: "grep",
		ToolCall: &copilot.ToolCall{ID: "t1", Name: "grep"}})); len(frags) != 0 {
		t.Errorf("an unchanged registry should emit no fragments: %v", frags)
	}
}

// Credits render from the registry (0.00 cr until S3 wires the priced value).
func TestSubagentRowRendersCreditsPlaceholder(t *testing.T) {
	s, _ := newTestServer()
	html := fragFor(s, subStart("tc-1", "Explore"), "subagents")
	if !strings.Contains(html, "0.00 cr") {
		t.Errorf("row should render the credits cell (0.00 cr until S3): %q", html)
	}
}

// S3 (issue 0072): a sub-agent's tagged usage is priced, folded onto the live
// registry row's credits, and attributed to the instance in the ledger — but
// kept OUT of the root/session token meters (the S1 invariant stays green).
func TestSubagentUsageMetersCreditsOntoRow(t *testing.T) {
	s, _ := newTestServer()
	s.spend, _ = telemetry.LoadSpendStore("") // ephemeral ledger for the assertion
	s.handleEvent(subStart("tc-1", "Explore"))
	html := fragFor(s, tagSub("sub-1", copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 100_000, OutputTokens: 50_000}}), "subagents")
	if html == "" || strings.Contains(html, "0.00 cr") {
		t.Fatalf("sub-agent usage should fold live credits onto the row: %q", html)
	}
	// The spend is attributed to the instance in the ledger (SubagentShares input).
	recs := s.spend.Records()
	if len(recs) != 1 || recs[0].SubagentID != "sub-1" {
		t.Fatalf("usage should append a sub-agent-tagged ledger record: %+v", recs)
	}
	if recs[0].SubagentName != "Explore" {
		t.Errorf("ledger record should carry the display name for a restart-surviving label: %+v", recs[0])
	}
	if recs[0].USD <= 0 {
		t.Errorf("the turn should be priced: %+v", recs[0])
	}
	// S1: a sub-agent's tokens are never metered as the root agent's spend.
	if in, _, out := s.meter.TotalTokens(); in != 0 || out != 0 {
		t.Fatalf("sub-agent usage leaked into the root meter: in=%d out=%d", in, out)
	}
}

// TestAgentTaggedEventsDoNotMutateRootTranscript is the reducer half of epic 0069
// S1 (ADR-0040), still binding under S2: a sub-agent-tagged delta/tool/usage now
// feeds the REGISTRY, but must never append to the root user-facing transcript
// or meter the root agent's spend. The sub-agent LIFECYCLE events (EvSubagentStart/
// End, AgentID empty — session-level events) keep working unchanged.
func TestAgentTaggedEventsDoNotMutateRootTranscript(t *testing.T) {
	s, _ := newTestServer()

	tagged := func(e copilot.Event) copilot.Event { e.AgentID = "sub-1"; return e }
	s.handleEvent(tagged(copilot.Event{Type: copilot.EvMessageDelta, Text: "secret sub-agent text"}))
	s.handleEvent(tagged(copilot.Event{Type: copilot.EvToolStart, Tool: "bash",
		ToolCall: &copilot.ToolCall{ID: "sa-tool", Name: "bash", Args: "rm -rf"}}))
	s.handleEvent(tagged(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 1000, OutputTokens: 500}}))

	if role, text := s.state.Pending(); text != "" {
		t.Fatalf("tagged delta leaked into root transcript: role=%v text=%q", role, text)
	}
	if got := len(s.state.Committed()); got != 0 {
		t.Fatalf("tagged events committed %d root turns, want 0", got)
	}
	if s.toolsUsed != 0 || len(s.state.ActiveTools()) != 0 {
		t.Fatalf("tagged tool mutated root tool state: toolsUsed=%d active=%v", s.toolsUsed, s.state.ActiveTools())
	}
	if in, _, out := s.meter.TotalTokens(); in != 0 || out != 0 {
		t.Fatalf("tagged usage metered as root spend: in=%d out=%d, want 0", in, out)
	}
}

// TestRootEventsStillMutateTranscript pins the other side: an untagged (root)
// delta + usage still drive the transcript and meter, so the filter is scoped to
// tagged events only.
func TestRootEventsStillMutateTranscript(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(copilot.Event{Type: copilot.EvMessageDelta, Text: "root says hi"})
	if _, text := s.state.Pending(); text != "root says hi" {
		t.Fatalf("root delta did not append to transcript: %q", text)
	}
	s.handleEvent(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 1000, OutputTokens: 500}})
	if in, _, out := s.meter.TotalTokens(); in != 1000 || out != 500 {
		t.Fatalf("root usage not metered: in=%d out=%d", in, out)
	}
}

func TestChatPartialRendersActiveSubagent(t *testing.T) {
	s, _ := newTestServer()
	s.handleEvent(subStart("tc-1", "Explore"))
	if html := s.chatPartial(); !strings.Contains(html, "Explore") {
		t.Errorf("chat page should render active subagents: %s", html)
	}
}

// The list is a labelled region: status is conveyed by text inside a named
// container, so assistive tech can find and read it (a11y acceptance).
func TestSubagentListIsLabelledRegion(t *testing.T) {
	s, _ := newTestServer()
	html := fragFor(s, subStart("tc-1", "Explore"), "subagents")
	if !strings.Contains(html, `aria-label="Sub-agents"`) {
		t.Errorf("list should be a labelled region: %q", html)
	}
}
