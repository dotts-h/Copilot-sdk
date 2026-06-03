package web

import (
	"strings"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

// streamDemoReply emits a scripted, token-by-token assistant reply through a
// MockClient so the walking skeleton streams visibly in a browser without a live
// Copilot runtime (WEB_UI_PLAN.md step 1). It mirrors the real client's event
// sequence: a run of EvMessageDelta, then EvMessage, then EvIdle.
func streamDemoReply(m *copilot.MockClient, prompt string) {
	reply := "Streaming skeleton is live. You said: " + strings.TrimSpace(prompt) +
		". Tokens arrive over SSE as msg-delta fragments and finalize on msg-done."

	for _, tok := range tokenize(reply) {
		m.Emit(copilot.Event{Type: copilot.EvMessageDelta, Text: tok})
		time.Sleep(35 * time.Millisecond)
	}
	m.Emit(copilot.Event{Type: copilot.EvMessage, Text: reply})
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
