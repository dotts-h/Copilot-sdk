package web

import (
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"
	"time"

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
	case convo.RoleDecision:
		return renderDecisionNote(t.Decision)
	case convo.RoleHookRun:
		return renderHookRunNote(t.HookRun)
	}
	return ""
}

// renderHookRunNote renders a PostToolUse hook-execution annotation (G5,
// ADR-0032): a compact, muted timeline line naming the hook, its resolved command,
// an exit/timeout status, and a bounded snippet of the command's output. The
// output is UNTRUSTED — html/template auto-escapes it (ADR-0001), so it is shown
// verbatim-as-text and can never inject markup or act as an agent instruction. It
// is telemetry, not a control.
func renderHookRunNote(h *convo.HookRunView) string {
	if h == nil {
		return ""
	}
	status := "ok"
	switch {
	case h.TimedOut:
		status = "timed out"
	case h.Failed:
		status = fmt.Sprintf("exited %d", h.ExitCode)
	}
	glyph := "⚙"
	if h.Failed {
		glyph = "⚠"
	}
	return frag("hookRunNote", map[string]any{
		"Glyph": glyph, "HookID": h.HookID, "Command": h.Command,
		"Status": status, "Failed": h.Failed,
		"Output": h.Output, "HasOutput": strings.TrimSpace(h.Output) != "",
	})
}

// renderDecisionNote renders a governance "why" annotation (ADR-0031): a compact,
// muted timeline line explaining that a hook auto-approved ("auto-approved by X")
// or denied ("denied: reason") a tool call without pausing for a gate. It is an
// annotation, not a control — the call already proceeded or was blocked.
func renderDecisionNote(d *convo.DecisionView) string {
	if d == nil {
		return ""
	}
	glyph, label := "✓", "auto-approved"
	if d.Kind == "deny" {
		glyph, label = "⛔", "denied"
	}
	return frag("decisionNote", map[string]any{
		"Kind": d.Kind, "Glyph": glyph, "Label": label,
		"HookID": d.HookID, "HasHook": d.HookID != "",
		"Detail": d.Detail, "HasDetail": d.Detail != "",
		"Reason": d.Reason, "HasReason": d.Reason != "",
	})
}

// maxToolResultLines bounds how many lines of a tool's result the timeline card
// renders, so a large command output can't flood the transcript.
const maxToolResultLines = 16

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
		// While the tool runs, the awaited result slot shows the skeleton
		// shimmer (W4, issue 0066 — the ADR-0037 staged-primitive's consumer);
		// it disappears with the settled (done/failed) re-render.
		"Pending": state == "running",
		"Result":  clampLines(tv.Result, maxToolResultLines), "ShowResult": tv.Done && tv.Result != "",
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

// maxDiffLines bounds how many diff lines the review lane renders, so a huge
// proposed change can't flood the timeline or balloon the SSE fragment. The
// remainder is elided with a note; the full change still applies on approve.
const maxDiffLines = 400

// renderPermForm renders an inline approve/reject control for a pending
// tool-permission request, posting the decision to /perm/{id}. A file-write
// request whose proposed change parses as a unified diff renders the dedicated
// review lane (item 3.1): the file, intention, and a collapsible inline diff with
// the approve/reject attached. Every other request (and a write with no parseable
// diff) renders the compact one-line form. The decision binds to the same
// /perm/{id} flow either way — the review lane is a richer affordance, not a new
// gate (ADR-0012).
func renderPermForm(req copilot.PermissionRequest) string {
	if view := parseUnifiedDiff(req.Diff); view.OK {
		return frag("permReview", map[string]any{
			"ID": req.ID, "FileName": req.FileName, "Intention": req.Intention,
			"Adds": view.Adds, "Dels": view.Dels,
			"Lines":  diffLineViews(view.Lines),
			"Reason": req.Reason, "HasReason": req.Reason != "",
		})
	}
	return frag("permForm", map[string]any{
		"ID": req.ID, "Detail": req.Detail,
		"Reason": req.Reason, "HasReason": req.Reason != "",
	})
}

// diffLineViews prepares parsed diff lines for the permReview template, bounding
// the count and mapping each kind to its CSS class and gutter marker. Numbers are
// rendered as strings ("" for the absent side) so the template stays declarative;
// the line Text is escaped by html/template at render time.
func diffLineViews(lines []diffLine) []map[string]any {
	n := len(lines)
	truncated := false
	if n > maxDiffLines {
		n, truncated = maxDiffLines, true
	}
	out := make([]map[string]any, 0, n+1)
	for _, l := range lines[:n] {
		out = append(out, map[string]any{
			"Class": diffClass(l.Kind), "Marker": diffMarker(l.Kind), "Label": diffLabel(l.Kind),
			"Old": gutterNum(l.OldNum), "New": gutterNum(l.NewNum), "Text": l.Text,
		})
	}
	if truncated {
		out = append(out, map[string]any{
			"Class": "diff-meta", "Marker": "", "Old": "", "New": "",
			"Text": fmt.Sprintf("… (+%d more lines — approve to apply the full change)", len(lines)-maxDiffLines),
		})
	}
	return out
}

// diffClass maps a diff line kind to its row CSS class.
func diffClass(k diffLineKind) string {
	switch k {
	case diffAdd:
		return "diff-add"
	case diffDel:
		return "diff-del"
	case diffHunk:
		return "diff-hunk"
	case diffMeta:
		return "diff-meta"
	default:
		return "diff-context"
	}
}

// diffMarker is the leading gutter glyph for a diff line kind (a redundant,
// non-color cue so add/remove are distinguishable without relying on color).
func diffMarker(k diffLineKind) string {
	switch k {
	case diffAdd:
		return "+"
	case diffDel:
		return "-"
	case diffContext:
		return " "
	default:
		return ""
	}
}

// diffLabel returns a visually-hidden prefix announcing an add/remove line to a
// screen reader, since the +/- gutter marker (the only non-color visual cue) is
// aria-hidden. Empty for context/hunk/meta, which read fine as-is (WCAG 1.3.1).
func diffLabel(k diffLineKind) string {
	switch k {
	case diffAdd:
		return "added "
	case diffDel:
		return "removed "
	default:
		return ""
	}
}

// gutterNum renders a line number for the diff gutter, blank when absent (0).
func gutterNum(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
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
// Each chip surfaces the sub-agent's Description as a title= tooltip so that,
// during a parallel run, concurrent sub-agents say *what* they are doing.
func renderSubagents(active []copilot.SubagentInfo) string {
	if len(active) == 0 {
		return ""
	}
	var b strings.Builder
	for _, sa := range active {
		// Description is model/SDK-originated text → it flows through html/template
		// auto-escaping (attribute context) exactly like Label, never trusted() raw
		// (ADR-0001). Empty descriptions render the prior chip shape: the template
		// omits the title attribute entirely.
		b.WriteString(frag("subagentChip", map[string]any{
			"Label": subagentLabel(sa), "Model": sa.Model, "Description": sa.Description,
		}))
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

// renderStatline builds the live chat statusline: the active model and mode, the
// context-window fill, the session timer, and the running message/tool counts and
// token/credit accounting. Caller must hold s.mu (it reads per-session counters);
// the meter has its own lock. The session timer reuses the .elapsed mechanism, so
// the client ticks it from data-start.
//
// Token/credit totals come from the per-session meter (sessionMeter) so the
// statusline reflects *this* conversation, not the account-wide meter that
// aggregates every cookie-keyed session (item 3.2 / TECH_DEBT #2). The topbar
// cost footer and the hard-cap projection stay account-wide on s.meter; the
// statusline's soft-warn tints when this conversation alone crosses the budget
// threshold, with the cumulative banner remaining the topbar gauge.
func renderStatline(s *Server) string {
	meter := s.sessionMeter
	in, cached, out := meter.TotalTokens()
	cacheWrite, reasoning := meter.ExtraTokens()
	totals := meter.Totals()

	hit := 0
	if in+cached > 0 {
		hit = int(float64(cached)/float64(in+cached)*100 + 0.5)
	}
	ctxPct := 0
	if s.ctxLimit > 0 {
		ctxPct = int(float64(s.ctxCurrent)/float64(s.ctxLimit)*100 + 0.5)
		if ctxPct > 100 {
			ctxPct = 100
		}
	}
	// Pre-flight estimate: what the next turn would cost to resend the current
	// context as input, so the abort decision is informed before sending. Shown
	// only once a context reading has arrived (EvContextWindow).
	est := meter.EstimateTurn(s.spec.Model, s.ctxCurrent)
	// Actual spend on the authoritative-cost-first basis (ADR-0033): the reported AIU
	// when the runtime reported one this session, else the price-book estimate. The
	// cell labels which source it is so an estimate is never mistaken for the real bill.
	estimateCredits := totals.Credits()
	reported := meter.HasReported()
	actualCredits := meter.ActualCredits()
	// Soft-warn: turn the cost item amber once this session's actual spend crosses the
	// budget threshold (the account-wide banner stays the topbar cost footer).
	costWarn := s.budget().Warned(actualCredits)
	// Compact burn-rate cell: account-wide projection off the ledger (ADR-0019),
	// shown only when there's a finite projection to make. Ambers when on track to
	// exhaust the monthly allowance before the month resets.
	hasForecast, fShort, fWarn, fTitle := statlineForecast(s, time.Now())
	return frag("statline", map[string]any{
		"Model": def(s.spec.Model, "default"), "Mode": s.mode,
		"HasCtx": s.ctxLimit > 0, "CtxPct": ctxPct,
		"StartMs": s.sessionStartMs, "Msgs": s.messagesSent, "Tools": s.toolsUsed,
		"In": humanTokens(in), "Out": humanTokens(out),
		"CacheRead": humanTokens(cached), "CacheWrite": humanTokens(cacheWrite),
		"Reasoning": humanTokens(reasoning), "Hit": hit,
		"HasEst": s.ctxCurrent > 0, "Est": telemetry.FormatCredits(est.Credits()),
		"CostWarn": costWarn,
		// Reported: the cell shows GitHub's authoritative figure; otherwise it shows
		// the price-book estimate, tagged "est". ShowEstimate surfaces the estimate
		// alongside the reported figure only when the two diverge, so the drift is
		// visible without cluttering the common case where they agree.
		"Credits":      telemetry.FormatCredits(actualCredits),
		"Reported":     reported,
		"Estimate":     telemetry.FormatCredits(estimateCredits),
		"ShowEstimate": reported && math.Abs(actualCredits-estimateCredits) >= 0.005,
		"USD":          telemetry.FormatUSD(totals.USD()),
		"HasForecast":  hasForecast, "Forecast": fShort, "ForecastWarn": fWarn, "ForecastTitle": fTitle,
	})
}

// statlineForecast builds the compact statusline burn-rate cell from the
// account-wide ledger projection (ADR-0019). It surfaces only a finite OK
// projection ("cap ~Nd"); every degenerate Status (no budget, idle, exhausted)
// leaves the cell off so the statusline stays uncluttered (the Telemetry page
// carries the explanatory text). The title spells out the rate, date, and turns.
func statlineForecast(s *Server, now time.Time) (show bool, short string, warn bool, title string) {
	fc, ok := s.forecast(now)
	if !ok || fc.Status != telemetry.ProjectionOK {
		return false, "", false, ""
	}
	// Ceil to match the ExhaustionDate (= today + ⌈DaysToCap⌉) named in the title.
	short = fmt.Sprintf("cap ~%dd", int(math.Ceil(fc.DaysToCap)))
	title = fmt.Sprintf("at ~%s/day, budget runs out around %s (~%d turns left)",
		telemetry.FormatCredits(fc.DailyRate), fc.ExhaustionDate.UTC().Format("2006-01-02"),
		int(math.Round(fc.TurnsToCap)))
	return true, short, forecastSoon(fc.ExhaustionDate, now), title
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

// renderCostFooter renders the ambient credit/budget meter from the account-wide
// month-to-date credits (ledger-derived, ADR-0016 — callers pass
// s.monthToDate().Credits(), not the in-process meter). It turns amber and shows a
// warning glyph once spend crosses the soft threshold (Budget.Warned), making the
// topbar itself the ambient over-budget banner.
func renderCostFooter(credits float64, budget telemetry.Budget) string {
	frac := budget.FractionUsed(credits)
	pct := frac * 100
	if pct > 100 {
		pct = 100
	}
	return frag("costFooter", map[string]any{
		"Credits":  telemetry.FormatCredits(credits),
		"Width":    fmt.Sprintf("%.1f%%", pct),
		"PctWhole": fmt.Sprintf("%.0f", frac*100),
		"Warn":     budget.Warned(credits),
	})
}

// renderActualCostFooter renders the ambient credit/budget meter on the
// authoritative-cost-first basis (ADR-0033): the month-to-date total prefers GitHub's
// reported AIU where present and falls back to the price-book estimate, and the figure
// is tagged with its source (reported / estimated / mixed) so an estimate is never read
// as the real bill. It is the cost-footer sibling of renderCostFooter, which still
// serves callers that only have the estimate-priced Cost.
func renderActualCostFooter(a telemetry.ActualSpend, budget telemetry.Budget) string {
	credits := a.ActualCredits
	frac := budget.FractionUsed(credits)
	pct := frac * 100
	if pct > 100 {
		pct = 100
	}
	// Source label: fully reported, fully estimated, or a mix (some turns reported,
	// some still on the estimate). A title spells the estimate out when it diverges.
	source := "est"
	title := "estimated from the price book — GitHub has not reported a cost yet"
	switch {
	case a.AllReported && a.AnyReported:
		source, title = "reported", "GitHub-reported actual cost"
	case a.AnyReported:
		source = "mixed"
		title = fmt.Sprintf("reported where GitHub billed it, estimated elsewhere (price-book estimate: %s)",
			telemetry.FormatCredits(a.EstimateCredits))
	}
	return frag("costFooter", map[string]any{
		"Credits":  telemetry.FormatCredits(credits),
		"Width":    fmt.Sprintf("%.1f%%", pct),
		"PctWhole": fmt.Sprintf("%.0f", frac*100),
		"Warn":     budget.Warned(credits),
		"Source":   source,
		"Title":    title,
	})
}

// renderBudgetForm renders the inline hard-cap gate: a paused over-budget turn
// with proceed / raise-cap / cancel controls, reusing the permission-form look.
// It posts each decision to /budget/{action}.
func renderBudgetForm(projected, capCredits float64) string {
	return frag("budgetForm", map[string]any{
		"Projected": telemetry.FormatCredits(projected),
		"Cap":       telemetry.FormatCredits(capCredits),
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
