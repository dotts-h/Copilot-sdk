// Copyright (c) 2026 Horia C. Rădulescu
// SPDX-License-Identifier: BUSL-1.1

package copilot

import (
	"testing"

	sdk "github.com/github/copilot-sdk/go"
)

// historyEvents is the pure normalizer that rebuilds a transcript from a
// session's persisted events (ADR 0002). These pin the mapping and the
// tool-name matching across start/complete.
func TestHistoryEvents(t *testing.T) {
	raw := []sdk.SessionEvent{
		{Data: &sdk.UserMessageData{Content: "fix it"}},
		{Data: &sdk.AssistantReasoningData{Content: "thinking"}},
		{Data: &sdk.AssistantMessageData{Content: "done"}},
		{Data: &sdk.ToolExecutionStartData{ToolCallID: "t1", ToolName: "bash", Arguments: map[string]any{"command": "ls"}}},
		{Data: &sdk.ToolExecutionCompleteData{ToolCallID: "t1", Success: true}},
		{Data: &sdk.SessionIdleData{}}, // not part of the transcript — dropped
	}
	got := historyEvents("s1", raw)

	if len(got) != 5 {
		t.Fatalf("got %d events, want 5: %+v", len(got), got)
	}
	want := []EventType{EvUserMessage, EvReasoning, EvMessage, EvToolStart, EvToolEnd}
	for i, wt := range want {
		if got[i].Type != wt {
			t.Errorf("event %d type = %v, want %v", i, got[i].Type, wt)
		}
		if got[i].SessionID != "s1" {
			t.Errorf("event %d not stamped with session id: %q", i, got[i].SessionID)
		}
	}
	if got[0].Text != "fix it" || got[2].Text != "done" {
		t.Errorf("text mismatch: %q / %q", got[0].Text, got[2].Text)
	}
	// The complete event borrows the tool name recorded by its start event.
	if got[4].ToolCall == nil || got[4].ToolCall.Name != "bash" || !got[4].ToolCall.Success {
		t.Errorf("tool-end not matched to start: %+v", got[4].ToolCall)
	}
}

func TestHistoryEventsEmpty(t *testing.T) {
	if got := historyEvents("s1", nil); len(got) != 0 {
		t.Errorf("empty history should yield no events, got %+v", got)
	}
}
