package web

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dotts-h/copilot-sdk/internal/config"
	"github.com/dotts-h/copilot-sdk/internal/ctxforge"
	"github.com/dotts-h/copilot-sdk/internal/telemetry"
)

// This file renders the top-level page partials served by GET /page/{name} and
// swapped into #main. They mirror the TUI's per-page views (internal/tui/views.go)
// as server-rendered HTML.

// pageNames is the nav order.
var pageNames = []struct{ slug, label string }{
	{"chat", "Chat"},
	{"sessions", "Sessions"},
	{"telemetry", "Telemetry"},
	{"skills", "Skills"},
	{"instructions", "Instructions"},
	{"agents", "Agents"},
	{"workflows", "Workflows"},
	{"runs", "Runs"},
	{"mcp", "MCP"},
	{"snippets", "Snippets"},
	{"models", "Models"},
	{"settings", "Settings"},
	{"help", "Help"},
}

// spendWindows is the allowed set of Telemetry "spend over time" trend windows
// (days); defaultSpendWindow is the fallback for a missing/out-of-range value and
// the historical behavior. The maxUSD scaling stays window-local (REGRESSIONS #14).
var spendWindows = []int{14, 30, 90}

const defaultSpendWindow = 14

// clampWindow parses a ?window= value to one of spendWindows, falling back to
// defaultSpendWindow (14) for an empty, unparseable, or out-of-range value.
func clampWindow(raw string) int {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return defaultSpendWindow
	}
	for _, w := range spendWindows {
		if n == w {
			return n
		}
	}
	return defaultSpendWindow
}

// renderPage returns the partial for a nav slug, or chat for unknown slugs. The
// window string is the (already raw) ?window= value, used only by the Telemetry
// page's trend selector; every other page ignores it.
func (s *Server) renderPage(slug, window string) string {
	switch slug {
	case "sessions":
		return s.sessionsPartial()
	case "telemetry":
		return s.telemetryPartial(clampWindow(window))
	case "skills":
		return s.skillsPartial()
	case "instructions":
		return s.instructionsPartial()
	case "agents":
		return s.agentsPartial()
	case "workflows":
		return s.workflowsPartial()
	case "runs":
		return s.runsPartial()
	case "mcp":
		return s.mcpServersPartial()
	case "snippets":
		return s.snippetsPartial()
	case "models":
		return s.modelsPartial()
	case "settings":
		return s.settingsPartial()
	case "help":
		return s.helpPartial()
	default:
		return s.chatPartial()
	}
}

// renderShortcuts renders the keyboard-shortcuts table (key → action) shared by
// the Help page and the help overlay. Keys and labels are HTML-escaped; the
// non-rebindable Esc-closes-overlay convention is appended as a fixed row.
func renderShortcuts(keymap []config.ResolvedKey) string {
	var b strings.Builder
	b.WriteString(`<table class="kv shortcuts">`)
	for _, k := range keymap {
		b.WriteString(`<tr><th><kbd>` + esc(k.Key) + `</kbd></th><td>` + esc(k.Label) + `</td></tr>`)
	}
	b.WriteString(`<tr><th><kbd>Esc</kbd></th><td>Close the shortcuts overlay</td></tr>`)
	b.WriteString(`</table>`)
	return b.String()
}

// helpOverlay renders the body-level keyboard-shortcuts overlay (hidden until
// the bound key opens it). It lives in the page shell so it works across htmx
// navigation; the keymap is the live, config-resolved set.
func helpOverlay(keymap []config.ResolvedKey) string { return helpOverlayAttr(keymap, "") }

// helpOverlayAttr renders the overlay with extra attributes spliced onto its
// root element — used to add hx-swap-oob="true" for the Settings live-apply swap
// (the OOB re-render is matched by #help-overlay), while the index render passes
// none. Keys/labels are HTML-escaped via renderShortcuts (ADR-0001).
func helpOverlayAttr(keymap []config.ResolvedKey, extraAttr string) string {
	return `<div id="help-overlay" class="overlay" hidden role="dialog" aria-modal="true" aria-label="Keyboard shortcuts"` + extraAttr + `>` +
		`<div class="overlay-card"><h2>Keyboard shortcuts</h2>` +
		renderShortcuts(keymap) +
		`<p class="dim">Shortcuts are ignored while you're typing in a field. Customise them on the Settings page.</p>` +
		`<button type="button" class="overlay-close" onclick="toggleHelpOverlay(false)">Close</button></div></div>`
}

// keymapJSON serializes the resolved keymap to the action→key JSON the frontend
// dispatcher reads from <body data-keymap>. Shared by the initial index render
// and the Settings live-apply OOB swap so both surfaces carry one source (the map
// marshals with sorted keys → deterministic).
func keymapJSON(keymap []config.ResolvedKey) string {
	dispatch := make(map[string]string, len(keymap))
	for _, k := range keymap {
		dispatch[k.ID] = k.Key
	}
	j, _ := json.Marshal(dispatch)
	return string(j)
}

// keymapLiveApply builds the Settings POST's live-apply payload: an hx-swap-oob
// re-render of the help overlay (matched by #help-overlay) plus a script that
// calls applyKeymap to update <body data-keymap> and the JS dispatcher's reverse
// map, so a rebind takes effect WITHOUT a full page reload (TECH_DEBT #13). The
// keymap reflects the PERSISTED config, so a no-op or rolled-back save re-emits
// the in-sync keymap and can never desync the live attribute from disk. The JSON
// is HTML-safe in the <script> context: encoding/json escapes <, >, & (so no
// </script> can form) and every key is a validated single character.
func keymapLiveApply(keymap []config.ResolvedKey) string {
	return helpOverlayAttr(keymap, ` hx-swap-oob="true"`) +
		`<script>applyKeymap(` + keymapJSON(keymap) + `)</script>`
}

// helpPartial renders the static Help/reference page: how the panels work, the
// composer slash commands, and the keyboard shortcuts. It is the discoverability
// surface behind /help in the composer.
func (s *Server) helpPartial() string {
	s.hub.forgeMu.Lock()
	keymap := s.config.Keymap()
	s.hub.forgeMu.Unlock()

	cmd := func(name, desc string) string {
		return `<tr><th><code>` + esc(name) + `</code></th><td>` + esc(desc) + `</td></tr>`
	}
	var b strings.Builder
	b.WriteString(`<section class="page help" tabindex="0"><h2>Help</h2>`)
	b.WriteString(`<p class="dim">my-orchestra is a cost-aware coding companion. Chat streams live; ` +
		`every tool call, the reasoning, and the credit spend are shown as they happen.</p>`)

	b.WriteString(`<h3>Composer commands</h3>`)
	b.WriteString(`<p class="dim">Type these in the chat composer instead of a prompt.</p>`)
	b.WriteString(`<table class="kv">`)
	b.WriteString(cmd("/model [name]", "Switch the model in place (restarts the session); no name shows the current one."))
	b.WriteString(cmd("/effort [low|medium|high]", "Set the reasoning effort (restarts the session); no value shows the current one."))
	b.WriteString(cmd("/agent [id|none]", "Activate a forge agent (applies its model + reasoning) or clear it."))
	b.WriteString(cmd("/plan [on|off]", "Toggle plan mode — the agent drafts a plan you approve or revise inline before it acts."))
	b.WriteString(cmd("/auto [on|off]", "Toggle autopilot — the agent runs tools without pausing to ask."))
	b.WriteString(cmd("/ask [on|off]", "Toggle ask mode — the agent checks in before each action."))
	b.WriteString(cmd("/clear", "Reset the conversation and start a fresh session."))
	b.WriteString(cmd("/cost", "Show credit usage and refresh the cost meter."))
	b.WriteString(cmd("/attach <path>", "Queue a file to send with your next message."))
	b.WriteString(cmd("/chat … /settings", "Jump to a page (chat, telemetry, skills, instructions, agents, settings)."))
	b.WriteString(cmd("/help", "List the commands in the timeline."))
	b.WriteString(`</table>`)

	b.WriteString(`<h3>Panels</h3><table class="kv">`)
	rows := [][2]string{
		{"Chat", "Stream prompts and replies; approve tool permissions, answer the agent's questions (ask_user), fill schema-driven forms from MCP servers (elicitation), and review its plans (approve or request changes) — all inline; abort an in-flight turn with ⏹ stop. " +
			"Type ahead while a turn runs — extra prompts queue and send automatically when the turn ends."},
		{"Sessions", "List, resume, or delete past conversations. Resuming restores the full context (the first turn after a gap won't hit the prompt cache); start fresh with + New chat."},
		{"Telemetry", "Live credit/token spend, per-model breakdown, and your monthly budget."},
		{"Skills", "Reusable prompt fragments; toggle which are active for the session."},
		{"Instructions", "Always-on guidance, ordered by priority. Import project files pulls in .github/copilot-instructions.md, AGENTS.md, and CLAUDE.md."},
		{"Agents", "Named model + reasoning + skill + tool-allowlist presets; select one as the default. A built-in Chat agent is always available."},
		{"Models", "Pick the model the session uses and its reasoning effort; selecting either restarts the session on your next prompt."},
		{"Settings", "Edit config.json's main knobs (model, effort, streaming, budget); applied on your next session. Advanced keys edit the file directly."},
	}
	for _, r := range rows {
		fmt.Fprintf(&b, `<tr><th>%s</th><td>%s</td></tr>`, esc(r[0]), esc(r[1]))
	}
	b.WriteString(`</table>`)

	b.WriteString(`<h3>Keyboard shortcuts</h3>`)
	b.WriteString(`<p class="dim">Press these anywhere outside a text field; the overlay also opens with its key. Customise them on the Settings page.</p>`)
	b.WriteString(renderShortcuts(keymap))

	b.WriteString(`</section>`)
	return b.String()
}

// chatPartial renders the chat page. The dynamic regions (timeline, pending
// request forms, sub-agent strip, context meter) are rendered by their own
// renderers and injected into the chatPage template as trusted HTML — they are
// already escaped at the source (renderTurn/renderPermForm/… via richtext/esc).
func (s *Server) chatPartial() string {
	s.mu.Lock()
	timeline := renderTimelineInner(&s.state)
	var perms, asks, plans, elicits strings.Builder
	for _, p := range s.perms {
		perms.WriteString(renderPermForm(p))
	}
	for _, q := range s.inputs {
		asks.WriteString(renderAskForm(q))
	}
	for _, p := range s.plans {
		plans.WriteString(renderPlanForm(p))
	}
	for _, e := range s.elicits {
		elicits.WriteString(renderElicitForm(e))
	}
	ctx := renderCtx(s.ctxCurrent, s.ctxLimit, s.compacting)
	subagents := renderSubagents(s.subagents)
	lanes := renderLanes(s.run)
	statline := renderStatline(s)
	budget := s.renderGate()
	s.mu.Unlock()

	return frag("chatPage", map[string]any{
		"Timeline": trusted(timeline), "Perms": trusted(perms.String()),
		"Asks": trusted(asks.String()), "Plans": trusted(plans.String()),
		"Elicits": trusted(elicits.String()), "Subagents": trusted(subagents),
		"Lanes": trusted(lanes),
		"Ctx":   trusted(ctx), "Statline": trusted(statline), "Budget": trusted(budget),
	})
}

func (s *Server) telemetryPartial(window int) string {
	// Account-wide budget accounting reads month-to-date from the persisted ledger
	// so "remaining this month" survives a restart (ADR-0016). The live token split
	// and per-model table below stay on the in-process meter — one source per
	// surface (see REGRESSIONS "two meters", now three sources).
	month := s.monthToDate()
	in, cached, out := s.meter.TotalTokens()
	budget := s.budget()
	frac := budget.FractionUsed(month.Credits())

	rows := [][2]string{
		{"Total cost", fmt.Sprintf("%s (%s)", telemetry.FormatCredits(month.Credits()), telemetry.FormatUSD(month.USD()))},
		{"Monthly budget", fmt.Sprintf("%.2f of %.0f cr", month.Credits(), budget.AllowanceCredits)},
		{"Remaining", telemetry.FormatCredits(budget.Remaining(month.Credits()))},
	}
	if s.hardCap > 0 {
		rows = append(rows, [2]string{"Hard cap", fmt.Sprintf("%.0f cr — a turn projected to exceed it pauses for confirmation", s.hardCap)})
	}
	if aiu := s.meter.ReportedAIU(); aiu > 0 {
		rows = append(rows, [2]string{"GitHub-reported cost", fmt.Sprintf("%.4f AIU", aiu)})
	}
	rows = append(rows, [2]string{"Tokens", fmt.Sprintf("input %d · cached %d · output %d", in, cached, out)})

	pct := frac * 100
	if pct > 100 {
		pct = 100
	}
	models := make([]map[string]any, 0)
	for _, r := range s.meter.ByModel() {
		models = append(models, map[string]any{
			"Model": r.Model, "In": r.InputTokens, "Cached": r.CachedTokens, "Out": r.OutputTokens,
			"Credits": fmt.Sprintf("%.2f", r.Credits()), "USD": telemetry.FormatUSD(r.USD()),
		})
	}
	days, shares, hasHistory := s.spendTrend(window)
	now := time.Now()
	agents, workflows := s.spendShares(now)
	var forecast map[string]any
	if fc, ok := s.forecast(now); ok {
		forecast = forecastView(fc, budget.AllowanceCredits, now)
	}
	windows := make([]map[string]any, 0, len(spendWindows))
	for _, w := range spendWindows {
		windows = append(windows, map[string]any{"Value": w, "Active": w == window})
	}
	return frag("telemetryPage", map[string]any{
		"Rows": rows, "Width": fmt.Sprintf("%.1f%%", pct), "Models": models,
		"Days": days, "Shares": shares, "HasHistory": hasHistory,
		"AgentShares": agents, "WorkflowShares": workflows,
		"Forecast": forecast, "Windows": windows,
	})
}

// spendTrend builds the persisted-ledger trend data for the Telemetry page: the
// per-day spend over the chosen window (most recent last, each bar scaled to the
// busiest day in view) and each model's share of all-time spend. The window (days)
// is the selector value, clamped upstream to spendWindows. Empty when no ledger is
// wired or none yet.
func (s *Server) spendTrend(window int) (days, shares []map[string]any, hasHistory bool) {
	days, shares = []map[string]any{}, []map[string]any{}
	if s.spend == nil {
		return days, shares, false
	}
	records := s.spend.Records()
	if len(records) == 0 {
		return days, shares, false
	}

	daily := telemetry.DailyTotals(records)
	// Slice to the chosen window FIRST, so a long history stays scannable and the
	// max below is computed over what's shown — never over full history (the
	// REGRESSIONS #14 invariant: an off-window peak must not shrink visible bars).
	if window > 0 && len(daily) > window {
		daily = daily[len(daily)-window:]
	}
	// Scale bars to the busiest day *in view*, so the visible window always uses
	// the full width even when an off-screen older day spent more.
	var maxUSD float64
	for _, d := range daily {
		if d.USD > maxUSD {
			maxUSD = d.USD
		}
	}
	for _, d := range daily {
		w := 0.0
		if maxUSD > 0 {
			w = d.USD / maxUSD * 100
		}
		days = append(days, map[string]any{
			"Day": d.Day, "Credits": fmt.Sprintf("%.2f", d.Credits),
			"USD": telemetry.FormatUSD(d.USD), "Turns": d.Count,
			"Width": fmt.Sprintf("%.1f%%", w),
		})
	}

	for _, m := range telemetry.ModelShares(records) {
		shares = append(shares, map[string]any{
			"Model": m.Model, "Credits": fmt.Sprintf("%.2f", m.Credits),
			"Pct": fmt.Sprintf("%.0f", m.Fraction*100), "Width": fmt.Sprintf("%.1f%%", m.Fraction*100),
		})
	}
	return days, shares, true
}

// spendShares builds the per-agent and per-workflow cost breakdowns for the
// Telemetry page from the persisted ledger — the orchestration-aware "which agent
// / which workflow burned my budget" view (ADR-0018) — each row joined to its burn
// TRAJECTORY (F3): the same per-bucket Forecast slope that AgentShares/WorkflowShares
// can't answer. Agent/workflow ids are resolved to display names under forgeMu
// (falling back to the raw id when a persona was renamed or deleted; an empty agent
// id is the built-in chat). The trajectory is keyed by the raw id (BucketForecasts'
// key), joined before the id is resolved to a label. One `now` is threaded through
// both the per-bucket Forecast and the month projection (the ADR-0019 single-`now`
// gotcha, per bucket). Both lists are empty when no ledger is wired or it holds no
// relevant records.
func (s *Server) spendShares(now time.Time) (agents, workflows []map[string]any) {
	agents, workflows = []map[string]any{}, []map[string]any{}
	if s.spend == nil {
		return agents, workflows
	}
	records := s.spend.Records()
	if len(records) == 0 {
		return agents, workflows
	}
	budget := s.budget()
	agentTraj := bucketTrajectories(telemetry.BucketForecasts(records, budget, now, agentKey, true), now)
	workflowTraj := bucketTrajectories(telemetry.BucketForecasts(records, budget, now, workflowKey, false), now)
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	for _, a := range telemetry.AgentShares(records) {
		row := shareRow(s.agentLabel(a.AgentID), a.Credits, a.Fraction)
		row["Traj"] = agentTraj[a.AgentID]
		agents = append(agents, row)
	}
	for _, w := range telemetry.WorkflowShares(records) {
		row := shareRow(s.workflowLabel(w.WorkflowID), w.Credits, w.Fraction)
		row["Traj"] = workflowTraj[w.WorkflowID]
		workflows = append(workflows, row)
	}
	return agents, workflows
}

// agentKey / workflowKey are the bucket keys BucketForecasts projects per (the same
// keyOf the *Shares readers bucket by), so the trajectory join lines up with the
// share rows exactly.
func agentKey(r telemetry.SpendRecord) string    { return r.AgentID }
func workflowKey(r telemetry.SpendRecord) string { return r.WorkflowID }

// bucketTrajectories renders each bucket's burn-trajectory sentence keyed by the
// bucket's raw id, so spendShares can join it onto the matching share row before
// the id is resolved to a display label. An empty string (no-budget bucket) joins
// as no trajectory cell.
func bucketTrajectories(bs []telemetry.BucketProjection, now time.Time) map[string]string {
	out := make(map[string]string, len(bs))
	for _, b := range bs {
		out[b.Key] = bucketTrajectoryText(b.Projection, now)
	}
	return out
}

// bucketTrajectoryText turns a per-bucket Projection into its trajectory sentence
// (F3). The honest per-bucket framing is rate + month projection, NOT a per-bucket
// exhaustion date: a bucket has no own allowance, so the account-wide DaysToCap/
// ExhaustionDate fields are deliberately not surfaced. Each degenerate Status gets
// its own sentence (or none) rather than a bogus figure, mirroring forecastView:
//   - OK: the bucket's recent rate + where it lands this month at that pace;
//   - Idle: no recent spend to project from;
//   - NoBudget / Exhausted: empty — no per-bucket trajectory line.
func bucketTrajectoryText(p telemetry.Projection, now time.Time) string {
	switch p.Status {
	case telemetry.ProjectionOK:
		// Project where this bucket lands by month-end at its recent pace: what it has
		// already spent this month plus the rate over the days still to come. No
		// per-bucket cap — just the trajectory.
		month := p.UsedCredits + p.DailyRate*float64(daysLeftInMonth(now))
		return fmt.Sprintf("at ~%s/day, on pace for ~%s this month",
			telemetry.FormatCredits(p.DailyRate), telemetry.FormatCredits(month))
	case telemetry.ProjectionIdle:
		return "idle — no recent spend to project a rate from"
	default: // ProjectionNoBudget, ProjectionExhausted — no per-bucket trajectory line
		return ""
	}
}

// daysLeftInMonth counts the whole UTC days remaining after now's day through the
// end of its calendar month (zero on the last day) — the horizon the per-bucket
// month projection extrapolates the rate over. Pure: same now → same count.
func daysLeftInMonth(now time.Time) int {
	t := now.UTC()
	y, m, d := t.Date()
	firstNextMonth := time.Date(y, m+1, 1, 0, 0, 0, 0, time.UTC)
	daysInMonth := firstNextMonth.AddDate(0, 0, -1).Day()
	return daysInMonth - d
}

// shareRow is the template shape shared by every spend-breakdown bar (per-model,
// per-agent, per-workflow): a label, a credit total, and a fraction rendered as a
// percentage and a bar width.
func shareRow(label string, credits, fraction float64) map[string]any {
	return map[string]any{
		"Label": label, "Credits": fmt.Sprintf("%.2f", credits),
		"Pct": fmt.Sprintf("%.0f", fraction*100), "Width": fmt.Sprintf("%.1f%%", fraction*100),
	}
}

// forecastView turns a burn-rate Projection into the Telemetry-page line (A3 /
// ADR-0019): a human sentence plus a Warn flag that ambers the line when the
// projected exhaustion falls within the current month (on track to blow this
// month's budget). Each degenerate Status gets its own explanatory sentence
// rather than a bogus date.
func forecastView(fc telemetry.Projection, allowance float64, now time.Time) map[string]any {
	switch fc.Status {
	case telemetry.ProjectionNoBudget:
		return map[string]any{"Text": "Set a monthly budget to see a burn-rate forecast.", "Warn": false}
	case telemetry.ProjectionIdle:
		return map[string]any{"Text": "No recent spend to project a burn rate from.", "Warn": false}
	case telemetry.ProjectionExhausted:
		return map[string]any{"Text": "This month's budget is already exhausted.", "Warn": true}
	default:
		// Ceil so the "~N days" count matches ExhaustionDate (= today + ⌈DaysToCap⌉);
		// rounding the count independently could print "~9 days" beside a +10-day date.
		days := int(math.Ceil(fc.DaysToCap))
		turns := int(math.Round(fc.TurnsToCap))
		text := fmt.Sprintf("At ~%s/day, your %.0f cr budget runs out around %s (~%d %s, ~%d %s).",
			telemetry.FormatCredits(fc.DailyRate), allowance,
			fc.ExhaustionDate.UTC().Format("2006-01-02"),
			days, plural(days, "day", "days"), turns, plural(turns, "turn", "turns"))
		return map[string]any{"Text": text, "Warn": forecastSoon(fc.ExhaustionDate, now)}
	}
}

// forecastSoon reports whether a projected exhaustion date falls on or before the
// last instant of now's UTC month — i.e. the burn rate is on track to spend the
// monthly allowance before it resets. Pure: same inputs → same answer.
func forecastSoon(exhaust, now time.Time) bool {
	if exhaust.IsZero() {
		return false
	}
	y, m, _ := now.UTC().Date()
	endOfMonth := time.Date(y, m+1, 1, 0, 0, 0, 0, time.UTC).Add(-time.Nanosecond)
	return !exhaust.After(endOfMonth)
}

// plural picks the singular or plural noun for n.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// agentLabel resolves an agent id to its display name for the cost breakdown,
// falling back to the raw id (a since-renamed/deleted agent) and labelling the
// empty-agent bucket as the built-in chat. Caller holds forgeMu.
func (s *Server) agentLabel(id string) string {
	if id == "" {
		return "chat (built-in)"
	}
	if a := s.forge.Agent(id); a != nil {
		return a.Name
	}
	return id
}

// workflowLabel resolves a workflow id to its display name, falling back to the
// raw id when the workflow was renamed or deleted. Caller holds forgeMu.
func (s *Server) workflowLabel(id string) string {
	if w := s.forge.Workflow(id); w != nil {
		return w.Name
	}
	return id
}

// handleSpendExport streams the full persisted spend ledger as a CSV download,
// so spend can be analysed outside the app (the trend view's accountable-ledger
// promise). Empty (header only) when no ledger is wired.
func (s *Server) handleSpendExport(w http.ResponseWriter, r *http.Request) {
	var records []telemetry.SpendRecord
	if s.spend != nil {
		records = s.spend.Records()
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="my-orchestra-spend.csv"`)
	if err := telemetry.WriteCSV(w, records); err != nil {
		s.logger.Printf("export spend csv: %v", err)
	}
}

func (s *Server) skillsPartial() string {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	rows := make([]map[string]any, 0, len(s.forge.Skills))
	for _, sk := range s.forge.Skills {
		rows = append(rows, map[string]any{
			"Kind": "skills", "ID": sk.ID, "On": sk.Enabled, "Name": sk.Name, "Desc": truncate(sk.Description, 80),
		})
	}
	return frag("skillsPage", map[string]any{"Add": addData("skills", "skill"), "Rows": rows})
}

func (s *Server) instructionsPartial() string {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	rows := make([]map[string]any, 0, len(s.forge.Instructions))
	for _, ins := range s.forge.Instructions {
		rows = append(rows, map[string]any{
			"Kind": "instructions", "ID": ins.ID, "On": ins.Enabled,
			"Name": fmt.Sprintf("%s (p%d)", ins.Title, ins.Priority), "Desc": truncate(ins.Body, 80),
		})
	}
	return frag("instructionsPage", map[string]any{"Add": addData("instructions", "instruction"), "Rows": rows})
}

func (s *Server) agentsPartial() string {
	s.hub.forgeMu.Lock()
	defer s.hub.forgeMu.Unlock()
	rows := make([]map[string]any, 0, len(s.forge.Agents)+1)
	for _, a := range s.forge.Agents {
		desc := fmt.Sprintf("%s · %s · %s", a.Model, def(a.ReasoningEffort, "medium"), a.Description)
		rows = append(rows, map[string]any{
			"ID": a.ID, "Active": a.ID == s.config.DefaultAgent, "Name": a.Name, "Desc": desc,
		})
	}
	// Always offer the built-in chat agent (unless the forge defines its own), so
	// chat has a baseline persona with no config. It is virtual: selectable but
	// not editable/deletable (ADR 0003).
	if !s.forge.HasOwnChatAgent() {
		b := ctxforge.DefaultChatAgent()
		rows = append(rows, map[string]any{
			"ID": b.ID, "Active": b.ID == s.config.DefaultAgent, "Name": b.Name + " (built-in)",
			"Desc": b.Description, "Builtin": true,
		})
	}
	return frag("agentsPage", map[string]any{"Add": addData("agents", "agent"), "Rows": rows})
}

// addData is the "+ Add" button's template data (route slug + singular noun).
func addData(kind, noun string) map[string]any { return map[string]any{"Kind": kind, "Noun": noun} }

// modelsPartial renders the model picker: every model the account can use, with
// the current one marked and the rest offering a one-click switch (POST
// /models/{id}/select, which restarts the session — workstream 3). It degrades
// to a notice when the runtime can't list models.
func (s *Server) modelsPartial() string {
	models, err := s.client.ListModels(context.Background())
	if err != nil {
		return frag("modelsPage", map[string]any{"Err": err.Error()})
	}

	s.mu.Lock()
	current := s.spec.Model
	curEffort := s.spec.ReasoningEffort
	s.mu.Unlock()

	rows := make([]map[string]any, 0, len(models))
	var efforts []map[string]any
	for _, m := range models {
		desc := m.ID
		if e := strings.Join(m.SupportedReasoningEfforts, ", "); e != "" {
			desc += " · reasoning: " + e
		}
		rows = append(rows, map[string]any{
			"ID": m.ID, "Active": m.ID == current, "Name": def(m.Name, m.ID), "Desc": desc,
		})
		if m.ID == current {
			for _, e := range m.SupportedReasoningEfforts {
				efforts = append(efforts, map[string]any{"Value": e, "Active": e == curEffort})
			}
		}
	}
	return frag("modelsPage", map[string]any{"Rows": rows, "Efforts": efforts})
}

func (s *Server) settingsPartial() string {
	return s.renderSettings("", "")
}

func def(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// truncate shortens s to at most n runes, appending an ellipsis when it cuts.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
