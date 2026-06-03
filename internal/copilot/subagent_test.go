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
	end := <-c.events
	if end.Type != EvSubagentEnd || end.Subagent == nil || !end.Subagent.Success {
		t.Fatalf("end: %+v", end)
	}
	if end.Subagent.Detail != "1.2s · 3.4k tok" {
		t.Fatalf("end summary = %q, want duration+tokens", end.Subagent.Detail)
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
