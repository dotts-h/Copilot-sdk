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

// This file renders convo state and copilot events into HTML fragments. The
// structural markup lives in templates/fragments.html (auto-escaped by
// html/template); each function here builds the template's data and executes it
// via frag(). Multi-line transcript text is escaped + newline-converted by the
// richtext template function — the same transform as esc().

// esc escapes text for safe insertion and converts newlines to <br>. It backs
// the richtext template function and is still used where a plain string (not a
// fragment) is needed.
func esc(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "\n", "<br>")
}

// deltaSpan is the incremental fragment appended to #cur for each streamed
// token. Wrapping in a span preserves whitespace across appends.
func deltaSpan(text string) string { return frag("deltaSpan", text) }

// renderTurn renders one committed transcript turn.
func renderTurn(t convo.Turn) string {
	switch t.Role {
	case convo.RoleUser:
		return frag("turnUser", t.Text)
	case convo.RoleAgent:
		return frag("turnAgent", t.Text)
	case convo.RoleReasoning:
		return frag("turnReasoning", t.Text)
	case convo.RoleSystem:
		return frag("turnSystem", t.Text)
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
	return frag("toolCard", map[string]any{
		"ID": tv.ID, "Name": tv.Name, "Args": tv.Args,
		"Glyph": glyph, "State": state,
		"Progress": tv.Progress, "ShowProgress": !tv.Done && tv.Progress != "",
		"Result": clampLines(tv.Result, 16), "ShowResult": tv.Done && tv.Result != "",
	})
}

// renderCur renders the trailing live node (#cur) from the in-flight buffer.
func renderCur(role convo.Role, text string) string {
	class := "turn assistant"
	if role == convo.RoleReasoning {
		class = "turn reasoning live"
	}
	return frag("cur", map[string]any{"Class": class, "Text": text, "Has": text != ""})
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
	return frag("permForm", map[string]any{"ID": id, "Detail": detail})
}

// renderAskForm renders an inline ask_user prompt: the question, one submit
// button per suggested choice, and (when freeform is allowed or no choices are
// offered) a text field for a custom answer. It posts to /ask/{id}.
func renderAskForm(req copilot.InputRequest) string {
	return frag("askForm", map[string]any{
		"ID": req.ID, "Question": req.Question, "Choices": req.Choices,
		"ShowFreeform": req.AllowFreeform || len(req.Choices) == 0,
	})
}

// renderPlanForm renders an inline exit-plan-mode review: the agent's summary,
// the full plan (collapsible), one approve button per offered action (the
// recommended one marked), and a freeform field to decline and request changes.
func renderPlanForm(req copilot.PlanRequest) string {
	return frag("planForm", map[string]any{
		"ID": req.ID, "Summary": req.Summary, "Plan": req.Plan,
		"Actions": req.Actions, "Recommended": req.Recommended,
	})
}

// renderElicitForm renders a schema-driven elicitation dialog from an MCP server:
// the message (and source), one control per field, and submit/decline buttons.
// It posts to /elicit/{id}.
func renderElicitForm(req copilot.ElicitRequest) string {
	views := make([]map[string]any, len(req.Fields))
	for i, f := range req.Fields {
		views[i] = elicitFieldView(f)
	}
	return frag("elicitForm", map[string]any{
		"ID": req.ID, "Message": req.Message, "Source": req.Source, "Fields": views,
	})
}

// elicitFieldView prepares one elicitation field for the elicitField template,
// choosing the control by type and resolving enum options + the default value.
func elicitFieldView(f copilot.ElicitField) map[string]any {
	type opt struct {
		Value, Label string
		Selected     bool
	}
	var opts []opt
	for i, o := range f.Enum {
		label := o
		if i < len(f.EnumLabels) && f.EnumLabels[i] != "" {
			label = f.EnumLabels[i]
		}
		opts = append(opts, opt{Value: o, Label: label, Selected: o == f.Default})
	}
	return map[string]any{
		"Key": elicitFieldKey(f.Name), "Label": f.Label, "Required": f.Required,
		"Type": f.Type, "Default": f.Default, "Description": f.Description,
		"Checked": f.Type == "boolean" && f.Default == "true",
		"HasEnum": len(f.Enum) > 0, "Options": opts,
		"Numeric": f.Type == "number" || f.Type == "integer", "Step": f.Type == "number",
	}
}

// elicitFieldKey returns the form-input name for an elicitation field. The "f."
// prefix keeps field inputs from colliding with the form's "action" button and
// lets handleElicit read each field's value by reconstructing the key.
func elicitFieldKey(name string) string { return "f." + name }

// subagentLabel returns a sub-agent's display name, falling back to its internal
// name, then to a generic label.
func subagentLabel(sa copilot.SubagentInfo) string {
	switch {
	case sa.DisplayName != "":
		return sa.DisplayName
	case sa.Name != "":
		return sa.Name
	default:
		return "sub-agent"
	}
}

// renderSubagents renders the background-activity strip: one animated chip per
// running sub-agent. Empty when nothing is running, so the strip is ambient.
func renderSubagents(active []copilot.SubagentInfo) string {
	if len(active) == 0 {
		return ""
	}
	var b strings.Builder
	for _, sa := range active {
		b.WriteString(frag("subagentChip", map[string]any{"Label": subagentLabel(sa), "Model": sa.Model}))
	}
	return b.String()
}

// renderStatus renders the status-line content swapped into #status. While a
// turn is active it appends a live elapsed-time timer (ticked client-side from
// the data-start epoch) and an inline abort control; when idle it is just the
// (possibly empty) status text.
func renderStatus(text string, active bool, startMs int64) string {
	return frag("status", map[string]any{
		"Text": text, "Active": active, "HasTimer": active && startMs > 0, "StartMs": startMs,
	})
}

// renderCtx renders the live context-window meter (#ctx): a token count and
// fill percentage, or a compaction indicator. Empty until the first reading.
func renderCtx(cur, limit int64, compacting bool) string {
	if compacting {
		return frag("ctx", map[string]any{"Compacting": true})
	}
	if limit <= 0 {
		if cur <= 0 {
			return ""
		}
		return frag("ctx", map[string]any{"ShowTokens": true, "Cur": humanTokens(cur)})
	}
	pct := int(float64(cur)/float64(limit)*100 + 0.5)
	if pct > 100 {
		pct = 100
	}
	cls := "ctx-meter"
	if pct >= 80 {
		cls += " warn"
	}
	return frag("ctx", map[string]any{
		"ShowMeter": true, "Class": cls, "Cur": humanTokens(cur), "Limit": humanTokens(limit), "Pct": pct,
	})
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
	return frag("costFooter", map[string]any{
		"Credits":  telemetry.FormatCredits(totals.Credits()),
		"Width":    fmt.Sprintf("%.1f%%", pct),
		"PctWhole": fmt.Sprintf("%.0f", frac*100),
	})
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
