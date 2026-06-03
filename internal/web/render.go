package web

import (
	"html"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/copilot"
)

// fragment is one rendered SSE message: a named event plus its HTML payload.
// An empty Event means the copilot.Event produces no DOM update (yet).
type fragment struct {
	Event string
	HTML  string
}

// renderEvent maps a normalized copilot.Event onto the SSE event→fragment
// contract documented in docs/WEB_UI_PLAN.md. Tool/reasoning/cost/permission
// events are recognized but left as no-ops here; they land in later steps.
func renderEvent(e copilot.Event) (fragment, bool) {
	switch e.Type {
	case copilot.EvMessageDelta:
		// Wrap each delta in a span so whitespace is preserved across appends.
		return fragment{Event: "msg-delta", HTML: "<span>" + escapeHTML(e.Text) + "</span>"}, true

	case copilot.EvMessage:
		// Finalize the streaming bubble into a committed turn and open a fresh,
		// empty #cur-msg for the next turn (outerHTML swap of #cur-msg).
		committed := `<div class="turn assistant"><div class="role">assistant</div>` +
			`<div class="body">` + escapeHTML(e.Text) + `</div></div>`
		fresh := `<div id="cur-msg" class="turn assistant"></div>`
		return fragment{Event: "msg-done", HTML: committed + fresh}, true

	case copilot.EvIdle:
		// Clear the spinner/status line at end of turn.
		return fragment{Event: "turn-end", HTML: ""}, true

	case copilot.EvError:
		msg := "unknown error"
		if e.Err != nil {
			msg = e.Err.Error()
		}
		return fragment{Event: "error", HTML: `<div class="turn error">` + escapeHTML(msg) + `</div>`}, true

	default:
		// EvReasoning(Delta), EvToolStart/Progress/End, EvUsage, EvPermission:
		// rendered in later steps of the migration.
		return fragment{}, false
	}
}

// userBubble renders the echoed user message inserted above #cur-msg on /send.
func userBubble(prompt string) string {
	return `<div class="turn user"><div class="role">you</div><div class="body">` +
		escapeHTML(prompt) + `</div></div>`
}

// escapeHTML escapes model/user text for safe insertion and converts newlines to
// <br> so multi-line content survives single-line SSE framing.
func escapeHTML(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "\n", "<br>")
}
