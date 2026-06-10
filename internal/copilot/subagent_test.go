package copilot

import (
	"testing"

	sdk "github.com/github/copilot-sdk/go"
)

func TestHandlerMapsSubagentLifecycle(t *testing.T) {
	c := newTestSDKClient()
	h := c.makeHandler("")
	model := "claude-sonnet-4-6"
	dur := int64(1200)
	tok := int64(3400)

	h(sdk.SessionEvent{Data: &sdk.SubagentStartedData{
		ToolCallID: "tc-1", AgentName: "explorer", AgentDisplayName: "Explore",
		AgentDescription: "search the repo", Model: &model,
	}})
	h(sdk.SessionEvent{Data: &sdk.SubagentCompletedData{
		ToolCallID: "tc-1", AgentName: "explorer", AgentDisplayName: "Explore",
		Model: &model, DurationMs: &dur, TotalTokens: &tok,
	}})

	start := <-c.events
	if start.Type != EvSubagentStart || start.Subagent == nil {
		t.Fatalf("start: %+v", start)
	}
	if start.Subagent.DisplayName != "Explore" || start.Subagent.ToolCallID != "tc-1" || start.Subagent.Model != model {
		t.Fatalf("start subagent not normalized: %+v", start.Subagent)
	}
	if start.Subagent.Description != "search the repo" {
		t.Fatalf("AgentDescription not mapped to Description: %+v", start.Subagent)
	}
	end := <-c.events
	if end.Type != EvSubagentEnd || end.Subagent == nil || !end.Subagent.Success {
		t.Fatalf("end: %+v", end)
	}
	if end.Subagent.Detail != "1.2s · 3.4k tok" {
		t.Fatalf("end summary = %q, want duration+tokens", end.Subagent.Detail)
	}
	// The raw token count rides beside the formatted Detail: the registry (S2)
	// cross-checks it before trusting a "completed" (claude-code#47936).
	if end.Subagent.TotalTokens != tok {
		t.Fatalf("end TotalTokens = %d, want %d", end.Subagent.TotalTokens, tok)
	}
}

func TestHandlerMapsSubagentFailure(t *testing.T) {
	c := newTestSDKClient()
	h := c.makeHandler("")
	h(sdk.SessionEvent{Data: &sdk.SubagentFailedData{
		ToolCallID: "tc-2", AgentName: "builder", AgentDisplayName: "Build", Error: "boom",
	}})
	ev := <-c.events
	if ev.Type != EvSubagentEnd || ev.Subagent == nil || ev.Subagent.Success {
		t.Fatalf("failure: %+v", ev)
	}
	if ev.Subagent.Detail != "boom" {
		t.Fatalf("failure detail = %q, want error", ev.Subagent.Detail)
	}
}

// TestHandlerStampsAgentID asserts the keystone of epic 0069 (S1): the envelope's
// AgentID is threaded onto EVERY normalized event the handler emits, so a consumer
// can tell a sub-agent's deltas/tools/usage from the root agent's (ADR-0040). The
// SDK sets SessionEvent.AgentID on every event a sub-agent instance produces.
func TestHandlerStampsAgentID(t *testing.T) {
	c := newTestSDKClient()
	h := c.makeHandler("s1")
	sub := "agent-7"
	p := &sub

	// A representative event of every type the streaming handler emits, each carrying
	// the sub-agent envelope tag. ToolStart/Complete share a call id so EvToolEnd is
	// emitted; ReasoningData is sent in its own (no-delta) segment so it isn't deduped.
	h(sdk.SessionEvent{AgentID: p, Data: &sdk.AssistantMessageDeltaData{DeltaContent: "hi"}})
	h(sdk.SessionEvent{AgentID: p, Data: &sdk.AssistantReasoningDeltaData{DeltaContent: "mm"}})
	h(sdk.SessionEvent{AgentID: p, Data: &sdk.AssistantMessageData{Content: "done"}})
	h(sdk.SessionEvent{AgentID: p, Data: &sdk.AssistantUsageData{Model: "m", InputTokens: i64(1)}})
	h(sdk.SessionEvent{AgentID: p, Data: &sdk.ToolExecutionStartData{ToolCallID: "t", ToolName: "bash"}})
	h(sdk.SessionEvent{AgentID: p, Data: &sdk.ToolExecutionProgressData{ToolCallID: "t", ProgressMessage: "x"}})
	h(sdk.SessionEvent{AgentID: p, Data: &sdk.ToolExecutionCompleteData{ToolCallID: "t", Success: true}})
	h(sdk.SessionEvent{AgentID: p, Data: &sdk.SessionUsageInfoData{CurrentTokens: 1, TokenLimit: 2}})
	h(sdk.SessionEvent{AgentID: p, Data: &sdk.SessionIdleData{}})

	want := []EventType{
		EvMessageDelta, EvReasoningDelta, EvMessage, EvUsage,
		EvToolStart, EvToolProgress, EvToolEnd, EvContextWindow, EvIdle,
	}
	for i, w := range want {
		ev := <-c.events
		if ev.Type != w {
			t.Fatalf("event %d type = %v, want %v", i, ev.Type, w)
		}
		if ev.AgentID != sub {
			t.Fatalf("event %d (%v) AgentID = %q, want %q", i, ev.Type, ev.AgentID, sub)
		}
	}
}

// TestHandlerLeavesAgentIDEmptyForRootAgent guards the seam's additivity: an
// event with no envelope AgentID (the root/main agent, every current path) carries
// an empty AgentID, so no existing consumer changes behaviour.
func TestHandlerLeavesAgentIDEmptyForRootAgent(t *testing.T) {
	c := newTestSDKClient()
	h := c.makeHandler("s1")
	h(sdk.SessionEvent{Data: &sdk.AssistantMessageDeltaData{DeltaContent: "hi"}})
	ev := <-c.events
	if ev.AgentID != "" {
		t.Fatalf("root-agent event AgentID = %q, want empty", ev.AgentID)
	}
}

func TestSubagentSummary(t *testing.T) {
	dur := int64(500)
	tok := int64(1_500_000)
	if got := subagentSummary(&dur, &tok); got != "0.5s · 1.5M tok" {
		t.Errorf("summary = %q", got)
	}
	if got := subagentSummary(nil, nil); got != "" {
		t.Errorf("empty summary = %q", got)
	}
}
