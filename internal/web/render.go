package web

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/convo"
	"github.com/dotts-h/copilot-sdk/internal/copilot"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file renders convo state and copilot events into HTML fragments. All
// model/user text flows through esc(), which HTML-escapes and converts newlines
// to <br> so multi-line content survives single-line SSE framing.

// esc escapes text for safe insertion and converts newlines to <br>.
func esc(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "\n", "<br>")
}

// deltaSpan is the incremental fragment appended to #cur for each streamed
// token. Wrapping in a span preserves whitespace across appends.
func deltaSpan(text string) string { return "<span>" + esc(text) + "</span>" }

// renderTurn renders one committed transcript turn.
func renderTurn(t convo.Turn) string {
	switch t.Role {
	case convo.RoleUser:
		return `<div class="turn user"><div class="role">you</div><div class="body">` + esc(t.Text) + `</div></div>`
	case convo.RoleAgent:
		return `<div class="turn assistant"><div class="role">orchestra</div><div class="body">` + esc(t.Text) + `</div></div>`
	case convo.RoleReasoning:
		return `<details class="turn reasoning"><summary>✻ thinking</summary><div class="body">` + esc(t.Text) + `</div></details>`
	case convo.RoleSystem:
		return `<div class="turn system">· ` + esc(t.Text) + `</div>`
	case convo.RoleTool:
		return renderToolCard(t.Tool)
	}
	return ""
}

// renderToolCard renders one tool-execution timeline entry: a status glyph with
// the tool name and argument summary, a live progress line while running, and
// the (bounded) result on completion.
func renderToolCard(tv *convo.ToolView) string {
	if tv == nil {
		return ""
	}
	glyph, state := "●", "running"
	switch {
	case tv.Failed:
		glyph, state = "✗", "failed"
	case tv.Done:
		glyph, state = "✓", "done"
	}
	var b strings.Builder
	id := ""
	if tv.ID != "" {
		id = ` id="tool-` + esc(tv.ID) + `"`
	}
	fmt.Fprintf(&b, `<div%s class="turn tool %s">`, id, state)
	b.WriteString(`<div class="tool-head"><span class="glyph">` + glyph + `</span> <span class="tool-name">` + esc(tv.Name) + `</span>`)
	if tv.Args != "" {
		b.WriteString(` <span class="tool-args">` + esc(tv.Args) + `</span>`)
	}
	b.WriteString(`</div>`)
	if !tv.Done && tv.Progress != "" {
		b.WriteString(`<div class="tool-progress">… ` + esc(tv.Progress) + `</div>`)
	}
	if tv.Done && tv.Result != "" {
		b.WriteString(`<pre class="tool-result">` + esc(clampLines(tv.Result, 16)) + `</pre>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// renderCur renders the trailing live node (#cur) from the in-flight buffer.
// Streamed deltas append spans directly into it; on the next structural
// re-render it is rebuilt here (or committed and replaced by an empty one).
func renderCur(role convo.Role, text string) string {
	class := "turn assistant"
	if role == convo.RoleReasoning {
		class = "turn reasoning live"
	}
	inner := ""
	if text != "" {
		inner = "<span>" + esc(text) + "</span>"
	}
	return `<div id="cur" class="` + class + `">` + inner + `</div>`
}

// renderTimelineInner builds the full #timeline contents: every committed turn
// followed by the live #cur node.
func renderTimelineInner(st *convo.State) string {
	var b strings.Builder
	for _, t := range st.Committed() {
		b.WriteString(renderTurn(t))
	}
	role, text := st.Pending()
	b.WriteString(renderCur(role, text))
	return b.String()
}

// renderPermForm renders an inline approve/reject control for a pending
// tool-permission request, posting the decision to /perm/{id}.
func renderPermForm(id, detail string) string {
	eid := esc(id)
	return `<form class="perm" id="perm-` + eid + `" hx-post="/perm/` + eid +
		`" hx-target="#perm-` + eid + `" hx-swap="outerHTML">` +
		`<span class="perm-q">⚠ allow <b>` + esc(detail) + `</b>?</span>` +
		`<button class="ok" name="approve" value="1">approve</button>` +
		`<button class="no" name="approve" value="0" formnovalidate>reject</button>` +
		`</form>`
}

// renderAskForm renders an inline ask_user prompt: the question, one submit
// button per suggested choice, and (when freeform is allowed or no choices are
// offered) a text field for a custom answer. It posts to /ask/{id}, mirroring
// renderPermForm. Choice buttons and the freeform field share the name "answer";
// the handler picks the first non-empty value, so an empty freeform field next
// to a clicked choice is harmless.
func renderAskForm(req copilot.InputRequest) string {
	eid := esc(req.ID)
	var b strings.Builder
	b.WriteString(`<form class="ask" id="ask-` + eid + `" hx-post="/ask/` + eid +
		`" hx-target="#ask-` + eid + `" hx-swap="outerHTML">`)
	b.WriteString(`<span class="ask-q">❓ <b>` + esc(req.Question) + `</b></span>`)
	if len(req.Choices) > 0 {
		b.WriteString(`<div class="ask-choices">`)
		for _, c := range req.Choices {
			b.WriteString(`<button class="ask-choice" name="answer" value="` + esc(c) + `">` + esc(c) + `</button>`)
		}
		b.WriteString(`</div>`)
	}
	if req.AllowFreeform || len(req.Choices) == 0 {
		b.WriteString(`<div class="ask-freeform">` +
			`<input type="text" name="answer" autocomplete="off" placeholder="type an answer…">` +
			`<button class="ask-send" type="submit">send</button>` +
			`</div>`)
	}
	b.WriteString(`</form>`)
	return b.String()
}

// renderPlanForm renders an inline exit-plan-mode review: the agent's summary,
// the full plan (collapsible), one approve button per offered action (the
// recommended one marked), and a freeform field to decline and request changes.
// It posts to /plan/{id}, reusing the elicitation form pattern. Approve buttons
// share name "action"; the request-changes field is name "feedback".
func renderPlanForm(req copilot.PlanRequest) string {
	eid := esc(req.ID)
	var b strings.Builder
	b.WriteString(`<form class="plan" id="plan-` + eid + `" hx-post="/plan/` + eid +
		`" hx-target="#plan-` + eid + `" hx-swap="outerHTML">`)
	b.WriteString(`<div class="plan-summary">📋 <b>` + esc(req.Summary) + `</b></div>`)
	if req.Plan != "" {
		b.WriteString(`<details class="plan-detail"><summary>view plan</summary><pre>` + esc(req.Plan) + `</pre></details>`)
	}
	if len(req.Actions) > 0 {
		b.WriteString(`<div class="plan-actions">`)
		for _, a := range req.Actions {
			cls := "plan-action"
			label := esc(a)
			if a == req.Recommended {
				cls += " recommended"
				label += " ★"
			}
			b.WriteString(`<button class="` + cls + `" name="action" value="` + esc(a) + `">` + label + `</button>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`<div class="plan-feedback">` +
		`<input type="text" name="feedback" autocomplete="off" placeholder="request changes…">` +
		`<button class="plan-reject" type="submit">request changes</button>` +
		`</div>`)
	b.WriteString(`</form>`)
	return b.String()
}

// renderStatus renders the status-line content swapped into #status. While a
// turn is active it appends a live elapsed-time timer (ticked client-side from
// the data-start epoch) and an inline abort control (POST /abort); when idle it
// is just the (possibly empty) status text, so both disappear on their own.
func renderStatus(text string, active bool, startMs int64) string {
	var b strings.Builder
	b.WriteString(`<span class="status-text">` + esc(text) + `</span>`)
	if active {
		if startMs > 0 {
			fmt.Fprintf(&b, ` <span class="elapsed" data-start="%d">0s</span>`, startMs)
		}
		b.WriteString(` <button class="abort" hx-post="/abort" hx-target="#status" hx-swap="innerHTML">⏹ stop</button>`)
	}
	return b.String()
}

// renderCtx renders the live context-window meter (#ctx): a token count and
// fill percentage of the model's context window, or a compaction indicator while
// the conversation is being compacted. Empty until the first usage reading.
func renderCtx(cur, limit int64, compacting bool) string {
	if compacting {
		return `<span class="ctx-compacting">✻ compacting…</span>`
	}
	if limit <= 0 {
		if cur <= 0 {
			return ""
		}
		return `<span class="ctx-tokens">` + humanTokens(cur) + ` tok</span>`
	}
	pct := int(float64(cur)/float64(limit)*100 + 0.5)
	if pct > 100 {
		pct = 100
	}
	cls := "ctx-meter"
	if pct >= 80 {
		cls += " warn"
	}
	return fmt.Sprintf(`<span class="%s">ctx %s/%s · %d%%</span>`, cls, humanTokens(cur), humanTokens(limit), pct)
}

// humanTokens renders a token count compactly (1.5k, 128.0k, 2.5M).
func humanTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	default:
		return strconv.FormatInt(n, 10)
	}
}

// renderCostFooter renders the ambient credit/budget meter.
func renderCostFooter(meter *telemetry.Meter, allowance float64) string {
	totals := meter.Totals()
	budget := telemetry.Budget{AllowanceCredits: allowance}
	frac := budget.FractionUsed(totals.Credits())
	pct := frac * 100
	if pct > 100 {
		pct = 100
	}
	return fmt.Sprintf(
		`<span class="credits">%s</span>`+
			`<span class="meter"><span class="meter-fill" style="width:%.1f%%"></span></span>`+
			`<span class="pct">%.0f%%</span>`,
		esc(telemetry.FormatCredits(totals.Credits())), pct, frac*100)
}

// clampLines limits text to at most n lines, appending an elision note when it
// truncates, so a large tool result never floods the timeline.
func clampLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n") + fmt.Sprintf("\n… (+%d more lines)", len(lines)-n)
}
