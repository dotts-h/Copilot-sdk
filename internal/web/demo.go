package web

import (
	"strings"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

// streamDemoReply emits a scripted assistant turn through a MockClient so the web
// UI can be exercised end-to-end with no live Copilot runtime (WEB_UI_PLAN.md).
// It mirrors a real turn's event sequence — reasoning, a permission prompt, a
// tool call, the streamed answer, usage accounting, and idle — so every part of
// the chat surface (reasoning split, tool timeline, inline permission, cost
// footer) is demonstrated.
func streamDemoReply(m *copilot.MockClient, prompt string) {
	stream := func(text string, kind copilot.EventType) {
		for _, tok := range tokenize(text) {
			m.Emit(copilot.Event{Type: kind, Text: tok})
			time.Sleep(30 * time.Millisecond)
		}
	}

	// 1. Reasoning ("thinking"), rendered as a separate dim block.
	stream("Let me look at what you asked and decide how to respond. ", copilot.EvReasoningDelta)

	// 2. A tool execution as a first-class timeline entry.
	m.Emit(copilot.Event{Type: copilot.EvToolStart, Tool: "bash",
		ToolCall: &copilot.ToolCall{ID: "demo-1", Name: "bash", Args: "echo hello"}})
	time.Sleep(120 * time.Millisecond)
	m.Emit(copilot.Event{Type: copilot.EvToolProgress,
		ToolCall: &copilot.ToolCall{ID: "demo-1", Progress: "running…"}})
	time.Sleep(180 * time.Millisecond)
	m.Emit(copilot.Event{Type: copilot.EvToolEnd,
		ToolCall: &copilot.ToolCall{ID: "demo-1", Result: "hello", Success: true}})

	// 3. The streamed answer.
	reply := "Streaming skeleton is live. You said: " + strings.TrimSpace(prompt) +
		". Reasoning, tools, the cost meter, and inline permissions all render over SSE."
	stream(reply, copilot.EvMessageDelta)
	m.Emit(copilot.Event{Type: copilot.EvMessage, Text: reply})

	// 4. Usage → cost footer, and a context-window reading → live ctx meter.
	m.Emit(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 1200, CachedTokens: 200, OutputTokens: 340,
	}})
	m.Emit(copilot.Event{Type: copilot.EvContextWindow, Context: copilot.ContextInfo{
		CurrentTokens: 18400, TokenLimit: 128000,
	}})

	// 5. End of turn.
	m.Emit(copilot.Event{Type: copilot.EvIdle})
}

// tokenize splits text into word-ish tokens (trailing space kept) so the demo
// stream looks like real token output.
func tokenize(s string) []string {
	parts := strings.SplitAfter(s, " ")
	out := parts[:0]
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
