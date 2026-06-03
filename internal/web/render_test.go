package web

import (
	"errors"
	"strings"
	"testing"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

func TestRenderEvent(t *testing.T) {
	tests := []struct {
		name      string
		event     copilot.Event
		wantOK    bool
		wantEvent string
		contains  []string
	}{
		{
			name:      "message delta wraps in span and escapes",
			event:     copilot.Event{Type: copilot.EvMessageDelta, Text: "a<b "},
			wantOK:    true,
			wantEvent: "msg-delta",
			contains:  []string{"<span>", "a&lt;b ", "</span>"},
		},
		{
			name:      "message done finalizes and reopens cur-msg",
			event:     copilot.Event{Type: copilot.EvMessage, Text: "done"},
			wantOK:    true,
			wantEvent: "msg-done",
			contains:  []string{`class="turn assistant"`, "done", `id="cur-msg"`},
		},
		{
			name:      "idle clears status",
			event:     copilot.Event{Type: copilot.EvIdle},
			wantOK:    true,
			wantEvent: "turn-end",
		},
		{
			name:      "error renders banner",
			event:     copilot.Event{Type: copilot.EvError, Err: errors.New("boom")},
			wantOK:    true,
			wantEvent: "error",
			contains:  []string{`class="turn error"`, "boom"},
		},
		{
			name:   "tool events are not rendered yet",
			event:  copilot.Event{Type: copilot.EvToolStart, Tool: "bash"},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frag, ok := renderEvent(tt.event)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if frag.Event != tt.wantEvent {
				t.Errorf("event = %q, want %q", frag.Event, tt.wantEvent)
			}
			for _, sub := range tt.contains {
				if !strings.Contains(frag.HTML, sub) {
					t.Errorf("html %q missing %q", frag.HTML, sub)
				}
			}
		})
	}
}

func TestEscapeHTMLNewlines(t *testing.T) {
	got := escapeHTML("line1\nline2 <x>")
	want := "line1<br>line2 &lt;x&gt;"
	if got != want {
		t.Errorf("escapeHTML = %q, want %q", got, want)
	}
}

func TestUserBubbleEscapes(t *testing.T) {
	got := userBubble("hi <script>")
	if strings.Contains(got, "<script>") {
		t.Errorf("user bubble did not escape: %q", got)
	}
	if !strings.Contains(got, `class="turn user"`) {
		t.Errorf("user bubble missing class: %q", got)
	}
}
