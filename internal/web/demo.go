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

	// 2b. A file-write permission, rendered as the diff review lane (item 3.1): a
	// collapsible inline unified diff with approve/reject attached. Nothing blocks
	// on the decision in demo mode; submitting it exercises the /perm route.
	m.Emit(copilot.Event{Type: copilot.EvPermission, Permission: &copilot.PermissionRequest{
		ID: "demo-perm-1", Kind: "write", Detail: "write file: internal/summary.go",
		FileName: "internal/summary.go", Intention: "add a Summarize helper",
		Diff: "--- a/internal/summary.go\n" +
			"+++ b/internal/summary.go\n" +
			"@@ -1,4 +1,7 @@\n" +
			" package summary\n" +
			" \n" +
			"-func todo() {}\n" +
			"+// Summarize condenses text to a single line.\n" +
			"+func Summarize(s string) string {\n" +
			"+\treturn s\n" +
			"+}\n",
	}})
	time.Sleep(120 * time.Millisecond)

	// 3. An ask_user prompt (elicitation), rendered as an inline form. In demo
	// mode nothing blocks on the answer; submitting it just exercises the route.
	m.Emit(copilot.Event{Type: copilot.EvUserInput, Input: &copilot.InputRequest{
		ID: "demo-ask-1", Question: "Want a short or detailed summary?",
		Choices: []string{"short", "detailed"}, AllowFreeform: true,
	}})
	time.Sleep(120 * time.Millisecond)

	// 4. A plan change + an exit-plan-mode review, rendered as an inline plan
	// card. Nothing blocks on the decision in demo mode.
	m.Emit(copilot.Event{Type: copilot.EvPlanChanged, Text: "plan created"})
	m.Emit(copilot.Event{Type: copilot.EvPlanReview, Plan: &copilot.PlanRequest{
		ID: "demo-plan-1", Summary: "Add the summary feature in three steps",
		Plan:    "1. parse input\n2. render summary\n3. add tests",
		Actions: []string{"proceed", "edit plan"}, Recommended: "proceed",
	}})
	time.Sleep(120 * time.Millisecond)

	// 4b. A schema-driven elicitation form from an MCP server, rendered as an
	// inline multi-field dialog. Nothing blocks on it in demo mode.
	m.Emit(copilot.Event{Type: copilot.EvElicitation, Elicit: &copilot.ElicitRequest{
		ID: "demo-elicit-1", Message: "Configure the deploy", Source: "deploy-mcp",
		Fields: []copilot.ElicitField{
			{Name: "env", Label: "Environment", Type: "string", Required: true,
				Enum: []string{"staging", "production"}, Default: "staging"},
			{Name: "replicas", Label: "Replicas", Type: "integer", Default: "3"},
			{Name: "confirm", Label: "Run migrations", Type: "boolean", Description: "Apply DB migrations on deploy"},
		},
	}})
	time.Sleep(120 * time.Millisecond)

	// 5. A sub-agent runs in the background, shown as an activity indicator that
	// appears while it works and clears when it finishes.
	model := "claude-sonnet-4-6"
	m.Emit(copilot.Event{Type: copilot.EvSubagentStart, Subagent: &copilot.SubagentInfo{
		ToolCallID: "demo-sa-1", Name: "explorer", DisplayName: "Explore",
		Description: "search the repo", Model: model,
	}})
	time.Sleep(300 * time.Millisecond)
	m.Emit(copilot.Event{Type: copilot.EvSubagentEnd, Subagent: &copilot.SubagentInfo{
		ToolCallID: "demo-sa-1", Name: "explorer", DisplayName: "Explore", Model: model,
		Success: true, Detail: "1.2s · 3.4k tok",
	}})

	// 6. The streamed answer.
	reply := "Streaming skeleton is live. You said: " + strings.TrimSpace(prompt) +
		". Reasoning, tools, the cost meter, and inline permissions all render over SSE."
	stream(reply, copilot.EvMessageDelta)
	m.Emit(copilot.Event{Type: copilot.EvMessage, Text: reply})

	// 7. Usage → cost footer, and a context-window reading → live ctx meter.
	m.Emit(copilot.Event{Type: copilot.EvUsage, Usage: copilot.UsageData{
		Model: "gpt-5", InputTokens: 1200, CachedTokens: 200, OutputTokens: 340,
	}})
	m.Emit(copilot.Event{Type: copilot.EvContextWindow, Context: copilot.ContextInfo{
		CurrentTokens: 18400, TokenLimit: 128000,
	}})

	// 8. End of turn.
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
