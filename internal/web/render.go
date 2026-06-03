package web

import (
	"fmt"
	"html"
	"strings"

	"github.com/dotts-h/copilot-sdk/internal/convo"
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

// renderStatus renders the status-line content swapped into #status. While a
// turn is active it appends an inline abort control (POST /abort); when idle it
// is just the (possibly empty) status text, so the button disappears on its own.
func renderStatus(text string, active bool) string {
	html := `<span class="status-text">` + esc(text) + `</span>`
	if active {
		html += ` <button class="abort" hx-post="/abort" hx-target="#status" hx-swap="innerHTML">⏹ stop</button>`
	}
	return html
}

// statusFragment is the SSE/OOB fragment carrying renderStatus output.
func statusFragment(text string, active bool) fragment {
	return fragment{Event: "status", HTML: renderStatus(text, active)}
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
